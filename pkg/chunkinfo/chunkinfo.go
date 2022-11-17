package chunkinfo

import (
	"context"
	"fmt"
	"sync"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/fileinfo"
	"github.com/FavorLabs/favorX/pkg/localstore"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/oracle"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/retrieval/aco"
	"github.com/FavorLabs/favorX/pkg/routetab"
	"github.com/FavorLabs/favorX/pkg/rpc"
	"github.com/FavorLabs/favorX/pkg/sctx"
	"github.com/FavorLabs/favorX/pkg/subscribe"
	"resenje.org/singleflight"
)

type Interface interface {
	Discover(ctx context.Context, auth []byte, rootCid boson.Address, isOracle bool) bool

	FindRoutes(ctx context.Context, rootCid boson.Address, bit int64) []aco.Route

	OnRetrieved(ctx context.Context, rootCid boson.Address, bit int64, overlay boson.Address) error

	OnTransferred(ctx context.Context, rootCid boson.Address, bit int64, overlay boson.Address) error

	OnFileUpload(ctx context.Context, rootCid boson.Address, bitLen int64) error

	CancelFindChunkInfo(rootCid boson.Address)
}

type ChunkInfo struct {
	addr           boson.Address
	route          routetab.RouteTab
	streamer       p2p.Streamer
	logger         logging.Logger
	metrics        metrics
	singleflight   singleflight.Group
	oracleChain    oracle.Resolver
	subPub         subscribe.SubPub
	fileInfo       fileinfo.Interface
	discover       sync.Map
	queuesLk       sync.RWMutex
	queues         sync.Map // map[string]*queue
	syncMsg        sync.Map // map[string]chan bool
	timeoutTrigger *timeoutTrigger
	chunkStore     *localstore.DB
}

func New(addr boson.Address, streamer p2p.Streamer, logger logging.Logger,
	chunkStore *localstore.DB, route routetab.RouteTab, oracleChain oracle.Resolver, fileInfo fileinfo.Interface,
	subPub subscribe.SubPub) *ChunkInfo {
	chunkInfo := &ChunkInfo{
		addr:           addr,
		route:          route,
		streamer:       streamer,
		logger:         logger,
		metrics:        newMetrics(),
		oracleChain:    oracleChain,
		fileInfo:       fileInfo,
		subPub:         subPub,
		timeoutTrigger: newTimeoutTrigger(),
		chunkStore:     chunkStore,
	}
	chunkInfo.triggerTimeOut()
	// chunkInfo.cleanDiscoverTrigger()
	return chunkInfo
}

type BitVector struct {
	Len int    `json:"len"`
	B   []byte `json:"b"`
}

type BitVectorInfo struct {
	RootCid   boson.Address
	Overlay   boson.Address
	Bitvector BitVector
}

func (ci *ChunkInfo) Discover(ctx context.Context, authInfo []byte, rootCid boson.Address, chain bool) bool {
	key := fmt.Sprintf("%s%s", rootCid, "chunkinfo")
	topCtx := ctx
	v, _, _ := ci.singleflight.Do(ctx, key, func(ctx context.Context) (interface{}, error) {

		if ci.isDiscover(rootCid) {
			return true, nil
		}
		if ci.isDownload(rootCid) {
			return true, nil
		}
		overlays, _ := sctx.GetTargets(topCtx)
		if overlays == nil {
			rootCid := sctx.GetRootHash(topCtx)
			value, ok := ci.discover.Load(rootCid.String())
			if !ok && chain {
				overlays = ci.oracleChain.GetNodesFromCid(rootCid.Bytes())
			}
			if ok {
				overlays = value.([]boson.Address)
			}
			oracles, _ := sctx.GetOracle(topCtx)
			if oracles != nil {
				overlays = removeRepeatElement(overlays, oracles...)
			}
			if len(overlays) <= 0 {
				return false, nil
			}
			ci.discover.Store(rootCid.String(), overlays)
		}
		ci.CancelFindChunkInfo(rootCid)
		return ci.FindChunkInfo(context.Background(), authInfo, rootCid, overlays), nil
	})
	if v == nil {
		return false
	}
	return v.(bool)
}

