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
	"errors"
	"time"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/sctx"
	"github.com/FavorLabs/favorX/pkg/shed"
	"github.com/FavorLabs/favorX/pkg/shed/driver"
	"github.com/FavorLabs/favorX/pkg/storage"
)

// Set updates database indexes for
// chunks represented by provided addresses.
// Set is required to implement chunk.Store
// interface.
func (db *DB) Set(ctx context.Context, mode storage.ModeSet, addrs ...boson.Address) (err error) {
	rootHash := sctx.GetRootHash(ctx)

	db.metrics.ModeSet.Inc()
	defer totalTimeMetric(db.metrics.TotalTimeSet, time.Now())
	err = db.set(mode, rootHash, addrs...)
	if err != nil {
		db.metrics.ModeSetFailure.Inc()
	}
	return err
}

// set updates database indexes for
// chunks represented by provided addresses.
// It acquires lockAddr to protect two calls
// of this function for the same address in parallel.
func (db *DB) set(mode storage.ModeSet, rootAddr boson.Address, addrs ...boson.Address) (err error) {
	// protect parallel updates
	db.batchMu.Lock()
	defer db.batchMu.Unlock()
	if db.gcRunning {
		db.dirtyAddresses = append(db.dirtyAddresses, addrs...)
	}

	batch := db.shed.NewBatch()

	// variables that provide information for operations
	// to be done after write batch function successfully executes
	var gcSizeChange int64 // number to add or subtract from gcSize
	// triggerPullFeed := make(map[uint8]struct{}) // signal pull feed subscriptions to iterate

	switch mode {
	case storage.ModeSetSync:
		for _, addr := range addrs {
			c, err := db.setSync(batch, addr, mode)
			if err != nil {
				return err
			}
			gcSizeChange += c
		}

	case storage.ModeSetRemove:
		for _, addr := range addrs {
			c, err := db.setRemove(batch, addr, addressToItem(rootAddr))
			if err != nil {
				return err
			}
			gcSizeChange += c
		}

	case storage.ModeSetPin:
		for _, addr := range addrs {
			has, err := db.retrievalDataIndex.Has(addressToItem(addr))
			if err != nil {
				return err
			}

			if !has {
				return storage.ErrNotFound
			}

			c, err := db.setPin(batch, addressToItem(addr), addressToItem(rootAddr))
			if err != nil {
				return err
			}
			gcSizeChange += c
		}
	case storage.ModeSetUnpin:
		for _, addr := range addrs {
			c, err := db.setUnpin(batch, addressToItem(addr), addressToItem(rootAddr))
			if err != nil {
				return err
			}
			gcSizeChange += c
		}
	default:
		return ErrInvalidMode
	}

	err = db.incGCSizeInBatch(batch, gcSizeChange)
	if err != nil {
		return err
	}

	err = batch.Commit()
	if err != nil {
		return err
	}

	return nil
}

