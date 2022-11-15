package oracle

import (
	"context"
	"math/big"
	"sync"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/chain"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/subscribe"
)

type Resolver interface {
	// GetNodesFromCid  Get source nodes of specified cid
	GetNodesFromCid([]byte) []boson.Address
	RegisterCidAndNode(ctx context.Context, rootCid boson.Address, address boson.Address, gasPrice, minGasPrice *big.Int) (err error)
	RemoveCidAndNode(ctx context.Context, rootCid boson.Address, address boson.Address, gasPrice, minGasPrice *big.Int) (err error)
}

type ChainOracle struct {
	sync.Mutex
	logger logging.Logger
	chain  *chain.Client
	subPub subscribe.SubPub
}

func NewServer(logger logging.Logger, backend *chain.Client, subPub subscribe.SubPub) (Resolver, error) {
	return &ChainOracle{
		logger: logger,
		chain:  backend,
		subPub: subPub,
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
	return
}

func (ora *ChainOracle) RegisterCidAndNode(ctx context.Context, rootCid boson.Address, address boson.Address, gasPrice, minGasPrice *big.Int) (err error) {
	ora.Lock()
	defer ora.Unlock()

	return ora.chain.Storage.RegisterCidAndNodeWatch(ctx, rootCid.Bytes(), address.Bytes())
}

func (ora *ChainOracle) RemoveCidAndNode(ctx context.Context, rootCid boson.Address, address boson.Address, gasPrice, minGasPrice *big.Int) (err error) {
	ora.Lock()
	defer ora.Unlock()

	return ora.chain.Storage.RemoveCidAndNodeWatch(ctx, rootCid.Bytes(), address.Bytes())
}
