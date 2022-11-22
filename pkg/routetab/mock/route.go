package mock

import (
	"context"
	"fmt"
	"time"

	"github.com/FavorLabs/favorX/pkg/address"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/routetab"
)

type MockRouteTable struct {
	RejectAddrList []boson.Address
	NeighborMap    map[string][]boson.Address
}

func NewMockRouteTable() MockRouteTable {
	return MockRouteTable{}
}

func (r *MockRouteTable) GetRoute(ctx context.Context, target boson.Address) (routes []*routetab.Path, err error) {
	return []*routetab.Path{}, nil
}

func (r *MockRouteTable) FindRoute(ctx context.Context, target boson.Address, timeout ...time.Duration) (route []*routetab.Path, err error) {
	return []*routetab.Path{}, nil
}

func (r *MockRouteTable) Connect(ctx context.Context, target boson.Address) error {
	for _, node := range r.RejectAddrList {
		if target.Equal(node) {
			return fmt.Errorf("reject")
		}
	}
	return nil
}

func (r *MockRouteTable) GetTargetNeighbor(ctx context.Context, target boson.Address, limit int) (addresses []boson.Address, err error) {
	return r.NeighborMap[target.String()], nil
}

func (r *MockRouteTable) IsNeighbor(dest boson.Address) (has bool) {
	return false
}

func (r *MockRouteTable) IsNeighborContainLightNode(dest boson.Address) (has bool) {
	return false
}

func (r *MockRouteTable) DelRoute(ctx context.Context, dest boson.Address) (err error) {
	return nil
}

func (r *MockRouteTable) FindUnderlay(ctx context.Context, target boson.Address) (addr *address.Address, err error) {
	return nil, err
}
