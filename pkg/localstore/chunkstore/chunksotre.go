package chunkstore

import (
	"fmt"
	"github.com/FavorLabs/favorX/pkg/bitvector"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/storage"
	"strings"
)

type ChunkType int

const (
	DISCOVER ChunkType = iota
	SERVICE
	SOURCE
)

var TypeError = fmt.Errorf("type error")

type Interface interface {
	Init() error
	Put(chunkType ChunkType, reference boson.Address, providers []Provider) error
	Get(chunkType ChunkType, reference boson.Address) ([]Consumer, error)
	GetByOverlay(chunkType ChunkType, reference, overlay boson.Address) (Consumer, error)
	GetAll(chunkType ChunkType) (map[string][]Consumer, error)
	Remove(chunkType ChunkType, reference, overlay boson.Address) error
	RemoveAll(chunkType ChunkType, reference boson.Address) error
	Has(chunkType ChunkType, reference, overlay boson.Address) (bool, error)
	HasChunk(chunkType ChunkType, reference boson.Address, bit int) (bool, error)
	StartFinder(rootCid boson.Address)
	CancelFinder(rootCid boson.Address)
	IsFinder(rootCid boson.Address) bool
}

type Provider struct {
	Overlay boson.Address
	Bit     int
	Len     int
	B       []byte
	Time    int64
}

type Consumer struct {
	Overlay boson.Address
	Len     int
	B       []byte
	Time    int64
}

type discoverBitVector struct {
	bit  *bitvector.BitVector
	time int64
}

type BitVector struct {
	Len int
	B   []byte
}

type chunkStore struct {
	stateStore storage.StateStorer
	source     map[string]map[string]*bitvector.BitVector
	service    map[string]map[string]*bitvector.BitVector
	discover   map[string]map[string]*discoverBitVector
	finder     map[string]struct{}
}

func New(stateStore storage.StateStorer) Interface {
	return &chunkStore{
		stateStore: stateStore,
		source:     make(map[string]map[string]*bitvector.BitVector),
		service:    make(map[string]map[string]*bitvector.BitVector),
		discover:   make(map[string]map[string]*discoverBitVector),
		finder:     make(map[string]struct{}),
	}
}

func (cs *chunkStore) Init() error {
	err := cs.initService()
	if err != nil {
		return err
	}
	err = cs.initDiscover()
	if err != nil {
		return err
	}
	err = cs.initSource()
	if err != nil {
		return err
	}
	return nil
}

