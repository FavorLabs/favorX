package oracle

import (
	"context"
	"sync"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/chain"
	"github.com/FavorLabs/favorX/pkg/fileinfo"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/rpc"
	"github.com/FavorLabs/favorX/pkg/subscribe"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

type Resolver interface {
	GetNodesFromCid([]byte) []boson.Address
	Register(rootCid boson.Address) error
	Remove(ctx context.Context, rootCid boson.Address, address boson.Address) (txn types.Hash, err error)
	API() rpc.API
}

type ChainOracle struct {
	sync.Mutex
	logger   logging.Logger
	chain    *chain.Client
	subPub   subscribe.SubPub
	fileInfo fileinfo.Interface
}

func NewServer(logger logging.Logger, backend *chain.Client, subPub subscribe.SubPub, fileInfo fileinfo.Interface) (Resolver, error) {
	return &ChainOracle{
		logger:   logger,
		chain:    backend,
		subPub:   subPub,
		fileInfo: fileInfo,
	}, nil
}

func (ora *ChainOracle) GetNodesFromCid(cid []byte) (res []boson.Address) {
	list, err := ora.chain.Storage.GetNodesFromCid(cid)
	if err == nil {
		for k := range list {
			addr := boson.NewAddress(list[k].ToBytes())
			res = append(res, addr)
		}
	}
	if len(res) > 0 {
		_ = ora.fileInfo.RegisterFile(boson.NewAddress(cid), true)
	}
	return
}

func (ora *ChainOracle) Register(rootCid boson.Address) error {
	ora.Lock()
	defer ora.Unlock()
	ora.PublishRegisterStatus(rootCid, true)
	return ora.fileInfo.RegisterFile(rootCid, true)
}

func (ora *ChainOracle) Remove(ctx context.Context, rootCid boson.Address, address boson.Address) (txn types.Hash, err error) {
	ora.Lock()
	defer ora.Unlock()

	return ora.chain.Storage.RemoveCidAndNode(ctx, rootCid.Bytes(), address.Bytes(), func(block types.Hash, txn types.Hash) {
		ora.PublishRegisterStatus(rootCid, false)
	})
}

func (ora *ChainOracle) SubscribeRegisterStatus(notifier *rpc.Notifier, sub *rpc.Subscription, rootCids []boson.Address) {
	iNotifier := subscribe.NewNotifier(notifier, sub)
	for i := 0; i < len(rootCids); i++ {
		_ = ora.subPub.Subscribe(iNotifier, "oracle", "registerStatus", rootCids[i].String())
	}
}

type RegisterStatus struct {
	RootCid boson.Address `json:"rootCid"`
	Status  bool          `json:"status"`
}

func (ora *ChainOracle) PublishRegisterStatus(rootCid boson.Address, status bool) {
	_ = ora.subPub.Publish("oracle", "registerStatus", rootCid.String(), RegisterStatus{
		RootCid: rootCid,
		Status:  status,
	})
}
