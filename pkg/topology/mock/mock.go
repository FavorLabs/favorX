package mock

import (
	"context"
	"sync"
	"time"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/subscribe"
	"github.com/FavorLabs/favorX/pkg/topology"
	"github.com/FavorLabs/favorX/pkg/topology/model"
)

type mock struct {
	peers           []boson.Address
	depth           uint8
	closestPeer     boson.Address
	closestPeerErr  error
	peersErr        error
	addPeersErr     error
	isWithinFunc    func(c boson.Address) bool
	marshalJSONFunc func() ([]byte, error)
	mtx             sync.Mutex
}

func (d *mock) GetPeersWithLatencyEWMA(list []boson.Address) (now []boson.Address) {
	//TODO implement me
	panic("implement me")
}

func (d *mock) RefreshProtectPeer(peer []boson.Address) {
	// TODO implement me
	panic("implement me")
}

func WithPeers(peers ...boson.Address) Option {
	return optionFunc(func(d *mock) {
		d.peers = peers
	})
}

func WithAddPeersErr(err error) Option {
	return optionFunc(func(d *mock) {
		d.addPeersErr = err
	})
}

func WithNeighborhoodDepth(dd uint8) Option {
	return optionFunc(func(d *mock) {
		d.depth = dd
	})
}

func WithClosestPeer(addr boson.Address) Option {
	return optionFunc(func(d *mock) {
		d.closestPeer = addr
	})
}

func WithClosestPeerErr(err error) Option {
	return optionFunc(func(d *mock) {
		d.closestPeerErr = err
	})
}

func WithMarshalJSONFunc(f func() ([]byte, error)) Option {
	return optionFunc(func(d *mock) {
		d.marshalJSONFunc = f
	})
}

func WithIsWithinFunc(f func(boson.Address) bool) Option {
	return optionFunc(func(d *mock) {
		d.isWithinFunc = f
	})
}

func NewTopologyDriver(opts ...Option) topology.Driver {
	d := new(mock)
	for _, o := range opts {
		o.apply(d)
	}
	return d
}

func (d *mock) AddPeers(addrs ...boson.Address) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	d.peers = append(d.peers, addrs...)
}

func (d *mock) Connected(ctx context.Context, peer p2p.Peer, _ bool) error {
	d.AddPeers(peer.Address)
	return nil
}
func (d *mock) Outbound(peer p2p.Peer) {
	d.AddPeers(peer.Address)
}

func (d *mock) DisconnectForce(addr boson.Address, reason string) error {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	for i, p := range d.peers {
		if p.Equal(addr) {
			d.peers = append(d.peers[:i], d.peers[i+1:]...)
			return nil
		}
	}
	return p2p.ErrPeerNotFound
}

func (d *mock) Disconnected(peer p2p.Peer, reason string) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	for i, addr := range d.peers {
		if addr.Equal(peer.Address) {
			d.peers = append(d.peers[:i], d.peers[i+1:]...)
			break
		}
	}
}

func (d *mock) Announce(_ context.Context, _ boson.Address, _ bool) error {
	return nil
}

func (d *mock) AnnounceTo(_ context.Context, _, _ boson.Address, _ bool) error {
	return nil
}

func (d *mock) NotifyPeerState(peer p2p.PeerInfo) {

}

func (d *mock) Peers() []boson.Address {
	return d.peers
}

func (d *mock) EachKnownPeer(f model.EachPeerFunc) error {
	return nil
}

func (d *mock) EachKnownPeerRev(f model.EachPeerFunc) error {
	return nil
}

func (d *mock) ClosestPeer(addr boson.Address, wantSelf bool, _ topology.Filter, skipPeers ...boson.Address) (peerAddr boson.Address, err error) {
	if len(skipPeers) == 0 {
		if d.closestPeerErr != nil {
			return d.closestPeer, d.closestPeerErr
		}
		if !d.closestPeer.Equal(boson.ZeroAddress) {
			return d.closestPeer, nil
		}
	}

	d.mtx.Lock()
	defer d.mtx.Unlock()

	if len(d.peers) == 0 {
		return peerAddr, topology.ErrNotFound
	}

	skipPeer := false
	for _, p := range d.peers {
		for _, a := range skipPeers {
			if a.Equal(p) {
				skipPeer = true
				break
			}
		}
		if skipPeer {
			skipPeer = false
			continue
		}

		if peerAddr.IsZero() {
			peerAddr = p
		}

		if cmp, _ := boson.DistanceCmp(addr.Bytes(), p.Bytes(), peerAddr.Bytes()); cmp == 1 {
			peerAddr = p
		}
	}

	if peerAddr.IsZero() {
		if wantSelf {
			return peerAddr, topology.ErrWantSelf
		} else {
			return peerAddr, topology.ErrNotFound
		}
	}

	return peerAddr, nil
}

func (d *mock) ClosestPeers(addr boson.Address, limit int, _ topology.Filter, skipPeers ...boson.Address) ([]boson.Address, error) {
	return nil, nil
}

func (d *mock) SubscribePeersChange(notifier subscribe.INotifier) {
	return
}

func (d *mock) SubscribePeerState(notifier subscribe.INotifier) {
	return
}

func (d *mock) NeighborhoodDepth() uint8 {
	return d.depth
}

func (d *mock) IsWithinDepth(addr boson.Address) bool {
	if d.isWithinFunc != nil {
		return d.isWithinFunc(addr)
	}
	return false
}

func (d *mock) EachNeighbor(f model.EachPeerFunc) error {
	return d.EachPeer(f, topology.Filter{})
}

func (*mock) EachNeighborRev(model.EachPeerFunc) error {
	panic("not implemented") // TODO: Implement
}

// EachPeer iterates from closest bin to farthest
func (d *mock) EachPeer(f model.EachPeerFunc, _ topology.Filter) (err error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	if d.peersErr != nil {
		return d.peersErr
	}

	for i, p := range d.peers {
		_, _, err = f(p, uint8(i))
		if err != nil {
			return
		}
	}

	return nil
}

// EachPeerRev iterates from farthest bin to closest
func (d *mock) EachPeerRev(f model.EachPeerFunc, _ topology.Filter) (err error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	for i := len(d.peers) - 1; i >= 0; i-- {
		_, _, err = f(d.peers[i], uint8(i))
		if err != nil {
			return
		}
	}

	return nil
}

func (d *mock) Snapshot() *model.KadParams {
	return new(model.KadParams)
}

func (d *mock) SnapshotConnected() (connected int, peers map[string]*model.PeerInfo) {
	return
}

func (d *mock) SnapshotAddr(addr boson.Address) *model.Snapshot {
	// TODO implement me
	panic("implement me")
}

func (d *mock) RecordPeerLatency(add boson.Address, t time.Duration) {

}

func (d *mock) Halt()        {}
func (d *mock) Close() error { return nil }

type Option interface {
	apply(*mock)
}

type optionFunc func(*mock)

func (f optionFunc) apply(r *mock) { f(r) }
