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
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"time"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/sctx"
	"github.com/FavorLabs/favorX/pkg/shed"
	"github.com/FavorLabs/favorX/pkg/shed/driver"
	"github.com/FavorLabs/favorX/pkg/storage"
)

// Put stores Chunks to database and depending
// on the Putter mode, it updates required indexes.
// Put is required to implement storage.Store
// interface.
func (db *DB) Put(ctx context.Context, mode storage.ModePut, chs ...boson.Chunk) (exist []bool, err error) {
	rootHash := sctx.GetRootHash(ctx)

	db.metrics.ModePut.Inc()
	defer totalTimeMetric(db.metrics.TotalTimePut, time.Now())

	exist, err = db.put(mode, rootHash, chs...)
	if err != nil {
		db.metrics.ModePutFailure.Inc()
	}

	return exist, err
}

// put stores Chunks to database and updates other indexes. It acquires lockAddr
// to protect two calls of this function for the same address in parallel. Item
// fields Address and Data must not be with their nil values. If chunks with the
// same address are passed in arguments, only the first chunk will be stored,
// and following ones will have exist set to true for their index in exist
// slice. This is the same behaviour as if the same chunks are passed one by one
// in multiple put method calls.
func (db *DB) put(mode storage.ModePut, rootAddr boson.Address, chs ...boson.Chunk) (exist []bool, err error) {

	// protect parallel updates
	db.batchMu.Lock()
	defer db.batchMu.Unlock()
	if db.gcRunning {
		for _, ch := range chs {
			db.dirtyAddresses = append(db.dirtyAddresses, ch.Address())
		}
	}

	batch := db.shed.NewBatch()

	// variables that provide information for operations
	// to be done after write batch function successfully executes
	var gcSizeChange int64 // number to add or subtract from gcSize

	exist = make([]bool, len(chs))

	// A lazy populated map of bin ids to properly set
	// BinID values for new chunks based on initial value from database
	// and incrementing them.
	// Values from this map are stored with the batch
	binIDs := make(map[uint8]uint64)

	rootItem := addressToItem(rootAddr)

	switch mode {
	case storage.ModePutRequest, storage.ModePutRequestPin:
		for i, ch := range chs {
			pin := mode == storage.ModePutRequestPin
			item := chunkToItem(ch)
			item.Type = 1
			exists, c, err := db.putRequest(batch, binIDs, item, rootItem, pin)
			if err != nil {
				return nil, err
			}
			if err = db.putTransfer(batch, rootAddr, ch.Address()); err != nil {
				return nil, err
			}

			exist[i] = exists
			gcSizeChange += c
		}

	case storage.ModePutUpload, storage.ModePutUploadPin:
		for i, ch := range chs {
			item := chunkToItem(ch)
			item.Type = 1
			exists, _, err := db.putUpload(batch, binIDs, item)
			if err != nil {
				return nil, err
			}
			exist[i] = exists
			if mode == storage.ModePutUploadPin {
				_, err = db.setPin(batch, item, addressToItem(rootAddr))
				if err != nil {
					return nil, err
				}
			}
		}
	case storage.ModePutChain:
		for i, ch := range chs {
			item := chunkToItem(ch)
			item.Type = 2
			exists, c, err := db.putRequest(batch, binIDs, item, rootItem, false)
			if err != nil {
				return nil, err
			}
			exist[i] = exists
			gcSizeChange += c
		}
	default:
		return nil, ErrInvalidMode
	}

	for po, id := range binIDs {
		_ = db.binIDs.PutInBatch(batch, uint64(po), id)
	}

	err = db.incGCSizeInBatch(batch, gcSizeChange)
	if err != nil {
		return nil, err
	}

	err = batch.Commit()
	if err != nil {
		return nil, err
	}

	return exist, nil
}

// putRequest adds an Item to the batch by updating required indexes:
//   - put to indexes: retrieve, gc
//   - it does not enter the syncpool
//
// The batch can be written to the database.
// Provided batch and binID map are updated.
func (db *DB) putRequest(batch driver.Batching, binIDs map[uint8]uint64, item, rootItem shed.Item, forcePin bool) (exists bool, gcSizeChange int64, err error) {
	exists, item, err = db.putUpload(batch, binIDs, item)

	if exists || err != nil {
		return exists, 0, err
	}

	if bytes.Equal(item.Address, rootItem.Address) {
		rootItem.BinID = item.BinID
	}

	gcSizeChange, err = db.preserveOrCache(batch, item, rootItem, forcePin)
	if err != nil {
		return false, gcSizeChange, err
	}

	return false, gcSizeChange, nil
}

