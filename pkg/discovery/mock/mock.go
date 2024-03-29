package mock

import (
	"context"
	"sync"

	"github.com/FavorLabs/favorX/pkg/boson"
)

type Discovery struct {
	mtx           sync.Mutex
	ctr           int //how many ops
	records       map[string][]boson.Address
	isHive2       bool
	broadcastFunc func(context.Context, boson.Address, ...boson.Address) error
}

type Option interface {
	apply(*Discovery)
}
type optionFunc func(*Discovery)

func (f optionFunc) apply(r *Discovery) { f(r) }

func WithBroadcastPeers(f func(context.Context, boson.Address, ...boson.Address) error) optionFunc {
	return optionFunc(func(r *Discovery) {
		r.broadcastFunc = f
	})
}

func NewDiscovery(opts ...Option) *Discovery {
	d := &Discovery{
		records: make(map[string][]boson.Address),
	}
	for _, opt := range opts {
		opt.apply(d)
	}
	return d
}

func (d *Discovery) BroadcastPeers(ctx context.Context, addressee boson.Address, peers ...boson.Address) error {
	if d.broadcastFunc != nil {
		return d.broadcastFunc(ctx, addressee, peers...)
	}
	for _, peer := range peers {
		d.mtx.Lock()
		d.records[addressee.String()] = append(d.records[addressee.String()], peer)
		d.mtx.Unlock()
	}
	d.mtx.Lock()
	d.ctr++
	d.mtx.Unlock()
	return nil
}

func (d *Discovery) Broadcasts() int {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	return d.ctr
}

func (d *Discovery) AddresseeRecords(addressee boson.Address) (peers []boson.Address, exists bool) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	peers, exists = d.records[addressee.String()]
	return
}

func (d *Discovery) Reset() {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.ctr = 0
	d.records = make(map[string][]boson.Address)
}

func (d *Discovery) DoFindNode(ctx context.Context, target, peer boson.Address, pos []int32, limit int32) (res chan boson.Address, err error) {
	return
}

func (d *Discovery) IsStart() bool {
	return true
}

func (d *Discovery) IsHive2() bool {
	return d.isHive2
}

func (d *Discovery) SetHive2(is bool) {
	d.isHive2 = is
}

func (d *Discovery) NotifyDiscoverWork(peers ...boson.Address) {

}
