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
	"encoding/binary"
	"errors"
	"os"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/localstore/chunkstore"
	"github.com/FavorLabs/favorX/pkg/localstore/filestore"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/shed"
	"github.com/FavorLabs/favorX/pkg/shed/driver"
	"github.com/FavorLabs/favorX/pkg/storage"
	"github.com/prometheus/client_golang/prometheus"
)

var _ storage.Storer = &DB{}

var (
	// ErrInvalidMode is retuned when an unknown Mode
	// is provided to the function.
	ErrInvalidMode = errors.New("invalid mode")
)

var (
	// Default value for Capacity DB option.
	defaultCapacity uint64 = 80000
	// Limit the number of goroutines created by Getters
	// that call updateGC function. Value 0 sets no limit.
	maxParallelUpdateGC = 1000
)

//type FileInterface interface {
//	GetListFile(page filestore.Page, filter []filestore.Filter, sort filestore.Sort) []filestore.FileView
//	PutFile(file filestore.FileView) error
//	DeleteFile(reference boson.Address) error
//	HashFile(reference boson.Address) bool
//}

// DB is the local store implementation and holds
// database related objects.
type DB struct {
	shed *shed.DB

	// schema name of loaded data
	schemaName shed.StringField

	fullNode bool

	// retrieval indexes
	retrievalDataIndex   shed.Index
	retrievalAccessIndex shed.Index
	transferDataIndex    shed.Index
	// binIDs stores the latest chunk serial ID for every
	// proximity order bin
	binIDs shed.Uint64Vector

	// garbage collection index
	gcIndex shed.Index

	// pin files Index
	pinIndex shed.Index

	// field that stores number of intems in gc index
	gcSize shed.Uint64Field

	chunkstore chunkstore.Interface

	filestore filestore.Interface
	// garbage collection is triggered when gcSize exceeds
	// the capacity value
	capacity uint64

	// triggers garbage collection event loop
	collectGarbageTrigger       chan struct{}
	collectMemoryGarbageTrigger chan struct{}

	// a buffered channel acting as a semaphore
	// to limit the maximal number of goroutines
	// created by Getters to call updateGC function
	updateGCSem chan struct{}
	// a wait group to ensure all updateGC goroutines
	// are done before closing the database
	updateGCWG sync.WaitGroup

	// baseKey is the overlay address
	baseKey []byte

	batchMu sync.Mutex

	fileMu sync.RWMutex

	// gcRunning is true while GC is running. it is
	// used to avoid touching dirty gc index entries
	// while garbage collecting.
	gcRunning bool

	// dirtyAddresses are marked while gc is running
	// in order to avoid the removal of dirty entries.
	dirtyAddresses []boson.Address

	// this channel is closed when close function is called
	// to terminate other goroutines
	close chan struct{}

	// protect Close method from exiting before
	// garbage collection and gc size write workers
	// are done
	collectGarbageWorkerDone chan struct{}

	metrics metrics

	logger logging.Logger
}

// Options struct holds optional parameters for configuring DB.
type Options struct {
	// Driver support: leveldb/wiredtiger
	Driver string

	// Capacity is a limit that triggers garbage collection when
	// number of items in gcIndex equals or exceeds it.
	Capacity uint64

	// MetricsPrefix defines a prefix for metrics names.
	MetricsPrefix string

	FullNode bool
}