// putUpload adds an Item to the batch by updating required indexes:
//   - put to indexes: retrieve, push, pull
//
// The batch can be written to the database.
// Provided batch and binID map are updated.
func (db *DB) putUpload(batch driver.Batching, binIDs map[uint8]uint64, item shed.Item) (exists bool, i shed.Item, err error) {
	exists, err = db.retrievalDataIndex.Has(item)
	if err != nil {
		return
	}
	if exists {
		t := item.Type
		item, err = db.retrievalDataIndex.Get(item)
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, item.Type)
		if b[0]&(0x1<<uint((t-1)%8)) == 0 {
			b[0] ^= 0x1 << uint8((t-1)%8)
			item.Type = binary.LittleEndian.Uint64(b)
		}
		item.Counter++
		if err != nil {
			return
		}
	} else {
		item.Counter = 1
		item.StoreTimestamp = now()
		item.BinID, err = db.incBinID(binIDs, db.po(boson.NewAddress(item.Address)))
		if err != nil {
			return
		}
	}
	err = db.retrievalDataIndex.PutInBatch(batch, item)
	if err != nil {
		return
	}

	return exists, item, err
}

// preserveOrCache is a helper function used to add chunks to either a pinned reserve or gc cache
// (the retrieval access index and the gc index)
func (db *DB) preserveOrCache(batch driver.Batching, item, rootItem shed.Item, forcePin bool) (gcSizeChange int64, err error) {
	if forcePin {
		return db.setPin(batch, item, rootItem)
	}

	return db.setGC(batch, rootItem)
}

// setGC is a helper function used to add chunks to the retrieval access
// index and the gc index in the cases that the putToGCCheck condition
// warrants a gc set. this is to mitigate index leakage in edge cases where
// a chunk is added to a node's localstore and given that the chunk is
// already within that node's NN (thus, it can be added to the gc index
// safely)
func (db *DB) setGC(batch driver.Batching, item shed.Item) (gcSizeChange int64, err error) {
	if item.Address == nil {
		return 0, nil
	}
	i, err := db.retrievalAccessIndex.Get(item)
	if err != nil {
		if !errors.Is(err, driver.ErrNotFound) {
			return 0, err
		}
		i.Address = item.Address
		i.AccessTimestamp = now()
		err = db.retrievalAccessIndex.PutInBatch(batch, i)
		if err != nil {
			return 0, err
		}
	}
	item.AccessTimestamp = i.AccessTimestamp
	if item.BinID == 0 {
		i, err = db.retrievalDataIndex.Get(item)
		if err != nil && !errors.Is(err, driver.ErrNotFound) {
			return 0, err
		}
		item.BinID = i.BinID
	}
	gcItem, err := db.gcIndex.Get(item)
	if err != nil {
		if !errors.Is(err, driver.ErrNotFound) {
			return 0, err
		}
		gcItem = item
		gcItem.GCounter = 1
	} else {
		gcItem.GCounter++
	}
	gcSizeChange++
	err = db.gcIndex.PutInBatch(batch, gcItem)
	if err != nil {
		return 0, err
	}
	return gcSizeChange, nil
}

// incBinID is a helper function for db.put* methods that increments bin id
// based on the current value in the database. This function must be called under
// a db.batchMu lock. Provided binID map is updated.
func (db *DB) incBinID(binIDs map[uint8]uint64, po uint8) (id uint64, err error) {
	if _, ok := binIDs[po]; !ok {
		binIDs[po], err = db.binIDs.Get(uint64(po))
		if err != nil {
			return 0, err
		}
	}
	binIDs[po]++
	return binIDs[po], nil
}

func (db *DB) putTransfer(batch driver.Batching, addr boson.Address, chs ...boson.Address) error {
	item := addressToItem(addr)
	exists, err := db.transferDataIndex.Has(item)
	if err != nil {
		return err
	}
	if exists {
		item, err = db.transferDataIndex.Get(item)
		if err != nil {
			return err
		}
	}
	b := make([]byte, 0, len(item.Data)+len(chs)*boson.HashSize)
	b = append(b, item.Data...)
	for _, c := range chs {
		b = append(b, c.Bytes()...)
	}
	item.Data = b
	err = db.transferDataIndex.PutInBatch(batch, item)
	return err
}
