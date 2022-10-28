// Copyright 2018 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package localstore

import (
	"context"
	"encoding/binary"
	"errors"
	"time"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/localstore/chunkstore"
	"github.com/FavorLabs/favorX/pkg/sctx"
	"github.com/FavorLabs/favorX/pkg/shed"
	"github.com/FavorLabs/favorX/pkg/shed/driver"
	"github.com/FavorLabs/favorX/pkg/storage"
)

// Get returns a chunk from the database. If the chunk is
// not found storage.ErrNotFound will be returned.
// All required indexes will be updated required by the
// Getter Mode. Get is required to implement chunk.Store
// interface.
func (db *DB) Get(ctx context.Context, mode storage.ModeGet, addr boson.Address, index int64) (ch boson.Chunk, err error) {
	db.metrics.ModeGet.Inc()
	defer totalTimeMetric(db.metrics.TotalTimeGet, time.Now())

	defer func() {
		if err != nil {
			db.metrics.ModeGetFailure.Inc()
		}
	}()

	out, err := db.get(mode, addr, sctx.GetRootHash(ctx), index)
	if err != nil {
		if errors.Is(err, driver.ErrNotFound) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return boson.NewChunk(boson.NewAddress(out.Address), out.Data), nil
}

// get returns Item from the retrieval index
// and updates other indexes.
func (db *DB) get(mode storage.ModeGet, addr, rootAddr boson.Address, index int64) (out shed.Item, err error) {
	item := addressToItem(addr)
	out, err = db.retrievalDataIndex.Get(item)
	if err != nil {
		return out, err
	}
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, out.Type)
	switch mode {
	// update the access timestamp and gc index
	case storage.ModeGetRequest:
		if b[0]&(0x1<<uint(0%8)) == 0 {
			return out, storage.ErrNotFound
		}
		if !rootAddr.IsZero() {
			db.updateGCItems(addressToItem(rootAddr))
			if ok, _ := db.HasChunkBit(chunkstore.SOURCE, rootAddr, int(index)); !ok {
				batch := db.shed.NewBatch()
				binIDs := make(map[uint8]uint64)
				db.batchMu.Lock()
				_, _, err = db.putUpload(batch, binIDs, out)
				if err != nil {
					return out, err
				}
				db.batchMu.Unlock()
			}
		} else {
			db.updateGCItems(out)
		}

	case storage.ModeGetPin:
		pinnedItem, err := db.pinIndex.Get(item)
		if err != nil {
			return out, err
		}
		return pinnedItem, nil
	case storage.ModeGetChain:
		if b[0]&(0x1<<uint(1%8)) == 0 {
			return out, storage.ErrNotFound
		}
	// no updates to indexes
	case storage.ModeGetSync:
	case storage.ModeGetLookup:
	default:
		return out, ErrInvalidMode
	}
	return out, nil
}

// updateGCItems is called when ModeGetRequest is used
// for Get or GetMulti to update access time and gc indexes
// for all returned chunks.
func (db *DB) updateGCItems(items ...shed.Item) {
	if db.updateGCSem != nil {
		// wait before creating new goroutines
		// if updateGCSem buffer id full
		db.updateGCSem <- struct{}{}
	}
	db.updateGCWG.Add(1)
	go func() {
		defer db.updateGCWG.Done()
		if db.updateGCSem != nil {
			// free a spot in updateGCSem buffer
			// for a new goroutine
			defer func() { <-db.updateGCSem }()
		}

		db.metrics.GCUpdate.Inc()
		defer totalTimeMetric(db.metrics.TotalTimeUpdateGC, time.Now())

		for _, item := range items {
			err := db.updateGC(item)
			if err != nil {
				db.metrics.GCUpdateError.Inc()
				db.logger.Errorf("localstore update gc: %v", err)
			}
		}
		// if gc update hook is defined, call it
		if testHookUpdateGC != nil {
			testHookUpdateGC()
		}
	}()
}

