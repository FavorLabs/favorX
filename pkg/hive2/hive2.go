package hive2

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/FavorLabs/favorX/pkg/address"
	"github.com/FavorLabs/favorX/pkg/addressbook"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/hive2/pb"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/p2p/protobuf"
	"github.com/FavorLabs/favorX/pkg/topology"
	"github.com/FavorLabs/favorX/pkg/topology/kademlia"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"golang.org/x/sync/semaphore"
)

const (
	protocolName    = "hive2"
	protocolVersion = "1.0.0"
	streamFindNode  = "findNode"

	messageTimeout         = 1 * time.Minute // maximum allowed time for a message to be read or written.
	maxPeersLimit          = 30
	pingTimeout            = time.Second * 5 // time to wait for ping to succeed
	batchValidationTimeout = 5 * time.Minute // prevent lock contention on peer validation

)

type Service struct {
	start           bool
	streamer        p2p.StreamerPinger
	addressBook     addressbook.GetPutter
	addPeersHandler func(...boson.Address)
	networkID       uint64
	logger          logging.Logger
	metrics         metrics
	config          Config
	quit            chan struct{}
	wg              sync.WaitGroup
	peersChan       chan resultChan
	sem             *semaphore.Weighted
	findWorkC       chan []boson.Address
}

type resultChan struct {
	pb         pb.Peers
	syncResult chan boson.Address
}

type Config struct {
	Kad               *kademlia.Kad
	Base              boson.Address
	AllowPrivateCIDRs bool
}

func New(streamer p2p.StreamerPinger, addressBook addressbook.GetPutter, networkID uint64, logger logging.Logger) *Service {
	srv := &Service{
		streamer:    streamer,
		logger:      logger,
		addressBook: addressBook,
		networkID:   networkID,
		metrics:     newMetrics(),
		quit:        make(chan struct{}),
		peersChan:   make(chan resultChan),
		sem:         semaphore.NewWeighted(int64(boson.MaxPO)),
		findWorkC:   make(chan []boson.Address, 1),
	}
	srv.startCheckPeersHandler()
	return srv
}

func (s *Service) SetConfig(config Config) {
	s.config = config
}

func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    protocolName,
		Version: protocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    streamFindNode,
				Handler: s.onFindNode,
			},
		},
	}
}

func (s *Service) SetAddPeersHandler(h func(addr ...boson.Address)) {
	s.addPeersHandler = h
}

func (s *Service) Close() error {
	close(s.quit)

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		s.wg.Wait()
	}()

	select {
	case <-stopped:
		return nil
	case <-time.After(time.Second * 5):
		return errors.New("hive2: waited 5 seconds to close active goroutines")
	}
}

func (s *Service) onFindNode(ctx context.Context, peer p2p.Peer, stream p2p.Stream) (err error) {
	start := time.Now()
	defer func() {
		s.logger.Debugf("hive2: onFindNode time consuming %v", time.Since(start).Seconds())
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.FullClose()
		}
	}()
	addr, err := s.addressBook.Get(peer.Address)
	if err != nil && !errors.Is(err, addressbook.ErrNotFound) {
		return err
	}
	isPeerPublic := addr != nil && manet.IsPublicAddr(addr.Underlay)

	s.metrics.OnFindNode.Inc()
	s.logger.Tracef("hive2: onFindNode start... peer=%s", peer.Address.String())
	w, r := protobuf.NewWriterAndReader(stream)
	var req pb.FindNodeReq
	if err = r.ReadMsgWithContext(ctx, &req); err != nil {
		return fmt.Errorf("hive2: onFindNode handler read message: %w", err)
	}

	if req.Limit > maxPeersLimit {
		req.Limit = maxPeersLimit
	}
	resp := &pb.Peers{}

	target := boson.NewAddress(req.Target)
	skip := []boson.Address{peer.Address}

	var (
		limitConn  = 1
		limitKnown = 1
	)
	if req.Limit > 2 {
		limitKnown = int(req.Limit / 2)
		limitConn = int(req.Limit) - limitKnown
	}

	addrFunc := func(address boson.Address, u uint8) (stop, jumpToNext bool, err error) {
		if address.MemberOf(skip) {
			return false, false, nil
		}
		po := boson.Proximity(target.Bytes(), address.Bytes())
		if inArray(po, req.Pos) {
			p, _ := s.addressBook.Get(address)
			if p != nil {
				if !s.config.AllowPrivateCIDRs && isPeerPublic && manet.IsPrivateAddr(p.Underlay) {
					// Don't advertise private CIDRs to the public network.
					skip = append(skip, p.Overlay)
					return false, false, nil
				}
				resp.Peers = append(resp.Peers, &pb.AuroraAddress{
					Underlay:  p.Underlay.Bytes(),
					Signature: p.Signature,
					Overlay:   p.Overlay.Bytes(),
				})
			}
		}
		return false, false, nil
	}

	_ = s.config.Kad.EachPeer(addrFunc, topology.Filter{Reachable: false})
	connResult := randPeersLimit(resp.Peers, limitConn)
	for _, v := range connResult {
		skip = append(skip, boson.NewAddress(v.Overlay))
	}
	resp.Peers = nil
	_ = s.config.Kad.EachKnownPeer(addrFunc)
	knownResult := randPeersLimit(resp.Peers, limitKnown)

	resp.Peers = append(connResult, knownResult...)
	s.metrics.OnFindNodePeers.Add(float64(len(resp.Peers)))

	err = w.WriteMsgWithContext(ctx, resp)
	if err != nil {
		return fmt.Errorf("hive2: onFindNode handler write message: %w", err)
	}
	return nil
}

