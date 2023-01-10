package routetab

import (
	"context"
	"time"

	"github.com/FavorLabs/favorX/pkg/address"
	"github.com/FavorLabs/favorX/pkg/boson"
)

type RouteTab interface {
	GetRoute(ctx context.Context, dest boson.Address) (paths []*Path, err error)
	FindRoute(ctx context.Context, dest boson.Address, timeout ...time.Duration) (paths []*Path, err error)
	DelRoute(ctx context.Context, dest boson.Address) (err error)
	Connect(ctx context.Context, dest boson.Address) error
	GetTargetNeighbor(ctx context.Context, dest boson.Address, limit int) (addresses []boson.Address, err error)
	IsNeighbor(dest boson.Address) (has bool)
	IsNeighborContainLightNode(dest boson.Address) (has bool)
	FindUnderlay(ctx context.Context, target boson.Address) (addr *address.Address, err error)
}

type RelayStream interface {
	GetNextHopRandomOrFind(ctx context.Context, target boson.Address, skips ...boson.Address) (next boson.Address, err error)
}