func removeRepeatElement(list []boson.Address, address ...boson.Address) []boson.Address {
	addresses := make(map[string]struct{}, len(list)+len(address))
	for _, over := range list {
		addresses[over.String()] = struct{}{}
	}
	for _, over := range address {
		_, ok := addresses[over.String()]
		if !ok {
			list = append(list, over)
		}
	}
	return list
}

func (ci *ChunkInfo) FindRoutes(_ context.Context, rootCid boson.Address, index int64) []aco.Route {
	route, err := ci.getRoutes(rootCid, int(index))
	if err != nil {
		ci.logger.Errorf("chunkInfo FindRoutes:%w", err)
		return nil
	}
	return route
}

func (ci *ChunkInfo) OnTransferred(_ context.Context, rootCid boson.Address, index int64, overlay boson.Address) error {
	return ci.updateService(rootCid, index, index+1, overlay)
}

func (ci *ChunkInfo) OnRetrieved(ctx context.Context, rootCid boson.Address, index int64, overlay boson.Address) error {

	length := sctx.GetRootLen(ctx)
	if length == 0 {
		length = index + 1
	}
	err := ci.updateService(rootCid, index, length, ci.addr)
	if err != nil {
		return err
	}
	err = ci.updateSource(rootCid, index, index+1, overlay)
	if err != nil {
		return err
	}

	return nil
}

func (ci *ChunkInfo) OnFileUpload(ctx context.Context, rootCid boson.Address, length int64) error {
	for i := int64(0); i < length; i++ {
		err := ci.updateService(rootCid, i, length, ci.addr)
		if err != nil {
			return err
		}
		err = ci.updateSource(rootCid, i, length, ci.addr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ci *ChunkInfo) CancelFindChunkInfo(rootCid boson.Address) {
	ci.queues.Delete(rootCid.String())
	ci.cancelPendingFinder(rootCid)
}

func (ci *ChunkInfo) SubscribeDownloadProgress(notifier *rpc.Notifier, sub *rpc.Subscription, rootCids []boson.Address) {
	iNotifier := subscribe.NewNotifierWithDelay(notifier, sub, 1, true)
	for _, rootCid := range rootCids {
		_ = ci.subPub.Subscribe(iNotifier, "chunkInfo", "downloadProgress", rootCid.String())
	}
}

func (ci *ChunkInfo) SubscribeRetrievalProgress(notifier *rpc.Notifier, sub *rpc.Subscription, rootCid boson.Address) {
	iNotifier := subscribe.NewNotifierWithDelay(notifier, sub, 1, true)
	_ = ci.subPub.Subscribe(iNotifier, "chunkInfo", "retrievalProgress", rootCid.String())
}

func (ci *ChunkInfo) SubscribeRootCidStatus(notifier *rpc.Notifier, sub *rpc.Subscription) {
	iNotifier := subscribe.NewNotifierWithDelay(notifier, sub, 1, true)
	_ = ci.subPub.Subscribe(iNotifier, "chunkInfo", "rootCidStatus", "")
}

func (ci *ChunkInfo) PublishDownloadProgress(rootCid boson.Address, bitV BitVectorInfo) {
	_ = ci.subPub.Publish("chunkInfo", "downloadProgress", rootCid.String(), bitV)
}

func (ci *ChunkInfo) PublishRetrievalProgress(rootCid boson.Address, bitV BitVectorInfo) {
	_ = ci.subPub.Publish("chunkInfo", "retrievalProgress", rootCid.String(), bitV)
}

func (ci *ChunkInfo) PublishRootCidStatus(statusEvent RootCidStatusEven) {
	_ = ci.subPub.Publish("chunkInfo", "rootCidStatus", statusEvent.RootCid.String(), statusEvent)
}

func generateKey(keyPrefix string, rootCid, overlay boson.Address) string {
	return keyPrefix + rootCid.String() + "-" + overlay.String()
}
