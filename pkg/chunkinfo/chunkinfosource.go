package chunkinfo

import (
	"github.com/FavorLabs/favorX/pkg/bitvector"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/localstore/chunkstore"
)

func (ci *ChunkInfo) updateSource(rootCid boson.Address, index, len int64, sourceOverlay boson.Address) error {
	sources, _ := ci.chunkStore.GetChunk(chunkstore.SOURCE, rootCid)
	for _, s := range sources {
		if int64(s.Len) <= index {
			continue
		}

		bv, _ := bitvector.NewFromBytes(s.B, s.Len)
		if bv.Get(int(index)) {
			return nil
		}
	}
	var provider chunkstore.Provider
	provider.Bit = int(index)
	provider.Len = int(len)
	provider.Overlay = sourceOverlay
	return ci.chunkStore.PutChunk(chunkstore.SOURCE, rootCid, []chunkstore.Provider{provider})
}