// New returns a new DB.  All fields and indexes are initialized
// and possible conflicts with schema from existing database is checked.
// One goroutine for writing batches is created.
func New(path string, baseKey []byte, stateStore storage.StateStorer, o *Options, logger logging.Logger) (db *DB, err error) {
	if o == nil {
		// default options
		o = &Options{
			Capacity: defaultCapacity,
		}
	}

	db = &DB{
		fullNode:   o.FullNode,
		capacity:   o.Capacity,
		baseKey:    baseKey,
		chunkstore: chunkstore.New(stateStore),
		filestore:  filestore.New(stateStore),
		// channel collectGarbageTrigger
		// needs to be buffered with the size of 1
		// to signal another event if it
		// is triggered during already running function
		collectGarbageTrigger:       make(chan struct{}, 1),
		collectMemoryGarbageTrigger: make(chan struct{}, 1),
		close:                       make(chan struct{}),
		collectGarbageWorkerDone:    make(chan struct{}),
		metrics:                     newMetrics(),
		logger:                      logger,
	}
	if db.capacity == 0 {
		db.capacity = defaultCapacity
	}

	capacityMB := float64(db.capacity*boson.ChunkSize) * 9.5367431640625e-7

	if capacityMB <= 1000 {
		db.logger.Infof("database capacity: %d chunks (approximately %fMB)", db.capacity, capacityMB)
	} else {
		db.logger.Infof("database capacity: %d chunks (approximately %0.1fGB)", db.capacity, capacityMB/1000)
	}

	if maxParallelUpdateGC > 0 {
		db.updateGCSem = make(chan struct{}, maxParallelUpdateGC)
	}

	shedOpts := &shed.Options{
		Driver: o.Driver,
	}

	db.shed, err = shed.NewDB(path, shedOpts)
	if err != nil {
		return nil, err
	}

	// Identify current storage schema by arbitrary name.
	db.schemaName, err = db.shed.NewStringField("schema-name")
	if err != nil {
		return nil, err
	}
	schemaName, err := db.schemaName.Get()
	if err != nil && !errors.Is(err, driver.ErrNotFound) {
		return nil, err
	}
	if schemaName == "" {
		// initial new localstore run
		err := db.schemaName.Put(DbSchemaCurrent)
		if err != nil {
			return nil, err
		}
	} else {
		// execute possible migrations
		err = db.migrate(schemaName)
		if err != nil {
			return nil, err
		}
	}

	// Persist gc size.
	db.gcSize, err = db.shed.NewUint64Field("gc-size")
	if err != nil {
		return nil, err
	}

	// Index storing actual chunk address, data and bin id.
	db.retrievalDataIndex, err = db.shed.NewIndex("Address->BinID|StoreTimestamp|Type|Counter|Data", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			return fields.Address, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.Address = key
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			b := make([]byte, 32)
			binary.BigEndian.PutUint64(b[:8], fields.BinID)
			binary.BigEndian.PutUint64(b[8:16], uint64(fields.StoreTimestamp))
			binary.BigEndian.PutUint64(b[16:24], fields.Type)
			binary.BigEndian.PutUint64(b[24:32], fields.Counter)
			value = append(b, fields.Data...)
			return value, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.StoreTimestamp = int64(binary.BigEndian.Uint64(value[8:16]))
			e.BinID = binary.BigEndian.Uint64(value[:8])
			e.Type = binary.BigEndian.Uint64(value[16:24])
			e.Counter = binary.BigEndian.Uint64(value[24:32])
			e.Data = value[32:]
			return e, nil
		},
	})
	if err != nil {
		return nil, err
	}
	// Index storing access timestamp for a particular address.
	// It is needed in order to update gc index keys for iteration order.
	db.retrievalAccessIndex, err = db.shed.NewIndex("Address->AccessTimestamp", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			return fields.Address, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.Address = key
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			b := make([]byte, 8)
			binary.BigEndian.PutUint64(b, uint64(fields.AccessTimestamp))
			return b, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.AccessTimestamp = int64(binary.BigEndian.Uint64(value))
			return e, nil
		},
	})
	if err != nil {
		return nil, err
	}
	db.transferDataIndex, err = db.shed.NewIndex("Address->Cids", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			return fields.Address, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.Address = key
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			return fields.Data, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.Data = value
			return e, nil
		},
	})
	if err != nil {
		return nil, err
	}
	// create a vector for bin IDs
	db.binIDs, err = db.shed.NewUint64Vector("bin-ids")
	if err != nil {
		return nil, err
	}
	// gc index for removable chunk ordered by ascending last access time
	db.gcIndex, err = db.shed.NewIndex("AccessTimestamp|BinID|Hash->GCounter", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			b := make([]byte, 16, 16+len(fields.Address))
			binary.BigEndian.PutUint64(b[:8], uint64(fields.AccessTimestamp))
			binary.BigEndian.PutUint64(b[8:16], fields.BinID)
			key = append(b, fields.Address...)
			return key, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.AccessTimestamp = int64(binary.BigEndian.Uint64(key[:8]))
			e.BinID = binary.BigEndian.Uint64(key[8:16])
			e.Address = key[16:]
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			b := make([]byte, 8)
			binary.BigEndian.PutUint64(b, fields.GCounter)
			return b, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.GCounter = binary.BigEndian.Uint64(value)
			return e, nil
		},
	})
	if err != nil {
		return nil, err
	}
	// Create a index structure for storing pinned chunks and their pin counts
	db.pinIndex, err = db.shed.NewIndex("Hash->PinCounter", shed.IndexFuncs{
		EncodeKey: func(fields shed.Item) (key []byte, err error) {
			return fields.Address, nil
		},
		DecodeKey: func(key []byte) (e shed.Item, err error) {
			e.Address = key
			return e, nil
		},
		EncodeValue: func(fields shed.Item) (value []byte, err error) {
			b := make([]byte, 8)
			binary.BigEndian.PutUint64(b[:8], fields.PinCounter)
			return b, nil
		},
		DecodeValue: func(keyItem shed.Item, value []byte) (e shed.Item, err error) {
			e.PinCounter = binary.BigEndian.Uint64(value[:8])
			return e, nil
		},
	})
	if err != nil {
		return nil, err
	}

	// read gcSize from index
	currentSize := uint64(0)
	err = db.gcIndex.Iterate(func(item shed.Item) (stop bool, err error) {
		currentSize += item.GCounter
		return false, nil
	}, nil)
	if err != nil {
		return nil, err
	}
	gcSize, err := db.gcSize.Get()
	if err != nil {
		return nil, err
	}
	if gcSize < currentSize {
		err = db.gcSize.Put(currentSize)
		if err != nil {
			return nil, err
		}
		gcSize = currentSize
	}
	db.logger.Infof("current gc size: %d", gcSize)

	// start garbage collection worker
	go db.collectGarbageWorker()

	return db, nil
}

