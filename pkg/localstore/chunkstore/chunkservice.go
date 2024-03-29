package chunkstore

import (
	"github.com/FavorLabs/favorX/pkg/bitvector"
	"github.com/FavorLabs/favorX/pkg/boson"
	"strings"
)

var keyPrefix = "chunk"

func (cs *chunkStore) initService() error {
	if err := cs.stateStore.Iterate(keyPrefix, func(k, v []byte) (bool, error) {
		if !strings.HasPrefix(string(k), keyPrefix) {
			return true, nil
		}
		key := string(k)
		rootCid, overlay, err := unmarshalKey(key)
		if err != nil {
			return false, err
		}
		var vb BitVector
		if err := cs.stateStore.Get(key, &vb); err != nil {
			return false, err
		}
		cs.putInitService(rootCid, overlay, vb.B, vb.Len)
		return false, nil
	}); err != nil {
		return err
	}
	return nil
}

func (cs *chunkStore) getChunkService(rootCid boson.Address) (map[string]*bitvector.BitVector, bool) {
	r := rootCid.String()
	v, ok := cs.service[r]
	return v, ok
}
func (cs *chunkStore) getChunkServiceByOverlay(rootCid, overlay boson.Address) (*bitvector.BitVector, bool) {
	r := rootCid.String()
	o := overlay.String()
	v, ok := cs.service[r][o]
	return v, ok
}

func (cs *chunkStore) getAllChunkService() map[string]map[string]*bitvector.BitVector {
	return cs.service
}

func (cs *chunkStore) putInitService(rootCid, overlay boson.Address, b []byte, len int) {
	if cs.hasService(rootCid, overlay) {
		return
	}
	r := rootCid.String()
	o := overlay.String()
	v, ok := cs.service[r]
	if !ok {
		v = make(map[string]*bitvector.BitVector)
	}
	bv, err := bitvector.NewFromBytes(b, len)
	if err != nil {
		return
	}
	v[o] = bv
	cs.service[r] = v
}

func (cs *chunkStore) putChunkService(rootCid, overlay boson.Address, bit, len int) error {
	r := rootCid.String()
	o := overlay.String()
	v, ok := cs.service[r]
	if !ok {
		v = make(map[string]*bitvector.BitVector)
	}
	bv, ok := v[o]
	if ok {
		if bv.Len() < len {
			newbv, err := bitvector.New(len)
			if err != nil {
				return err
			}
			err = newbv.SetBytes(bv.Bytes())
			if err != nil {
				return err
			}
			bv = newbv
		} else if bv.Get(bit) {
			return nil
		}
	} else {
		newbv, err := bitvector.New(len)
		if err != nil {
			return err
		}
		bv = newbv
	}
	bv.Set(bit)
	v[o] = bv
	cs.service[r] = v
	return cs.stateStore.Put(generateKey(keyPrefix, rootCid, overlay), &BitVector{B: bv.Bytes(), Len: bv.Len()})
}

func (cs *chunkStore) removeService(rootCid boson.Address) error {
	r := rootCid.String()
	if v, ok := cs.service[r]; ok {
		for k := range v {
			err := cs.stateStore.Delete(generateKey(keyPrefix, rootCid, boson.MustParseHexAddress(k)))
			if err != nil {
				return err
			}
		}
		delete(cs.service, r)
	}

	return nil
}

func (cs *chunkStore) removeServiceByOverlay(rootCid, overlay boson.Address) error {
	r := rootCid.String()
	o := overlay.String()
	if _, ok := cs.service[r][o]; ok {
		err := cs.stateStore.Delete(generateKey(keyPrefix, rootCid, overlay))
		if err != nil {
			return err
		}
		delete(cs.service[r], o)
	}
	return nil
}

func (cs *chunkStore) hasService(rootCid, overlay boson.Address) bool {
	r := rootCid.String()
	o := overlay.String()
	_, ok := cs.service[r][o]
	return ok
}

func (cs *chunkStore) hasServiceChunk(rootCid boson.Address, bit int) bool {
	r := rootCid.String()
	s, ok := cs.service[r]
	if !ok {
		return ok
	}
	for _, v := range s {
		if v.Len() > bit {
			if v.Get(bit) {
				return true
			}
		}
	}
	return false
}