func (cs *chunkStore) Put(chunkType ChunkType, reference boson.Address, providers []Provider) (err error) {

	switch chunkType {
	case DISCOVER:
		for _, provider := range providers {
			err = cs.putDiscover(reference, provider.Overlay, provider.B, provider.Len)
		}
	case SOURCE:
		for _, provider := range providers {
			err = cs.putChunkSource(reference, provider.Overlay, provider.Bit, provider.Len)
		}
	case SERVICE:
		for _, provider := range providers {
			err = cs.putChunkService(reference, provider.Overlay, provider.Bit, provider.Len)
		}
	default:
		return TypeError
	}
	return err
}
func (cs *chunkStore) Get(chunkType ChunkType, reference boson.Address) ([]Consumer, error) {
	switch chunkType {
	case DISCOVER:
		d, ok := cs.getDiscover(reference)
		if !ok {
			return nil, storage.ErrNotFound
		}
		p := make([]Consumer, 0, len(d))
		for k, v := range d {
			p = append(p, Consumer{
				Overlay: boson.MustParseHexAddress(k),
				Len:     v.bit.Len(),
				B:       v.bit.Bytes(),
				Time:    v.time,
			})
		}
		return p, nil
	case SOURCE:
		d, ok := cs.getChunkSource(reference)
		if !ok {
			return nil, storage.ErrNotFound
		}
		p := make([]Consumer, 0, len(d))
		for k, v := range d {
			p = append(p, Consumer{
				Overlay: boson.MustParseHexAddress(k),
				Len:     v.Len(),
				B:       v.Bytes(),
			})
		}
		return p, nil
	case SERVICE:
		d, ok := cs.getChunkService(reference)
		if !ok {
			return nil, storage.ErrNotFound
		}
		p := make([]Consumer, 0, len(d))
		for k, v := range d {
			p = append(p, Consumer{
				Overlay: boson.MustParseHexAddress(k),
				Len:     v.Len(),
				B:       v.Bytes(),
			})
		}
		return p, nil
	default:
		return nil, TypeError
	}
}
func (cs *chunkStore) GetByOverlay(chunkType ChunkType, reference, overlay boson.Address) (Consumer, error) {
	switch chunkType {
	case DISCOVER:
		v, ok := cs.getDiscoverByOverlay(reference, overlay)
		if !ok {
			return Consumer{}, nil
		}
		return Consumer{
			Overlay: overlay,
			Len:     v.bit.Len(),
			B:       v.bit.Bytes(),
			Time:    v.time,
		}, nil
	case SOURCE:
		v, ok := cs.getChunkSourceByOverlay(reference, overlay)
		if !ok {
			return Consumer{}, nil
		}
		return Consumer{
			Overlay: overlay,
			Len:     v.Len(),
			B:       v.Bytes(),
		}, nil
	case SERVICE:
		v, ok := cs.getChunkServiceByOverlay(reference, overlay)
		if !ok {
			return Consumer{}, nil
		}
		return Consumer{
			Overlay: overlay,
			Len:     v.Len(),
			B:       v.Bytes(),
		}, nil
	default:
		return Consumer{}, TypeError
	}
}
func (cs *chunkStore) GetAll(chunkType ChunkType) (map[string][]Consumer, error) {
	switch chunkType {
	case DISCOVER:
		r := make(map[string][]Consumer)
		d := cs.getAllDiscover()
		for rootCid, node := range d {
			p := make([]Consumer, 0, len(node))
			for overlay, bv := range node {
				p = append(p, Consumer{
					Overlay: boson.MustParseHexAddress(overlay),
					Len:     bv.bit.Len(),
					B:       bv.bit.Bytes(),
					Time:    bv.time,
				})
			}
			r[rootCid] = p
		}
		return r, nil
	case SOURCE:
		r := make(map[string][]Consumer)
		d := cs.getAllChunkSource()
		for rootCid, node := range d {
			p := make([]Consumer, 0, len(node))
			for overlay, bv := range node {
				p = append(p, Consumer{
					Overlay: boson.MustParseHexAddress(overlay),
					Len:     bv.Len(),
					B:       bv.Bytes(),
				})
			}
			r[rootCid] = p
		}
		return r, nil
	case SERVICE:
		r := make(map[string][]Consumer)
		d := cs.getAllChunkService()
		for rootCid, node := range d {
			p := make([]Consumer, 0, len(node))
			for overlay, bv := range node {
				p = append(p, Consumer{
					Overlay: boson.MustParseHexAddress(overlay),
					Len:     bv.Len(),
					B:       bv.Bytes(),
				})
			}
			r[rootCid] = p
		}
		return r, nil
	default:
		return nil, TypeError
	}
}

func (cs *chunkStore) Remove(chunkType ChunkType, reference, overlay boson.Address) error {
	switch chunkType {
	case DISCOVER:
		return cs.removeDiscoverByOverlay(reference, overlay)
	case SOURCE:
		return cs.removeSourceByOverlay(reference, overlay)
	case SERVICE:
		return cs.removeServiceByOverlay(reference, overlay)
	default:
		return TypeError
	}
}

func (cs *chunkStore) RemoveAll(chunkType ChunkType, reference boson.Address) error {
	switch chunkType {
	case DISCOVER:
		return cs.removeDiscover(reference)
	case SOURCE:
		return cs.removeSource(reference)
	case SERVICE:
		return cs.removeService(reference)
	default:
		return TypeError
	}
}

func (cs *chunkStore) Has(chunkType ChunkType, reference, overlay boson.Address) (bool, error) {
	switch chunkType {
	case DISCOVER:
		return cs.hasDiscover(reference, overlay), nil
	case SOURCE:
		return cs.hasSource(reference, overlay), nil
	case SERVICE:
		return cs.hasService(reference, overlay), nil
	default:
		return false, TypeError
	}
}

func (cs *chunkStore) HasChunk(chunkType ChunkType, reference boson.Address, bit int) (bool, error) {
	switch chunkType {
	case DISCOVER:
		return cs.hasDiscoverChunk(reference, bit), nil
	case SOURCE:
		return cs.hasSourceChunk(reference, bit), nil
	case SERVICE:
		return cs.hasServiceChunk(reference, bit), nil
	default:
		return false, TypeError
	}
}

func generateKey(keyPrefix string, rootCid, overlay boson.Address) string {
	return keyPrefix + "-" + rootCid.String() + "-" + overlay.String()
}

func unmarshalKey(key string) (boson.Address, boson.Address, error) {
	keys := strings.Split(key, "-")
	rootCid, err := boson.ParseHexAddress(keys[1])
	if err != nil {
		return boson.ZeroAddress, boson.ZeroAddress, err
	}
	overlay, err := boson.ParseHexAddress(keys[2])
	if err != nil {
		return boson.ZeroAddress, boson.ZeroAddress, err
	}
	return rootCid, overlay, nil
}
