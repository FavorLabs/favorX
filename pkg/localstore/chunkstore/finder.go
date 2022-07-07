package chunkstore

import (
	"github.com/gauss-project/aurorafs/pkg/boson"
)

func (cs *chunkStore) StartFinder(rootCid boson.Address) {
	cs.finder[rootCid.String()] = struct{}{}
}

func (cs *chunkStore) CancelFinder(rootCid boson.Address) {
	delete(cs.finder, rootCid.String())
}

func (cs *chunkStore) IsFinder(rootCid boson.Address) bool {
	_, ok := cs.finder[rootCid.String()]
	return ok
}
