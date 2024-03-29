package pslice

import (
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/topology/model"
	"sync"
)

// PSlice maintains a list of addresses, indexing them by their different proximity orders.
type PSlice struct {
	peers     [][]boson.Address // the slice of peers
	baseBytes []byte
	mu        sync.RWMutex
	maxBins   int
}

// New creates a new PSlice.
func New(maxBins int, base boson.Address) *PSlice {
	return &PSlice{
		peers:     make([][]boson.Address, maxBins),
		baseBytes: base.Bytes(),
		maxBins:   maxBins,
	}
}

// Add a peer at a certain PO.
func (s *PSlice) Add(addrs ...boson.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// bypass unnecessary allocations below if address count is one
	if len(addrs) == 1 {
		addr := addrs[0]
		po := s.po(addr.Bytes())
		if e, _ := s.index(addr, po); e {
			return
		}
		s.peers[po] = append(s.peers[po], addr)
		return
	}

	addrPo := make([]uint8, 0, len(addrs))
	binChange := make([]int, s.maxBins)
	exists := make([]bool, len(addrs))

	for i, addr := range addrs {
		po := s.po(addr.Bytes())
		addrPo = append(addrPo, po)
		if e, _ := s.index(addr, po); e {
			exists[i] = true
		} else {
			binChange[po]++
		}
	}

	for i, count := range binChange {
		peers := s.peers[i]
		if count > 0 && cap(peers) < len(peers)+count {
			newPeers := make([]boson.Address, len(peers), len(peers)+count)
			copy(newPeers, peers)
			s.peers[i] = newPeers
		}
	}

	for i, addr := range addrs {
		if exists[i] {
			continue
		}

		po := addrPo[i]
		s.peers[po] = append(s.peers[po], addr)
	}
}

// iterates over all peers from deepest bin to shallowest.
func (s *PSlice) EachBin(pf model.EachPeerFunc) error {

	for i := s.maxBins - 1; i >= 0; i-- {

		s.mu.RLock()
		peers := s.peers[i]
		s.mu.RUnlock()

		for _, peer := range peers {
			stop, next, err := pf(peer, uint8(i))
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
	}

	return nil
}

// EachBinRev iterates over all peers from shallowest bin to deepest.
func (s *PSlice) EachBinRev(pf model.EachPeerFunc) error {

	for i := 0; i < s.maxBins; i++ {

		s.mu.RLock()
		peers := s.peers[i]
		s.mu.RUnlock()

		for _, peer := range peers {

			stop, next, err := pf(peer, uint8(i))
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
	}
	return nil
}

func (s *PSlice) BinSize(bin uint8) int {

	if int(bin) >= s.maxBins {
		return 0
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.peers[bin])
}

func (s *PSlice) BinPeers(bin uint8) []boson.Address {

	if int(bin) >= s.maxBins {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	ret := make([]boson.Address, len(s.peers[bin]))
	copy(ret, s.peers[bin])

	return ret
}

// Length returns the number of peers in the Pslice.
func (s *PSlice) Length() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var ret int

	for _, peers := range s.peers {
		ret += len(peers)
	}

	return ret
}

// ShallowestEmpty returns the shallowest empty bin if one exists.
// If such bin does not exists, returns true as bool value.
func (s *PSlice) ShallowestEmpty() (uint8, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i, peers := range s.peers {

		if len(peers) == 0 {
			return uint8(i), false
		}

	}

	return 0, true
}

// Exists checks if a peer exists.
func (s *PSlice) Exists(addr boson.Address) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, _ := s.index(addr, s.po(addr.Bytes()))
	return e
}

// Remove a peer at a certain PO.
func (s *PSlice) Remove(addr boson.Address) {
	s.mu.Lock()
	defer s.mu.Unlock()

	po := s.po(addr.Bytes())

	e, i := s.index(addr, po)
	if !e {
		return
	}

	// Since order of elements does not matter, the optimized removing process
	// below replaces the index to be removed with the last element of the array,
	// and shortens the array by one.

	// make copy of the bin slice with one fewer element
	newLength := len(s.peers[po]) - 1
	cpy := make([]boson.Address, newLength)
	copy(cpy, s.peers[po][:newLength])

	// if the index is the last element, then assign slice and return early
	if i == newLength {
		s.peers[po] = cpy
		return
	}

	// replace index being removed with last element
	lastItem := s.peers[po][newLength]
	cpy[i] = lastItem

	// assign the copy with the index removed back to the original array
	s.peers[po] = cpy
}

func (s *PSlice) po(peer []byte) uint8 {
	po := boson.Proximity(s.baseBytes, peer)
	if int(po) >= s.maxBins {
		return uint8(s.maxBins) - 1
	}
	return po
}

// index returns if a peer exists and the index in the slice.
func (s *PSlice) index(addr boson.Address, po uint8) (bool, int) {

	for i, peer := range s.peers[po] {
		if peer.Equal(addr) {
			return true, i
		}
	}

	return false, 0
}
