package libp2p

import (
	"bytes"
	"context"
	"sort"
	"sync"

	"github.com/FavorLabs/favorX/pkg/address"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/libp2p/go-libp2p/core/network"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

type peerRegistry struct {
	underlays   map[string]libp2ppeer.ID                    // map overlay address to underlay peer id
	overlays    map[libp2ppeer.ID]boson.Address             // map underlay peer id to overlay address
	modes       map[libp2ppeer.ID]address.Model             // map to track whether a node mode
	connections map[libp2ppeer.ID]map[network.Conn]struct{} // list of connections for safe removal on Disconnect notification
	streams     map[libp2ppeer.ID]map[network.Stream]context.CancelFunc
	mu          sync.RWMutex

	disconnecter     disconnecter // peerRegistry notifies libp2p on peer disconnection
	network.Notifiee              // peerRegistry can be the receiver for network.Notify
}

type disconnecter interface {
	disconnected(peer p2p.Peer)
}

func newPeerRegistry() *peerRegistry {
	return &peerRegistry{
		underlays:   make(map[string]libp2ppeer.ID),
		overlays:    make(map[libp2ppeer.ID]boson.Address),
		modes:       make(map[libp2ppeer.ID]address.Model),
		connections: make(map[libp2ppeer.ID]map[network.Conn]struct{}),
		streams:     make(map[libp2ppeer.ID]map[network.Stream]context.CancelFunc),

		Notifiee: new(network.NoopNotifiee),
	}
}

func (r *peerRegistry) Exists(overlay boson.Address) (found bool) {
	_, found = r.peerID(overlay)
	return found
}

// Disconnected Disconnect removes the peer from registry in disconnect.
// peerRegistry has to be set by network.Network.Notify().
func (r *peerRegistry) Disconnected(_ network.Network, c network.Conn) {
	peerID := c.RemotePeer()

	r.mu.Lock()

	// remove only the related connection,
	// not eventusally newly created one for the same peer
	if _, ok := r.connections[peerID][c]; !ok {
		r.mu.Unlock()
		return
	}

	// if there are multiple libp2p connections, consider the node disconnected only when the last connection is disconnected
	delete(r.connections[peerID], c)
	if len(r.connections[peerID]) > 0 {
		r.mu.Unlock()
		return
	}

	delete(r.connections, peerID)
	overlay := r.overlays[peerID]
	delete(r.overlays, peerID)
	delete(r.underlays, overlay.ByteString())
	for _, cancel := range r.streams[peerID] {
		cancel()
	}
	delete(r.streams, peerID)
	mode := r.modes[peerID]
	delete(r.modes, peerID)
	r.mu.Unlock()
	r.disconnecter.disconnected(p2p.Peer{
		Address: overlay,
		Mode:    mode,
	})

}

func (r *peerRegistry) addStream(peerID libp2ppeer.ID, stream network.Stream, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.streams[peerID]; !ok {
		// it is possible that an addStream will be called after a disconnect
		return
	}
	r.streams[peerID][stream] = cancel
}

func (r *peerRegistry) removeStream(peerID libp2ppeer.ID, stream network.Stream) {
	r.mu.Lock()
	defer r.mu.Unlock()

	peer, ok := r.streams[peerID]
	if !ok {
		return
	}

	cancel, ok := peer[stream]
	if !ok {
		return
	}

	cancel()

	delete(r.streams[peerID], stream)
}

func (r *peerRegistry) peers() []p2p.Peer {
	r.mu.RLock()
	peers := make([]p2p.Peer, 0, len(r.overlays))
	for p, a := range r.overlays {
		peers = append(peers, p2p.Peer{
			Address: a,
			Mode:    r.modes[p],
		})
	}
	r.mu.RUnlock()
	sort.Slice(peers, func(i, j int) bool {
		return bytes.Compare(peers[i].Address.Bytes(), peers[j].Address.Bytes()) == -1
	})
	return peers
}

func (r *peerRegistry) addIfNotExists(c network.Conn, overlay boson.Address, md address.Model) (exists bool) {
	peerID := c.RemotePeer()
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.connections[peerID]; !ok {
		r.connections[peerID] = make(map[network.Conn]struct{})
	}
	// the connection is added even if the peer already exists in peer registry
	// this is solving a case of multiple underlying libp2p connections for the same peer
	r.connections[peerID][c] = struct{}{}

	if _, exists := r.underlays[overlay.ByteString()]; exists {
		return true
	}

	r.streams[peerID] = make(map[network.Stream]context.CancelFunc)
	r.underlays[overlay.ByteString()] = peerID
	r.overlays[peerID] = overlay
	r.modes[peerID] = md
	return false

}

func (r *peerRegistry) peerID(overlay boson.Address) (peerID libp2ppeer.ID, found bool) {
	r.mu.RLock()
	peerID, found = r.underlays[overlay.ByteString()]
	r.mu.RUnlock()
	return peerID, found
}

func (r *peerRegistry) overlay(peerID libp2ppeer.ID) (boson.Address, bool) {
	r.mu.RLock()
	overlay, found := r.overlays[peerID]
	r.mu.RUnlock()
	return overlay, found
}

func (r *peerRegistry) mode(peerID libp2ppeer.ID) (address.Model, bool) {
	r.mu.RLock()
	md, found := r.modes[peerID]
	r.mu.RUnlock()
	return md, found
}

func (r *peerRegistry) isConnected(peerID libp2ppeer.ID, remoteAddr ma.Multiaddr) (*p2p.Peer, bool) {
	if remoteAddr == nil {
		return nil, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	overlay, found := r.overlays[peerID]
	if !found {
		return nil, false
	}

	// check connection remote address
	conns, ok := r.connections[peerID]
	if !ok {
		return nil, false
	}

	for c := range conns {
		if c.RemoteMultiaddr().Equal(remoteAddr) {
			// we ARE connected to the peer on expected address
			return &p2p.Peer{
				Address: overlay,
				Mode:    r.modes[peerID],
			}, true
		}
	}

	return nil, false
}

func (r *peerRegistry) remove(overlay boson.Address) (found bool, md address.Model, peerID libp2ppeer.ID) {
	r.mu.Lock()
	peerID, found = r.underlays[overlay.ByteString()]
	delete(r.overlays, peerID)
	delete(r.underlays, overlay.ByteString())
	delete(r.connections, peerID)
	for _, cancel := range r.streams[peerID] {
		cancel()
	}
	delete(r.streams, peerID)
	md = r.modes[peerID]
	delete(r.modes, peerID)
	r.mu.Unlock()

	return found, md, peerID
}

func (r *peerRegistry) setDisconnecter(d disconnecter) {
	r.disconnecter = d
}
