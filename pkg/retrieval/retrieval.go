package retrieval

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/FavorLabs/favorX/pkg/accounting"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/cac"
	"github.com/FavorLabs/favorX/pkg/chunkinfo"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/p2p/protobuf"
	"github.com/FavorLabs/favorX/pkg/retrieval/aco"
	"github.com/FavorLabs/favorX/pkg/retrieval/pb"
	"github.com/FavorLabs/favorX/pkg/routetab"
	"github.com/FavorLabs/favorX/pkg/sctx"
	"github.com/FavorLabs/favorX/pkg/soc"
	"github.com/FavorLabs/favorX/pkg/storage"
	"github.com/FavorLabs/favorX/pkg/subscribe"
	"github.com/FavorLabs/favorX/pkg/tracing"
	"github.com/opentracing/opentracing-go"
	"resenje.org/singleflight"
)

type requestSourceContextKey struct{}

const (
	protocolName    = "retrieval"
	protocolVersion = "2.0.0"
	streamName      = "retrieval"
)

var _ Interface = (*Service)(nil)

type Interface interface {
	RetrieveChunk(ctx context.Context, rootAddr, chunkAddr boson.Address, index int64) (chunk boson.Chunk, err error)
	GetRouteScore(time int64) map[string]int64
}

type Service struct {
	addr         boson.Address
	streamer     p2p.Streamer
	storer       storage.Storer
	logger       logging.Logger
	metrics      metrics
	tracer       *tracing.Tracer
	chunkinfo    chunkinfo.Interface
	acoServer    *aco.AcoServer
	routeTab     routetab.RouteTab
	accounting   accounting.Interface
	singleflight singleflight.Group
	isFullNode   bool
	subPub       subscribe.SubPub
	relay        bool
}

type retrievalResult struct {
	chunk boson.Chunk
	err   error
}

const (
	retrieveChunkTimeout          = 10 * time.Second
	retrieveRetryIntervalDuration = 10 * time.Second
	totalRouteCount               = 5
)

func New(addr boson.Address, streamer p2p.Streamer, routeTable routetab.RouteTab, storer storage.Storer, relay bool,
	isFullNode bool, logger logging.Logger, tracer *tracing.Tracer, accounting accounting.Interface, subPub subscribe.SubPub) *Service {
	acoServer := aco.NewAcoServer()
	return &Service{
		addr:       addr,
		streamer:   streamer,
		storer:     storer,
		logger:     logger,
		metrics:    newMetrics(),
		tracer:     tracer,
		acoServer:  acoServer,
		routeTab:   routeTable,
		accounting: accounting,
		isFullNode: isFullNode,
		subPub:     subPub,
		relay:      relay,
	}
}

func (s *Service) Config(chunkInfo chunkinfo.Interface) {
	s.chunkinfo = chunkInfo
}

func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    protocolName,
		Version: protocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    streamName,
				Handler: s.handler,
			},
		},
	}
}

func (s *Service) RetrieveChunk(ctx context.Context, rootAddr, chunkAddr boson.Address, index int64) (chunk boson.Chunk, err error) {
	s.metrics.RequestCounter.Inc()

	flightRoute := fmt.Sprintf("%s,%s", rootAddr, chunkAddr)
	topCtx := ctx

	v, _, err := s.singleflight.Do(context.Background(), flightRoute, func(ctx context.Context) (interface{}, error) {
		var (
			maxRequestAttempt = 2
			requestAttempt    int
			resultC           = make(chan retrievalResult, totalRouteCount)
		)
		ctx = tracing.WithContext(context.Background(), tracing.FromContext(topCtx))
		// get the tracing span
		span, _, ctx := s.tracer.StartSpanFromContext(ctx, "retrieve-chunk", s.logger,
			opentracing.Tag{Key: "rootAddr,chunkAddr", Value: flightRoute})
		defer span.Finish()
		ticker := time.NewTicker(retrieveRetryIntervalDuration)
		defer ticker.Stop()

		routeList := s.getRetrievalRouteList(topCtx, rootAddr, chunkAddr, index)
		if len(routeList) == 0 {
			return nil, fmt.Errorf("no route available")
		}

		for requestAttempt < maxRequestAttempt {
			requestAttempt++
			for _, retrievalRoute := range routeList {
				s.metrics.PeerRequestCounter.Inc()

				go func() {
					ctx, cancel := context.WithCancel(ctx)
					defer cancel()
					chunk, err := s.retrieveChunk(ctx, retrievalRoute, rootAddr, chunkAddr, index)
					select {
					case resultC <- retrievalResult{
						chunk: chunk,
						err:   err,
					}:
					case <-ctx.Done():
					}
				}()

				select {
				case <-ticker.C:
					// continue next route
				case result := <-resultC:
					if result.err != nil {
						s.logger.Debugf("retrieval: failed to get chunk (%s,%s) from route %s: %v",
							rootAddr, chunkAddr, retrievalRoute, result.err)
					} else {
						return result.chunk, nil
					}
				case <-ctx.Done():
					s.logger.Tracef("retrieval: failed to get chunk: ctx.Done() (%s:%s): %v", rootAddr, chunkAddr, ctx.Err())
					return nil, fmt.Errorf("retrieval: %w", ctx.Err())
				}
			}
		}

		return nil, storage.ErrNotFound
	})

	if err != nil {
		return nil, err
	}
	return v.(boson.Chunk), nil
}

