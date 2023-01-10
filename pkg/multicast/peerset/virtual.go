package peerset

import (
	"sync"
	"time"

	"github.com/FavorLabs/favorX/pkg/boson"
	model2 "github.com/FavorLabs/favorX/pkg/multicast/model"
	"github.com/FavorLabs/favorX/pkg/topology/model"
)

const EWMASmoothing = 0.1

type VirtualState int

const (
	Shaking VirtualState = iota
	ShakeDone
)

type Virtual struct {
	Mix          sync.RWMutex
	State        VirtualState
	RelatedGIDs  []boson.Address
	LastSeenTime time.Time
	MinTimeout   int64 // seconds
	LatencyEWMA  time.Duration
	Direction    model.PeerConnectionDirection
}

type VirtualManager struct {
	peers map[string]*Virtual
	mu    sync.RWMutex
}

func NewVirtual() *VirtualManager {
	return &VirtualManager{
		peers: make(map[string]*Virtual),
	}
}

func (s *VirtualManager) Add(addr boson.Address, dir model.PeerConnectionDirection) (obj *Virtual, shaking bool) {
	s.mu.Lock()

	v, ok := s.peers[addr.ByteString()]
	if !ok {
		v = &Virtual{Direction: dir}
		s.peers[addr.ByteString()] = v
		s.mu.Unlock()
	} else {
		s.mu.Unlock()
		v.Mix.Lock()
		if v.State == Shaking {
			v.Mix.Unlock()
			return v, true
		}
		v.State = Shaking
		v.Mix.Unlock()
	}
	return v, false
}

func (s *VirtualManager) HandshakeDone(addr boson.Address, timeout int64, latency time.Duration) {
	s.mu.RLock()
	v, ok := s.peers[addr.ByteString()]
	s.mu.RUnlock()
	if ok {
		v.Mix.Lock()
		v.LastSeenTime = time.Now()
		v.State = ShakeDone
		if v.Direction == model.PeerConnectionDirectionInbound && timeout > 0 && ((timeout < v.MinTimeout) || (v.MinTimeout == 0)) {
			v.MinTimeout = timeout
		}
		if v.LatencyEWMA == 0 {
			v.LatencyEWMA = latency
		} else {
			t := (EWMASmoothing * float64(latency)) + (1-EWMASmoothing)*float64(v.LatencyEWMA)
			v.LatencyEWMA = time.Duration(t)
		}
		v.Mix.Unlock()
	}
}

func (s *VirtualManager) Clean(addr boson.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.peers, addr.ByteString())
}

func (s *VirtualManager) RemoveGroup(addr, gid boson.Address, whenInbound func()) {
	s.mu.RLock()

	v, ok := s.peers[addr.ByteString()]
	s.mu.RUnlock()
	if ok {
		s.mu.Lock()
		defer s.mu.Unlock()
		v.Mix.Lock()
		var now []boson.Address
		for _, vv := range v.RelatedGIDs {
			if !vv.Equal(gid) {
				now = append(now, vv)
			} else {
				go whenInbound()
			}
		}
		if len(now) > 0 {
			v.RelatedGIDs = now
			v.Mix.Unlock()
			return
		}
		v.Mix.Unlock()
		delete(s.peers, addr.ByteString())
	}
}

func (s *VirtualManager) BindGroup(addr, gid boson.Address, whenInbound func()) {
	s.mu.RLock()

	v, ok := s.peers[addr.ByteString()]
	s.mu.RUnlock()
	if ok {
		v.Mix.Lock()
		if !gid.MemberOf(v.RelatedGIDs) {
			v.RelatedGIDs = append(v.RelatedGIDs, gid)
		}
		v.Mix.Unlock()
		if v.Direction == model.PeerConnectionDirectionInbound {
			whenInbound()
		}
	}
}

func (s *VirtualManager) GetInfo(addr boson.Address) *Virtual {
	s.mu.RLock()
	v, ok := s.peers[addr.ByteString()]
	s.mu.RUnlock()
	if ok {
		return v
	}
	return nil
}

func (s *VirtualManager) SnapshotAddr(addr boson.Address) model2.SnapshotView {
	s.mu.RLock()
	res, ok := s.peers[addr.ByteString()]
	s.mu.RUnlock()
	if ok {
		return model2.SnapshotView{
			Timeout: res.MinTimeout,
			LastSeenTimestamp: func() int64 {
				if res.LastSeenTime.IsZero() {
					return 0
				} else {
					return res.LastSeenTime.Unix()
				}
			}(),
			SessionConnectionDirection: res.Direction,
			LatencyEWMA:                res.LatencyEWMA.Milliseconds(),
		}
	}
	return model2.SnapshotView{}
}

func (s *VirtualManager) SnapshotNow() map[string]model2.SnapshotView {
	s.mu.RLock()
	res := s.peers
	s.mu.RUnlock()
	out := make(map[string]model2.SnapshotView)
	for k, v := range res {
		out[k] = model2.SnapshotView{
			Timeout: v.MinTimeout,
			LastSeenTimestamp: func() int64 {
				if v.LastSeenTime.IsZero() {
					return 0
				} else {
					return v.LastSeenTime.Unix()
				}
			}(),
			SessionConnectionDirection: v.Direction,
			LatencyEWMA:                v.LatencyEWMA.Milliseconds(),
		}
	}
	return out
}