// setSync adds the chunk to the garbage collection after syncing by updating indexes
//   - ModeSetSync - the corresponding tag is incremented, then item is removed
//     from push sync index
//   - update to gc index happens given item does not exist in pin index
//
// Provided batch is updated.
func (db *DB) setSync(batch driver.Batching, addr boson.Address, mode storage.ModeSet) (gcSizeChange int64, err error) {
	item := addressToItem(addr)

	// need to get access timestamp here as it is not
	// provided by the access function, and it is not
	// a property of a chunk provided to Accessor.Put.

	i, err := db.retrievalDataIndex.Get(item)
	if err != nil {
		if errors.Is(err, driver.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	item.StoreTimestamp = i.StoreTimestamp
	item.BinID = i.BinID

	i, err = db.retrievalAccessIndex.Get(item)
	switch {
	case err == nil:
		item.AccessTimestamp = i.AccessTimestamp
		err = db.gcIndex.DeleteInBatch(batch, item)
		if err != nil {
			return 0, err
		}
		gcSizeChange--
	case errors.Is(err, driver.ErrNotFound):
		// the chunk is not accessed before
	default:
		return 0, err
	}
	item.AccessTimestamp = now()
	err = db.retrievalAccessIndex.PutInBatch(batch, item)
	if err != nil {
		return 0, err
	}

	// Add in gcIndex only if this chunk is not pinned
	ok, err := db.pinIndex.Has(item)
	if err != nil {
		return 0, err
	}
	if !ok {
		err = db.gcIndex.PutInBatch(batch, item)
		if err != nil {
			return 0, err
		}
		gcSizeChange++
	}

	return gcSizeChange, nil
}

// setRemove removes the chunk by updating indexes:
//   - delete from retrieve, pull, gc
//
// Provided batch is updated.
func (db *DB) setRemove(batch driver.Batching, addr boson.Address, rootItem shed.Item) (gcSizeChange int64, err error) {
	item := addressToItem(addr)
	item, err = db.retrievalDataIndex.Get(item)
	if err != nil && !errors.Is(err, driver.ErrNotFound) {
		return 0, err
	}
	if errors.Is(err, driver.ErrNotFound) {
		return db.gcRemove(batch, rootItem)
	}
	if item.Counter > 1 {
		item.Counter--
		err = db.retrievalDataIndex.PutInBatch(batch, item)
		return 0, err
	}
	i, err := db.retrievalAccessIndex.Get(item)
	switch {
	case err == nil:
		item.AccessTimestamp = i.AccessTimestamp
	case errors.Is(err, driver.ErrNotFound):
	default:
		return 0, err
	}

	db.metrics.GCStoreTimeStamps.Set(float64(item.StoreTimestamp))
	db.metrics.GCStoreAccessTimeStamps.Set(float64(item.AccessTimestamp))

	pinItem, err := db.pinIndex.Get(item)
	switch {
	case err == nil:
		pinItem.PinCounter--
		if pinItem.PinCounter > 0 {
			err = db.pinIndex.PutInBatch(batch, pinItem)
			if err != nil {
				return 0, err
			}
			// chunk still pinned
			return 0, nil
		} else {
			err = db.pinIndex.DeleteInBatch(batch, item)
			if err != nil {
				return 0, err
			}
		}
	case errors.Is(err, driver.ErrNotFound):
	default:
		return 0, err
	}

	err = db.retrievalDataIndex.DeleteInBatch(batch, item)
	if err != nil && !errors.Is(err, driver.ErrNotFound) {
		return 0, err
	}

	if addr.Equal(boson.NewAddress(rootItem.Address)) {
		rootItem = item
	} else {
		err = db.retrievalAccessIndex.DeleteInBatch(batch, item)
		if err != nil && !errors.Is(err, driver.ErrNotFound) {
			return 0, err
		}
	}

	return db.gcRemove(batch, rootItem)
}

func (db *DB) gcRemove(batch driver.Batching, rootItem shed.Item) (gcSizeChange int64, err error) {

	if rootItem.StoreTimestamp == 0 {
		i, err := db.retrievalDataIndex.Get(rootItem)
		if err != nil {
			return 0, err
		}
		rootItem.StoreTimestamp = i.StoreTimestamp
		rootItem.BinID = i.BinID
		// need to get access timestamp here as it is not
		// provided by the access function, and it is not
		// a property of a chunk provided to Accessor.Put.
		i, err = db.retrievalAccessIndex.Get(rootItem)
		switch {
		case err == nil:
			rootItem.AccessTimestamp = i.AccessTimestamp
		case errors.Is(err, driver.ErrNotFound):
			return 0, nil
		default:
			return 0, err
		}
	}
	rootCid := boson.NewAddress(rootItem.Address)
	err = db.deleteFile(rootCid)
	if err != nil {
		db.logger.Errorf("del file :%s error: %w", rootCid.String(), err)
		return 0, err
	}
	// a check is needed for decrementing gcSize
	// as delete is not reporting if the key/value pair
	// is deleted or not
	var gcItem shed.Item
	gcItem, err = db.gcIndex.Get(rootItem)
	if err != nil {
		if !errors.Is(err, driver.ErrNotFound) {
			return 0, err
		}
		return 0, nil
	}
	if gcItem.GCounter > 1 {
		gcItem.GCounter--
		err = db.gcIndex.PutInBatch(batch, gcItem)
		if err != nil {
			db.logger.Errorf("put gc err:%w", err)
			return 0, err
		}
	} else {
		db.metrics.GCStoreTimeStamps.Set(float64(rootItem.StoreTimestamp))
		db.metrics.GCStoreAccessTimeStamps.Set(float64(rootItem.AccessTimestamp))

		err = db.retrievalAccessIndex.DeleteInBatch(batch, rootItem)
		if err != nil {
			db.logger.Errorf("del retrieval gc err:%w", err)
			return 0, err
		}

		err = db.gcIndex.DeleteInBatch(batch, gcItem)
		if err != nil {
			db.logger.Errorf("del gc err:%w", err)
			return 0, err
		}
	}

	return -1, nil
}

// setPin increments pin counter for the chunk by updating
// pin index and sets the chunk to be excluded from garbage collection.
// Provided batch is updated.
func (db *DB) setPin(batch driver.Batching, item, rootItem shed.Item) (gcSizeChange int64, err error) {
	i, err := db.pinIndex.Get(item)
	switch {
	case err == nil:
	case errors.Is(err, driver.ErrNotFound):
	default:
		return 0, err
	}
	item.PinCounter = i.PinCounter

	if rootItem.Address != nil {
		i, err = db.retrievalAccessIndex.Get(rootItem)
		if err != nil {
			if !errors.Is(err, driver.ErrNotFound) {
				return 0, err
			}
		} else {
			rootItem.AccessTimestamp = i.AccessTimestamp
			i, err = db.retrievalDataIndex.Get(rootItem)
			if err != nil {
				return 0, err
			}

			rootItem.StoreTimestamp = i.StoreTimestamp
			rootItem.BinID = i.BinID

			var gcItem shed.Item

			gcItem, err = db.gcIndex.Get(rootItem)
			if err != nil {
				if !errors.Is(err, driver.ErrNotFound) {
					return 0, err
				}
			} else {
				if gcItem.GCounter == 1 {
					err = db.gcIndex.DeleteInBatch(batch, gcItem)
					if err != nil {
						return 0, err
					}
				} else {
					gcItem.GCounter--
					err = db.gcIndex.Put(gcItem)
					if err != nil {
						return 0, err
					}
				}
			}
			gcSizeChange--
		}
	}

	item.PinCounter++
	err = db.pinIndex.PutInBatch(batch, item)
	if err != nil {
		return gcSizeChange, err
	}

	return
}

// setUnpin decrements pin counter for the chunk by updating pin index.
// Provided batch is updated.
func (db *DB) setUnpin(batch driver.Batching, item, rootItem shed.Item) (gcSizeChange int64, err error) {
	// Get the existing pin counter of the chunk
	i, err := db.pinIndex.Get(item)
	if err != nil {
		return 0, err
	}
	item.PinCounter = i.PinCounter
	// Decrement the pin counter or
	// delete it from pin index if the pin counter has reached 0
	if item.PinCounter > 1 {
		item.PinCounter--
		return 0, db.pinIndex.PutInBatch(batch, item)
	}

	err = db.pinIndex.DeleteInBatch(batch, item)
	if err != nil {
		return 0, err
	}

	if rootItem.Address != nil {
		i, err = db.retrievalAccessIndex.Get(rootItem)
		if err != nil {
			if !errors.Is(err, driver.ErrNotFound) {
				return 0, err
			}
			rootItem.AccessTimestamp = now()
			err = db.retrievalAccessIndex.PutInBatch(batch, rootItem)
			if err != nil {
				return 0, err
			}
		} else {
			rootItem.AccessTimestamp = i.AccessTimestamp
		}

		i, err = db.retrievalDataIndex.Get(rootItem)
		if err != nil {
			return 0, err
		}
		rootItem.StoreTimestamp = i.StoreTimestamp
		rootItem.BinID = i.BinID

		var gcItem shed.Item

		gcItem, err = db.gcIndex.Get(rootItem)
		if err != nil {
			if !errors.Is(err, driver.ErrNotFound) {
				return 0, err
			}
			rootItem.GCounter = 1
			err = db.gcIndex.PutInBatch(batch, rootItem)
			if err != nil {
				return 0, err
			}
		} else {
			gcItem.GCounter++
			err = db.gcIndex.PutInBatch(batch, gcItem)
			if err != nil {
				return 0, err
			}
		}

		gcSizeChange++
	}

	return
}

func (db *DB) setRemoveTransfer(batch driver.Batching, rootCid boson.Address) error {
	item := addressToItem(rootCid)
	err := db.transferDataIndex.DeleteInBatch(batch, item)
	return err
}

func (db *DB) setRemoveAll(item shed.Item) error {
	db.batchMu.Lock()
	defer db.batchMu.Unlock()
	batch := db.shed.NewBatch()
	var gcSizeChange int64 // number to add or subtract from gcSize
	rootCid := boson.NewAddress(item.Address)
	chunks, err := db.getAllChunks(rootCid)
	if err != nil {
		return err
	}
	for _, addr := range chunks {
		c, err := db.setRemove(batch, addr, item)
		if err != nil {
			return err
		}
		gcSizeChange += c
	}
	err = db.setRemoveTransfer(batch, rootCid)
	if err != nil {
		return err
	}
	err = db.incGCSizeInBatch(batch, gcSizeChange)
	if err != nil {
		return err
	}

	err = batch.Commit()
	if err != nil {
		return err
	}
	return nil
}
