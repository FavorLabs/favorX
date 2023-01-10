package peerset

import (
	"sync"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/topology/model"
)

type PeersManager struct {
	peers          []boson.Address // the slice of peers
	mu             sync.RWMutex
	chanNotifyFunc chan func()
}

func New(chanNotifyFunc chan func()) *PeersManager {
	return &PeersManager{
		peers:          make([]boson.Address, 0),
		chanNotifyFunc: chanNotifyFunc,
	}
}

func (s *PeersManager) AddIfNotExists(addr boson.Address) (existed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if addr.MemberOf(s.peers) {
		return true
	}

	s.peers = append(s.peers, addr)
	return false
}

func (s *PeersManager) AddAndNotify(addr boson.Address, notify func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if addr.MemberOf(s.peers) {
		return
	}

	s.peers = append(s.peers, addr)
	s.chanNotifyFunc <- notify
}

func (s *PeersManager) Add(addr boson.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if addr.MemberOf(s.peers) {
		return
	}

	s.peers = append(s.peers, addr)
}

func (s *PeersManager) Length() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.peers)
}

func (s *PeersManager) Exists(addr boson.Address) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return addr.MemberOf(s.peers)
}

func (s *PeersManager) RemoveFirst() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.peers = s.peers[1:]
}

func (s *PeersManager) RemoveAndNotify(addr boson.Address, notify func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var now []boson.Address
	for _, v := range s.peers {
		if v.Equal(addr) {
			s.chanNotifyFunc <- notify
		} else {
			now = append(now, v)
		}
	}
	s.peers = now
}

func (s *PeersManager) Remove(addr boson.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var now []boson.Address
	for _, v := range s.peers {
		if !v.Equal(addr) {
			now = append(now, v)
		}
	}
	s.peers = now
}

func (s *PeersManager) Peers() (list []boson.Address) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list = s.peers
	return
}

func (s *PeersManager) Iter(fn func(addr boson.Address)) {
	for _, v := range s.Peers() {
		fn(v)
	}
}

func (s *PeersManager) EachBin(pf model.EachPeerFunc) error {
	for _, peer := range s.Peers() {
		stop, next, err := pf(peer, uint8(0))
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
		if next {
			break
		}
	}

	return nil
}