// updateGC updates garbage collection index for
// a single item. Provided item is expected to have
// only Address and Data fields with non zero values,
// which is ensured by the get function.
func (db *DB) updateGC(item shed.Item) (err error) {
	db.batchMu.Lock()
	defer db.batchMu.Unlock()
	if db.gcRunning {
		db.dirtyAddresses = append(db.dirtyAddresses, boson.NewAddress(item.Address))
	}

	batch := db.shed.NewBatch()

	// update accessTimeStamp in retrieve, gc

	i, err := db.retrievalAccessIndex.Get(item)
	switch {
	case err == nil:
		item.AccessTimestamp = i.AccessTimestamp
	case errors.Is(err, driver.ErrNotFound):
		// no chunk accesses
	default:
		return err
	}
	if item.AccessTimestamp == 0 {
		// chunk is not yet synced
		// do not add it to the gc index
		return nil
	}
	if item.BinID == 0 {
		i, err = db.retrievalDataIndex.Get(item)
		if err != nil && !errors.Is(err, driver.ErrNotFound) {
			return err
		}
		item.BinID = i.BinID
	}
	// update the gc item timestamp in case
	// it exists
	var gcItem shed.Item
LOOP:
	gcItem, err = db.gcIndex.Get(item)
	if err != nil {
		if !errors.Is(err, driver.ErrNotFound) {
			return err
		}
		if item.BinID == 0 {
			return nil
		}
		item.BinID = 0
		goto LOOP
	}
	item.BinID = i.BinID
	// delete current entry from the gc index
	err = db.gcIndex.DeleteInBatch(batch, gcItem)
	if err != nil {
		return err
	}
	item.GCounter = gcItem.GCounter
	item.AccessTimestamp = now()
	err = db.gcIndex.PutInBatch(batch, item)
	if err != nil {
		return err
	}

	// update retrieve access index
	err = db.retrievalAccessIndex.PutInBatch(batch, item)
	if err != nil {
		return err
	}

	return batch.Commit()
}

func (db *DB) getAllChunks(rootCid boson.Address) ([]boson.Address, error) {
	chunks, err := db.getTransfer(rootCid)
	if chunks == nil {
		chunks, err = db.getChunk(rootCid, 0)
		if err != nil {
			if err == driver.ErrNotFound {
				return nil, storage.ErrNotFound
			}
			return nil, err
		}
	}
	return chunks, nil
}

func (db *DB) getTransfer(rootCid boson.Address) ([]boson.Address, error) {
	item := addressToItem(rootCid)
	out, err := db.transferDataIndex.Get(item)
	if err != nil {
		return nil, err
	}
	data := out.Data
	chs := make([]boson.Address, 0, boson.HashSize+len(data)/boson.HashSize)
	for i := len(data); i > 0; i -= boson.HashSize {
		addr := data[i-boson.HashSize : i]
		ch := boson.NewAddress(addr)
		chs = append(chs, ch)
	}
	return chs, nil
}

func (db *DB) getChunk(ch boson.Address, t int) (chs []boson.Address, err error) {
	item := addressToItem(ch)
	exists, err := db.retrievalDataIndex.Has(item)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	item, err = db.retrievalDataIndex.Get(item)
	if err != nil {
		return nil, err
	}
	data := item.Data[8:]
	size := int64(binary.LittleEndian.Uint64(item.Data[:8]))
	var nodes []*node
	if size <= int64(len(data)) {
		if t == 1 {
			chs = append(chs, ch)
			return
		}
		nodes, _ = UnmarshalBinary(data)
		for _, n := range nodes {
			ps, _ := db.getChunk(n.address, 0)
			chs = append(chs, ps...)
		}
	}
	chs = append(chs, ch)
	if len(nodes) > 0 {
		return chs, err
	}
	for i := 0; i < len(data); i += boson.HashSize {
		ch = boson.NewAddress(data[i : i+boson.HashSize])
		x := [32]byte{}
		slice := x[:]
		if ch.Equal(boson.NewAddress(slice)) {
			continue
		}
		ps, err := db.getChunk(ch, 1)
		if err != nil {
			return nil, err
		}
		chs = append(chs, ps...)
	}
	return
}

// testHookUpdateGC is a hook that can provide
// information when a garbage collection index is updated.
var testHookUpdateGC func()