// Close closes the underlying database.
func (db *DB) Close() (err error) {
	close(db.close)

	// wait for all handlers to finish
	done := make(chan struct{})
	go func() {
		db.updateGCWG.Wait()
		// wait for gc worker to
		// return before closing the shed
		<-db.collectGarbageWorkerDone
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		db.logger.Errorf("localstore closed with still active goroutines")
		// Print a full goroutine dump to debug blocking.
		// TODO: use a logger to write a goroutine profile
		prof := pprof.Lookup("goroutine")
		err = prof.WriteTo(os.Stdout, 2)
		if err != nil {
			return err
		}
	}
	return db.shed.Close()
}

// po computes the proximity order between the address
// and database base key.
func (db *DB) po(addr boson.Address) (bin uint8) {
	return boson.Proximity(db.baseKey, addr.Bytes())
}

// DebugIndices returns the index sizes for all indexes in localstore
// the returned map keys are the index name, values are the number of elements in the index
func (db *DB) DebugIndices() (indexInfo map[string]int, err error) {
	indexInfo = make(map[string]int)
	for k, v := range map[string]shed.Index{
		"retrievalDataIndex":   db.retrievalDataIndex,
		"retrievalAccessIndex": db.retrievalAccessIndex,
		"gcIndex":              db.gcIndex,
		"pinIndex":             db.pinIndex,
	} {
		indexSize, err := v.Count()
		if err != nil {
			return indexInfo, err
		}
		indexInfo[k] = indexSize
	}
	val, err := db.gcSize.Get()
	if err != nil {
		return indexInfo, err
	}
	indexInfo["gcSize"] = int(val)

	return indexInfo, err
}

// chunkToItem creates new Item with data provided by the Chunk.
func chunkToItem(ch boson.Chunk) shed.Item {
	return shed.Item{
		Address: ch.Address().Bytes(),
		Data:    ch.Data(),
		Tag:     ch.TagID(),
	}
}

