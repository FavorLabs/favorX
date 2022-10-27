package chunkinfo

import (
	"github.com/FavorLabs/favorX/pkg/boson"
)

// updatePendingFinder
func (ci *ChunkInfo) updatePendingFinder(rootCid boson.Address) {
	ci.chunkStore.StartFinder(rootCid)
}

// cancelPendingFinder
func (ci *ChunkInfo) cancelPendingFinder(rootCid boson.Address) {
	ci.chunkStore.CancelFinder(rootCid)
}

// getPendingFinder
func (ci *ChunkInfo) getPendingFinder(rootCid boson.Address) bool {
	return ci.chunkStore.IsFinder(rootCid)
}
