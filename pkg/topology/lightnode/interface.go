package lightnode

import (
	"context"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/topology/model"
)

const (
	DefaultLightNodeLimit = 100
)

type LightNodes interface {
	Connected(context.Context, p2p.Peer)
	Disconnected(p2p.Peer)
	Count() int
	RandomPeer(boson.Address) (boson.Address, error)
	EachPeer(pf model.EachPeerFunc) error
}