func (s *Service) DoFindNode(ctx context.Context, target, peer boson.Address, pos []int32, limit int32) (res chan boson.Address, err error) {
	s.metrics.DoFindNode.Inc()
	stream, err := s.streamer.NewStream(ctx, peer, nil, protocolName, protocolVersion, streamFindNode)
	if err != nil {
		s.logger.Debugf("hive2: DoFindNode NewStream %s, err=%s", peer.String(), err)
		return
	}

	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			go stream.FullClose()
		}
	}()

	ctx1, cancel := context.WithTimeout(ctx, messageTimeout)
	defer cancel()

	w, r := protobuf.NewWriterAndReader(stream)
	err = w.WriteMsgWithContext(ctx1, &pb.FindNodeReq{
		Target: target.Bytes(),
		Pos:    pos,
		Limit:  limit,
	})
	if err != nil {
		err = fmt.Errorf("hive2: DoFindNode write message: %w", err)
		return
	}

	var result pb.Peers
	if err = r.ReadMsgWithContext(ctx1, &result); err != nil {
		err = fmt.Errorf("hive2: DoFindNode read message: %w", err)
		return
	}

	s.logger.Tracef("hive2: result peers %d from %s", len(result.Peers), peer)
	s.metrics.DoFindNodePeers.Add(float64(len(result.Peers)))

	res = make(chan boson.Address, 1)
	select {
	case s.peersChan <- resultChan{
		pb:         result,
		syncResult: res,
	}:
	case <-s.quit:
		return res, errors.New("failed to process peers, shutting down hive2")
	}

	return res, nil
}

func (s *Service) startCheckPeersHandler() {
	ctx, cancel := context.WithCancel(context.Background())
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		<-s.quit
		cancel()
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case result := <-s.peersChan:
				s.wg.Add(1)
				go func() {
					defer s.wg.Done()
					cctx, cancel := context.WithTimeout(ctx, batchValidationTimeout)
					defer cancel()
					s.checkAndAddPeers(cctx, result)
				}()
			}
		}
	}()
}

func (s *Service) checkAndAddPeers(ctx context.Context, result resultChan) {

	wg := sync.WaitGroup{}

	for _, p := range result.pb.Peers {
		err := s.sem.Acquire(ctx, 1)
		if err != nil {
			return
		}

		wg.Add(1)
		go func(newPeer *pb.AuroraAddress) {
			defer func() {
				s.sem.Release(1)
				wg.Done()
			}()

			multiUnderlay, err := ma.NewMultiaddrBytes(newPeer.Underlay)
			if err != nil {
				s.logger.Errorf("hive2: multi address underlay err: %v", err)
				return
			}

			ctx, cancel := context.WithTimeout(ctx, pingTimeout)
			defer cancel()

			// check if the underlay is usable by doing a raw ping using libp2p
			if _, err = s.streamer.Ping(ctx, multiUnderlay); err != nil {
				s.metrics.UnreachablePeers.Inc()
				s.logger.Debugf("hive2: peer %s: underlay %s not reachable", hex.EncodeToString(newPeer.Overlay), multiUnderlay)
				return
			}

			addr := address.Address{
				Overlay:   boson.NewAddress(newPeer.Overlay),
				Underlay:  multiUnderlay,
				Signature: newPeer.Signature,
			}

			err = s.addressBook.Put(addr.Overlay, addr)
			if err != nil {
				s.logger.Warningf("hive2: skipping peer in response %s: %v", newPeer.String(), err)
				return
			}

			if s.addPeersHandler != nil {
				s.addPeersHandler(addr.Overlay)
			}
			<-time.After(time.Millisecond * 500)
			result.syncResult <- addr.Overlay

		}(p)
	}

	wg.Wait()

	close(result.syncResult)
}

func (s *Service) BroadcastPeers(ctx context.Context, addressee boson.Address, peers ...boson.Address) error {

	return nil
}

func (s *Service) IsStart() bool {
	return s.start
}

func (s *Service) Start() {
	go s.discover()
	s.start = true
}

func (s *Service) IsHive2() bool {
	return true
}

func randPeersLimit(peers []*pb.AuroraAddress, limit int) []*pb.AuroraAddress {
	total := len(peers)
	if total > limit {
		rand.Seed(time.Now().UnixNano())
		rand.Shuffle(total, func(i, j int) {
			peers[i], peers[j] = peers[j], peers[i]
		})
		return peers[:limit]
	}
	return peers
}
