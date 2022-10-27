package chunkinfo

import (
	"github.com/FavorLabs/favorX/pkg/bitvector"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/localstore/chunkstore"
	"github.com/FavorLabs/favorX/pkg/storage"
)

type RootCidStatusEven struct {
	RootCid boson.Address
	Status  RootCidStatus
}

type RootCidStatus = int

const (
	RootCid_DEL RootCidStatus = iota
	RootCid_ADD
)

func (ci *ChunkInfo) isDownload(rootCid boson.Address) bool {
	c, err := ci.chunkStore.GetChunkByOverlay(chunkstore.SERVICE, rootCid, ci.addr)
	if err != nil {
		if err != storage.ErrNotFound {
			ci.logger.Errorf("chunkInfo isDownload:%w", err)
		}
		return false
	}
	bv, err := bitvector.NewFromBytes(c.B, c.Len)
	if err != nil {
		ci.logger.Errorf("chunkInfo isDownload construct bitVector:%w", err)
		return false
	}
	return bv.Equals()
}

func (ci *ChunkInfo) updateService(rootCid boson.Address, index, len int64, overlay boson.Address) error {
	has, err := ci.chunkStore.HasChunk(chunkstore.SERVICE, rootCid, overlay)
	if err != nil {
		return err
	}

	var provider chunkstore.Provider
	provider.Len = int(len)
	provider.Bit = int(index)
	provider.Overlay = overlay
	err = ci.chunkStore.PutChunk(chunkstore.SERVICE, rootCid, []chunkstore.Provider{provider})
	if err != nil {
		return err
	}

	var consumer chunkstore.Consumer
	consumerList, _ := ci.chunkStore.GetChunk(chunkstore.SERVICE, rootCid)

	for i := range consumerList {
		if consumerList[i].Overlay.Equal(overlay) {
			consumer = consumerList[i]
			break
		}
	}

	if !has {
		if overlay.Equal(ci.addr) {
			go ci.PublishRootCidStatus(RootCidStatusEven{
				RootCid: rootCid,
				Status:  RootCid_ADD,
			})
		}
	} else {
		bv := BitVector{B: consumer.B, Len: consumer.Len}
		if overlay.Equal(ci.addr) {
			go ci.PublishDownloadProgress(rootCid, BitVectorInfo{
				RootCid:   rootCid,
				Bitvector: bv,
			})
		} else {
			go ci.PublishRetrievalProgress(rootCid, BitVectorInfo{
				RootCid:   rootCid,
				Overlay:   overlay,
				Bitvector: bv,
			})
		}
	}

	return nil
}
