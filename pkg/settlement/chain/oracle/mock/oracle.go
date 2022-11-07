package mock

import (
	"context"
	"math/big"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/settlement/chain/oracle"
)

type ChainOracle struct {
	oracle *oracle.Resolver
}

func (ora *ChainOracle) RegisterCidAndNode(ctx context.Context, rootCid boson.Address, address boson.Address, gasPrice, minGasPrice *big.Int) (err error) {
	// TODO implement me
	panic("implement me")
}

func (ora *ChainOracle) RemoveCidAndNode(ctx context.Context, rootCid boson.Address, address boson.Address, gasPrice, minGasPrice *big.Int) (err error) {
	// TODO implement me
	panic("implement me")
}

func NewServer() *ChainOracle {
	return &ChainOracle{}
}

func (ora *ChainOracle) GetNodesFromCid(cid []byte) []boson.Address {
	overs := make([]boson.Address, 0)
	return overs
}