func (s *Service) RetrieveChunkFromNode(ctx context.Context, targetNode boson.Address, rootAddr, chunkAddr boson.Address, index int64) (boson.Chunk, error) {
	s.metrics.RequestCounter.Inc()

	if !s.relay {
		return nil, storage.ErrNotFound
	}
	flightRoute := fmt.Sprintf("%s,%s", targetNode.String(), chunkAddr.String())
	topCtx := ctx
	v, _, err := s.singleflight.Do(context.Background(), flightRoute, func(ctx context.Context) (interface{}, error) {
		// get the tracing span
		ctx = tracing.WithContext(context.Background(), tracing.FromContext(topCtx))
		span, _, ctx := s.tracer.StartSpanFromContext(ctx, "retrieve-chunk", s.logger,
			opentracing.Tag{Key: "targetNode,rootAddr,chunkAddr", Value: flightRoute})
		defer span.Finish()
		retrievalRoute := aco.NewRoute(targetNode, targetNode)
		var (
			maxRequestAttempt = 1
			requestAttempt    = 0
			resultC           = make(chan retrievalResult, maxRequestAttempt)
		)
		ticker := time.NewTicker(retrieveRetryIntervalDuration)
		defer ticker.Stop()

		for {
			requestAttempt++
			s.metrics.PeerRequestCounter.Inc()
			go func() {

				ctx, cancel := context.WithCancel(ctx)
				defer cancel()

				chunk, err := s.retrieveChunk(ctx, retrievalRoute, rootAddr, chunkAddr, index)
				select {
				case resultC <- retrievalResult{
					chunk: chunk,
					err:   err,
				}:
				case <-ctx.Done():
				}
			}()

			select {
			case <-ticker.C:
				// continu next for route
			case result := <-resultC:
				if result.err != nil {
					s.logger.Debugf("retrieval: failed to get chunk (%s,%s) from route %s: %v",
						rootAddr, chunkAddr, retrievalRoute, result.err)
				} else {
					return result.chunk, nil
				}
			case <-ctx.Done():
				s.logger.Tracef("retrieval: failed to get chunk: ctx.Done() (%s:%s): %v", rootAddr, chunkAddr, ctx.Err())
				return nil, fmt.Errorf("retrieval: %w", ctx.Err())
			}
			if requestAttempt >= maxRequestAttempt {
				return nil, storage.ErrNotFound
			}
		}
	})
	if err != nil {
		return nil, err
	}

	return v.(boson.Chunk), nil
}

func (s *Service) retrieveChunk(ctx context.Context, route aco.Route, rootAddr, chunkAddr boson.Address, index int64) (chunk boson.Chunk, err error) {

	var (
		startMs  = time.Now().UnixNano() / 1e6
		endMs    = int64(0)
		dataSize = 0
	)
	defer func() {
		if err != nil && (ctx.Err() == nil || !errors.Is(ctx.Err(), context.Canceled)) {
			go func() {
				s.logger.Tracef("retrieveChunk trigger find route")
				_, _ = s.routeTab.FindRoute(context.Background(), route.TargetNode, time.Second*5)
			}()
		}
		if err == nil || (err != nil && (ctx.Err() == nil || !errors.Is(ctx.Err(), context.Canceled))) {
			endMs = time.Now().UnixNano() / 1e6
			s.logger.Tracef("link : %s  , from  %d  to %d, chunk size: %d ", route.ToString(), startMs, endMs, dataSize)
			downloadDetail := aco.DownloadDetail{
				StartMs: startMs,
				EndMs:   endMs,
				Size:    int64(dataSize),
			}
			s.acoServer.OnDownloadFinish(route, &downloadDetail)
		}
	}()

	s.acoServer.OnDownloadStart(route)
	defer s.acoServer.OnDownloadEnd(route)

	if err = s.routeTab.Connect(ctx, route.LinkNode); err != nil {
		s.logger.Errorf("connect failed, peer: %v  err: %s", route.LinkNode.String(), err)
		return nil, fmt.Errorf("connect failed, peer: %v", route.LinkNode.String())
	}

	if err := s.accounting.Reserve(route.LinkNode, 256); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, retrieveChunkTimeout)
	defer cancel()
	stream, err := s.streamer.NewStream(ctx, route.LinkNode, nil, protocolName, protocolVersion, streamName)
	if err != nil {
		s.metrics.TotalErrors.Inc()
		s.logger.Errorf("new stream: %w", err)
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go func() {
				_ = stream.FullClose()
			}()
		}
	}()

	w, r := protobuf.NewWriterAndReader(stream)
	if err := w.WriteMsgWithContext(ctx, &pb.RequestChunk{
		TargetAddr: route.TargetNode.Bytes(),
		RootAddr:   rootAddr.Bytes(),
		ChunkAddr:  chunkAddr.Bytes(),
		Bit:        int64(index),
	}); err != nil {
		s.metrics.TotalErrors.Inc()
		return nil, fmt.Errorf("write request: %w route %v,%v", err, route.LinkNode.String(), route.TargetNode.String())
	}

	var d pb.Delivery
	if err := r.ReadMsgWithContext(ctx, &d); err != nil {
		s.metrics.TotalErrors.Inc()
		return nil, fmt.Errorf("read delivery: %w route %v,%v", err, route.LinkNode.String(), route.TargetNode.String())
	}
	s.metrics.TotalRetrieved.Inc()
	dataSize = len(d.Data)

	chunk = boson.NewChunk(chunkAddr, d.Data)
	if !cac.Valid(chunk) {
		if !soc.Valid(chunk) {
			s.metrics.InvalidChunkRetrieved.Inc()
			s.metrics.TotalErrors.Inc()
			return nil, boson.ErrInvalidChunk
		}
	}
	if err := s.accounting.Credit(context.Background(), route.LinkNode, 256); err != nil {
		return nil, err
	}

	s.logger.Tracef("retrieval: chunk %s is received", chunkAddr)
	err = s.chunkinfo.OnRetrieved(ctx, rootAddr, index, route.LinkNode)
	if err != nil {
		return nil, fmt.Errorf("retrieval: report chunk source: %v", err)
	}
	exists, err := s.storer.Put(sctx.SetRootHash(ctx, rootAddr), storage.ModePutRequest, chunk)
	if err != nil {
		return nil, fmt.Errorf("retrieval: storage put cache:%v", err)
	}
	if exists[0] {
		s.logger.Warningf("cid %s store is exists", chunkAddr.String())
	}
	return
}

