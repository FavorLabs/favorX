package mock

import (
	"context"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/rpc"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

type Service struct {
}

func NewServer() *Service {
	return &Service{}
}

func (s *Service) GetNodesFromCid([]byte) []boson.Address {
	return nil
}

func (s *Service) Register(rootCid boson.Address) error {
	return nil
}

func (s *Service) Remove(ctx context.Context, rootCid boson.Address, address boson.Address) (txn types.Hash, err error) {
	return types.Hash{}, err
}

func (s *Service) API() rpc.API {
	return rpc.API{}
}
