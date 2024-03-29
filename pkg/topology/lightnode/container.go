package lightnode

import (
	"context"
	"crypto/rand"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/topology/model"
	"github.com/FavorLabs/favorX/pkg/topology/pslice"
	"math/big"
	"sync"
)

type Container struct {
	base              boson.Address
	peerMu            sync.Mutex // peerMu guards connectedPeers and disconnectedPeers.
	connectedPeers    *pslice.PSlice
	disconnectedPeers *pslice.PSlice
	metrics           metrics
}

func NewContainer(base boson.Address) *Container {
	return &Container{
		base:              base,
		connectedPeers:    pslice.New(1, base),
		disconnectedPeers: pslice.New(1, base),
		metrics:           newMetrics(),
	}
}

func (c *Container) Connected(ctx context.Context, peer p2p.Peer) {
	c.peerMu.Lock()
	defer c.peerMu.Unlock()

	addr := peer.Address
	c.connectedPeers.Add(addr)
	c.disconnectedPeers.Remove(addr)

	c.metrics.CurrentlyConnectedPeers.Set(float64(c.connectedPeers.Length()))
	c.metrics.CurrentlyDisconnectedPeers.Set(float64(c.disconnectedPeers.Length()))
}

func (c *Container) Disconnected(peer p2p.Peer) {
	c.peerMu.Lock()
	defer c.peerMu.Unlock()

	addr := peer.Address
	if found := c.connectedPeers.Exists(addr); found {
		c.connectedPeers.Remove(addr)
		c.disconnectedPeers.Add(addr)
	}

	c.metrics.CurrentlyConnectedPeers.Set(float64(c.connectedPeers.Length()))
	c.metrics.CurrentlyDisconnectedPeers.Set(float64(c.disconnectedPeers.Length()))
}

func (c *Container) Count() int {
	return c.connectedPeers.Length()
}

func (c *Container) RandomPeer(not boson.Address) (boson.Address, error) {
	c.peerMu.Lock()
	defer c.peerMu.Unlock()
	var (
		cnt   = big.NewInt(int64(c.Count()))
		addr  = boson.ZeroAddress
		count = int64(0)
	)

PICKPEER:
	i, e := rand.Int(rand.Reader, cnt)
	if e != nil {
		return boson.ZeroAddress, e
	}
	i64 := i.Int64()

	count = 0
	_ = c.connectedPeers.EachBinRev(func(peer boson.Address, _ uint8) (bool, bool, error) {
		if count == i64 {
			addr = peer
			return true, false, nil
		}
		count++
		return false, false, nil
	})

	if addr.Equal(not) {
		goto PICKPEER
	}

	return addr, nil
}

func (c *Container) EachPeer(pf model.EachPeerFunc) error {
	return c.connectedPeers.EachBin(pf)
}

func (c *Container) PeerInfo() model.BinInfo {
	return model.BinInfo{
		BinPopulation:     uint(c.connectedPeers.Length()),
		BinConnected:      uint(c.connectedPeers.Length()),
		DisconnectedPeers: peersInfo(c.disconnectedPeers),
		ConnectedPeers:    peersInfo(c.connectedPeers),
	}
}

func peersInfo(s *pslice.PSlice) []*model.PeerInfo {
	if s.Length() == 0 {
		return nil
	}
	peers := make([]*model.PeerInfo, 0, s.Length())
	_ = s.EachBin(func(addr boson.Address, po uint8) (bool, bool, error) {
		peers = append(peers, &model.PeerInfo{Address: addr})
		return false, false, nil
	})
	return peers
}