func (s *Service) handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	w, r := protobuf.NewWriterAndReader(stream)
	defer func() {
		if err != nil {
			s.metrics.ChunkTransferredError.Inc()
			_ = stream.Reset()
		} else {
			go func() {
				_ = stream.FullClose()
			}()
		}
	}()
	s.logger.Tracef("handler %v recv msg from: %v", s.addr.String(), p.Address.String())

	ctx = context.WithValue(ctx, requestSourceContextKey{}, p.Address.String())

	var req pb.RequestChunk
	if err := r.ReadMsgWithContext(ctx, &req); err != nil {
		return fmt.Errorf("read request: %w peer %s", err, p.Address.String())
	}

	targetAddr, rootAddr, chunkAddr, index := boson.NewAddress(req.TargetAddr), boson.NewAddress(req.RootAddr), boson.NewAddress(req.ChunkAddr), req.Bit

	span, _, ctx := s.tracer.StartSpanFromContext(ctx, "handle-retrieve-chunk",
		s.logger, opentracing.Tag{Key: "address", Value: fmt.Sprintf("%s,%s", rootAddr, chunkAddr)},
	)
	defer span.Finish()

	chunk, err := s.storer.Get(ctx, storage.ModeGetRequest, chunkAddr, 0)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			if s.addr.Equal(targetAddr) {
				return fmt.Errorf("get from store: %w", err)
			} else {
				if chunk, err = s.RetrieveChunkFromNode(ctx, targetAddr, rootAddr, chunkAddr, index); err != nil {
					return fmt.Errorf("retrieve chunk: %w", err)
				}
			}
		} else {
			return fmt.Errorf("get from store: %w", err)
		}
	}

	if err := w.WriteMsgWithContext(ctx, &pb.Delivery{
		Data: chunk.Data(),
	}); err != nil {
		return fmt.Errorf("write delivery: %w peer %s", err, p.Address.String())
	}
	if err := s.accounting.Debit(p.Address, 256); err != nil {
		return err
	}
	if s.chunkinfo != nil && p.Mode.IsFull() {
		s.logger.Tracef("retrieval: chunk %s transfer to node %s", chunkAddr, p.Address)
		err := s.chunkinfo.OnTransferred(ctx, rootAddr, index, p.Address)
		if err != nil {
			return fmt.Errorf("retrieval: report chunk transfer: %w", err)
		}
	}

	s.metrics.TotalTransferred.Inc()
	return nil
}

func (s *Service) getRetrievalRouteList(ctx context.Context, rootAddr, chunkAddr boson.Address, bit int64) []aco.Route {

	targets, _ := sctx.GetTargets(ctx)
	routList := make([]aco.Route, 0)
	if targets != nil {
		for _, target := range targets {
			route := aco.Route{LinkNode: target, TargetNode: target}
			routList = append(routList, route)
		}
		return routList
	}
	chunkResult := s.chunkinfo.FindRoutes(ctx, rootAddr, bit)
	if len(chunkResult) == 0 {
		// return nil, fmt.Errorf("no result from chunkinfo")
		return nil
	}

	directRouteAcoIndexList := s.acoServer.GetRouteAcoIndex(chunkResult, totalRouteCount)

	for _, acoIndex := range directRouteAcoIndexList {
		curRoute := chunkResult[acoIndex]
		routList = append(routList, curRoute)
	}

	return routList
}

func (s *Service) GetRouteScore(time int64) map[string]int64 {
	return s.acoServer.GetRouteScore(time)
}
