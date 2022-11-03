package storage

import (
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/chain"
)

type Interface interface {
	GetNodesFromCid(cid []byte) []boson.Address
}

// storage exposes methods for querying storage
type storage struct {
	client *chain.SubstrateAPI
}

// New creates a new oracle struct
func New(c *chain.SubstrateAPI) Interface {
	return &storage{client: c}
}

func (s *storage) GetNodesFromCid(cid []byte) []boson.Address {
	return nil

}
