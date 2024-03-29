package mock

import (
	"context"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/rpc"
	"github.com/FavorLabs/favorX/pkg/settlement/chain"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type ChainOracle struct {
	// logger logging.Logger
	// oracle *oracle.Oracle
}

func (ora *ChainOracle) API() rpc.API {
	// TODO implement me
	panic("implement me")
}

func NewServer() *ChainOracle {
	return &ChainOracle{}
}

func (ora *ChainOracle) GetCid(_ string) []byte {
	return nil
}

func (ora *ChainOracle) GetNodesFromCid(cid []byte) []boson.Address {
	overs := make([]boson.Address, 0)
	return overs
}

func (ora *ChainOracle) GetSourceNodes(_ string) []boson.Address {

	return nil
}

func (ora *ChainOracle) OnStoreMatched(cid boson.Address, dataLen uint64, salt uint64, address boson.Address) {

}

func (ora *ChainOracle) DataStoreFinished(cid boson.Address, dataLen uint64, salt uint64, proof []byte, resCh chan chain.ChainResult) {

}

func (ora *ChainOracle) RegisterCidAndNode(ctx context.Context, rootCid boson.Address, address boson.Address) (common.Hash, error) {
	return common.Hash{}, nil
}
func (ora *ChainOracle) RemoveCidAndNode(ctx context.Context, rootCid boson.Address, address boson.Address) (common.Hash, error) {
	return common.Hash{}, nil
}

func (ora *ChainOracle) GetRegisterState(ctx context.Context, rootCid boson.Address, address boson.Address) (bool, error) {
	return false, nil
}

func (ora *ChainOracle) WaitForReceipt(ctx context.Context, rootCid boson.Address, txHash common.Hash) (receipt *types.Receipt, err error) {
	return nil, nil
}