// addressToItem creates new Item with a provided address.
func addressToItem(addr boson.Address) shed.Item {
	return shed.Item{
		Address: addr.Bytes(),
	}
}

// addressesToItems constructs a slice of Items with only
// addresses set on them.
func addressesToItems(addrs ...boson.Address) []shed.Item {
	items := make([]shed.Item, len(addrs))
	for i, addr := range addrs {
		items[i] = shed.Item{
			Address: addr.Bytes(),
		}
	}
	return items
}

// now is a helper function that returns a current unix timestamp
// in UTC timezone.
// It is set in the init function for usage in production, and
// optionally overridden in tests for data validation.
var now func() int64

func init() {
	// set the now function
	now = func() (t int64) {
		return time.Now().UTC().UnixNano()
	}
}

// totalTimeMetric logs a message about time between provided start time
// and the time when the function is called and sends a resetting timer metric
// with provided name appended with ".total-time".
func totalTimeMetric(metric prometheus.Counter, start time.Time) {
	totalTime := time.Since(start)
	metric.Add(float64(totalTime))

}

func (db *DB) Init() error {
	db.batchMu.Lock()
	defer db.batchMu.Unlock()
	err := db.filestore.Init()
	if err != nil {
		return err
	}
	err = db.chunkstore.Init()
	return err
}

func (db *DB) GetFile(reference boson.Address) (filestore.FileView, bool) {
	db.fileMu.RLock()
	defer db.fileMu.RUnlock()
	return db.filestore.Get(reference)
}

func (db *DB) GetListFile(page filestore.Page, filter []filestore.Filter, sort filestore.Sort) ([]filestore.FileView, int) {
	db.fileMu.RLock()
	defer db.fileMu.RUnlock()
	return db.filestore.GetList(page, filter, sort)
}
func (db *DB) PutFile(file filestore.FileView) error {
	db.fileMu.Lock()
	defer db.fileMu.Unlock()
	return db.filestore.Put(file)
}
func (db *DB) DeleteFile(reference boson.Address) error {
	return db.commonDelete(addressToItem(reference))
}

func (db *DB) gcFile(item shed.Item) error {
	err := db.commonDelete(item)
	return err
}

func (db *DB) commonDelete(item shed.Item) error {
	db.fileMu.Lock()
	defer db.fileMu.Unlock()
	reference := boson.NewAddress(item.Address)
	db.chunkstore.CancelFinder(reference)
	err := db.filestore.Delete(reference)
	if err != nil {
		return err
	}
	return db.setRemoveAll(item)
}

func (db *DB) deleteFile(reference boson.Address) error {
	err := db.chunkstore.RemoveAll(chunkstore.DISCOVER, reference)
	if err != nil {
		return err
	}
	err = db.chunkstore.RemoveAll(chunkstore.SERVICE, reference)
	if err != nil {
		return err
	}
	err = db.chunkstore.RemoveAll(chunkstore.SOURCE, reference)
	if err != nil {
		return err
	}
	return nil
}

func (db *DB) HasFile(reference boson.Address) bool {
	db.fileMu.RLock()
	defer db.fileMu.RUnlock()
	return db.filestore.Has(reference)
}

func (db *DB) UpdateFile(file filestore.FileView) error {
	db.fileMu.Lock()
	defer db.fileMu.Unlock()
	return db.filestore.Update(file)
}

func (db *DB) PutChunk(chunkType chunkstore.ChunkType, reference boson.Address, providers []chunkstore.Provider) error {
	db.fileMu.Lock()
	defer db.fileMu.Unlock()
	return db.chunkstore.Put(chunkType, reference, providers)
}
func (db *DB) GetChunk(chunkType chunkstore.ChunkType, reference boson.Address) ([]chunkstore.Consumer, error) {
	db.fileMu.RLock()
	defer db.fileMu.RUnlock()
	return db.chunkstore.Get(chunkType, reference)
}

func (db *DB) GetChunkByOverlay(chunkType chunkstore.ChunkType, reference, overlay boson.Address) (chunkstore.Consumer, error) {
	db.fileMu.RLock()
	defer db.fileMu.RUnlock()
	return db.chunkstore.GetByOverlay(chunkType, reference, overlay)
}

func (db *DB) GetAllChunk(ChunkType chunkstore.ChunkType) (map[string][]chunkstore.Consumer, error) {
	db.fileMu.RLock()
	defer db.fileMu.RUnlock()
	return db.chunkstore.GetAll(ChunkType)
}

func (db *DB) DeleteChunk(chunkType chunkstore.ChunkType, reference, overlay boson.Address) error {
	db.fileMu.Lock()
	defer db.fileMu.Unlock()
	return db.chunkstore.Remove(chunkType, reference, overlay)
}
func (db *DB) DeleteAllChunk(chunkType chunkstore.ChunkType, reference boson.Address) error {
	db.fileMu.Lock()
	defer db.fileMu.Unlock()
	return db.chunkstore.RemoveAll(chunkType, reference)
}
func (db *DB) HasChunk(chunkType chunkstore.ChunkType, reference, overlay boson.Address) (bool, error) {
	db.fileMu.RLock()
	defer db.fileMu.RUnlock()
	return db.chunkstore.Has(chunkType, reference, overlay)
}

func (db *DB) HasChunkBit(chunkType chunkstore.ChunkType, reference boson.Address, bit int) (bool, error) {
	db.fileMu.RLock()
	defer db.fileMu.RUnlock()
	return db.chunkstore.HasChunk(chunkType, reference, bit)
}

func (db *DB) StartFinder(rootCid boson.Address) {
	db.fileMu.Lock()
	defer db.fileMu.Unlock()
	db.chunkstore.StartFinder(rootCid)
}

func (db *DB) CancelFinder(rootCid boson.Address) {
	db.fileMu.Lock()
	defer db.fileMu.Unlock()
	db.chunkstore.CancelFinder(rootCid)
}

func (db *DB) IsFinder(rootCid boson.Address) bool {
	db.fileMu.RLock()
	defer db.fileMu.RUnlock()
	return db.chunkstore.IsFinder(rootCid)
}

func (db *DB) GetMirror(reference boson.Address) (*filestore.FileMirror, error) {
	db.fileMu.RLock()
	defer db.fileMu.RUnlock()
	return db.filestore.GetMirror(reference)
}

func (db *DB) GetMirrors(reference boson.Address) (fms []*filestore.FileMirror, err error) {
	db.fileMu.RLock()
	defer db.fileMu.RUnlock()
	return db.filestore.GetMirrors(reference)
}

func (db *DB) PutMirrorFile(next, reference boson.Address, ope filestore.Operation) error {
	db.fileMu.Lock()
	defer db.fileMu.Unlock()
	file, ok := db.filestore.Get(reference)
	if !ok {
		return storage.ErrNotFound
	}
	err := db.filestore.Delete(reference)
	if err != nil {
		return err
	}
	m, err := db.filestore.GetMirror(reference)
	pre := boson.ZeroAddress
	if err != nil {
		if err != storage.ErrNotFound {
			return err
		}
	} else {
		pre = m.RootCid
	}
	return db.filestore.PutMirror(pre, next, ope, file)
}

func (db *DB) DeleteMirror(reference boson.Address) error {
	db.fileMu.Lock()
	defer db.fileMu.Unlock()
	return db.filestore.DeleteMirror(reference)
}

func (db *DB) ChunkCounter(reference boson.Address) error {
	db.fileMu.Lock()
	defer db.fileMu.Unlock()
	chunks, err := db.getChunk(reference, 1)
	if err != nil {
		return err
	}
	batch := db.shed.NewBatch()
	binIDs := make(map[uint8]uint64)
	db.batchMu.Lock()
	for _, c := range chunks {
		item := addressToItem(c)
		item.Type = 1
		_, _, err = db.putUpload(batch, binIDs, item)
		if err != nil {
			return err
		}
	}
	err = batch.Commit()
	db.batchMu.Unlock()
	return err
}
