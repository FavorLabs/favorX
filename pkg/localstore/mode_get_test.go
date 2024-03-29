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
	"github.com/FavorLabs/favorX/pkg/boson"
	filetest "github.com/FavorLabs/favorX/pkg/file/testing"
	"github.com/FavorLabs/favorX/pkg/storage"
	"testing"
	"time"
)

// TestModeGetRequest validates ModeGetRequest index values on the provided DB.
func TestModeGetRequest(t *testing.T) {
	db := newTestDB(t, nil)

	uploadTimestamp := time.Now().UTC().UnixNano()
	defer setNow(func() (t int64) {
		return uploadTimestamp
	})()

	ch := generateTestRandomChunk()

	_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
	if err != nil {
		t.Fatal(err)
	}

	// set update gc test hook to signal when
	// update gc goroutine is done by sending to
	// testHookUpdateGCChan channel, which is
	// used to wait for garbage colletion index
	// changes
	testHookUpdateGCChan := make(chan struct{})
	defer setTestHookUpdateGC(func() {
		testHookUpdateGCChan <- struct{}{}
	})()

	t.Run("get unsynced", func(t *testing.T) {
		got, err := db.Get(context.Background(), storage.ModeGetRequest, ch.Address(), 0)
		if err != nil {
			t.Fatal(err)
		}
		// wait for update gc goroutine to be done
		<-testHookUpdateGCChan

		if !got.Address().Equal(ch.Address()) {
			t.Errorf("got chunk address %x, want %x", got.Address(), ch.Address())
		}

		if !bytes.Equal(got.Data(), ch.Data()) {
			t.Errorf("got chunk data %x, want %x", got.Data(), ch.Data())
		}

		t.Run("retrieve indexes", newRetrieveIndexesTestWithAccess(db, ch, uploadTimestamp, 0))

		t.Run("gc index count", newItemsCountTest(db.gcIndex, 0))

		t.Run("gc size", newIndexGCSizeTest(db))
	})

	// set chunk to synced state
	err = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("first get", func(t *testing.T) {
		got, err := db.Get(context.Background(), storage.ModeGetRequest, ch.Address(), 0)
		if err != nil {
			t.Fatal(err)
		}
		// wait for update gc goroutine to be done
		<-testHookUpdateGCChan

		if !got.Address().Equal(ch.Address()) {
			t.Errorf("got chunk address %x, want %x", got.Address(), ch.Address())
		}

		if !bytes.Equal(got.Data(), ch.Data()) {
			t.Errorf("got chunk data %x, want %x", got.Data(), ch.Data())
		}

		t.Run("retrieve indexes", newRetrieveIndexesTestWithAccess(db, ch, uploadTimestamp, uploadTimestamp))

		t.Run("gc index", newGCIndexTest(db, ch, uploadTimestamp, uploadTimestamp, 1, nil))

		t.Run("gc index count", newItemsCountTest(db.gcIndex, 1))

		t.Run("gc size", newIndexGCSizeTest(db))
	})

	t.Run("second get", func(t *testing.T) {
		accessTimestamp := time.Now().UTC().UnixNano()
		defer setNow(func() (t int64) {
			return accessTimestamp
		})()

		got, err := db.Get(context.Background(), storage.ModeGetRequest, ch.Address(), 0)
		if err != nil {
			t.Fatal(err)
		}
		// wait for update gc goroutine to be done
		<-testHookUpdateGCChan

		if !got.Address().Equal(ch.Address()) {
			t.Errorf("got chunk address %x, want %x", got.Address(), ch.Address())
		}

		if !bytes.Equal(got.Data(), ch.Data()) {
			t.Errorf("got chunk data %x, want %x", got.Data(), ch.Data())
		}

		t.Run("retrieve indexes", newRetrieveIndexesTestWithAccess(db, ch, uploadTimestamp, accessTimestamp))

		t.Run("gc index", newGCIndexTest(db, ch, uploadTimestamp, accessTimestamp, 1, nil))

		t.Run("gc index count", newItemsCountTest(db.gcIndex, 1))

		t.Run("gc size", newIndexGCSizeTest(db))
	})

	t.Run("multi", func(t *testing.T) {
		got, err := db.GetMulti(context.Background(), storage.ModeGetRequest, ch.Address())
		if err != nil {
			t.Fatal(err)
		}
		// wait for update gc goroutine to be done
		<-testHookUpdateGCChan

		if !got[0].Address().Equal(ch.Address()) {
			t.Errorf("got chunk address %x, want %x", got[0].Address(), ch.Address())
		}

		if !bytes.Equal(got[0].Data(), ch.Data()) {
			t.Errorf("got chunk data %x, want %x", got[0].Data(), ch.Data())
		}

		t.Run("retrieve indexes", newRetrieveIndexesTestWithAccess(db, ch, uploadTimestamp, uploadTimestamp))

		t.Run("gc index", newGCIndexTest(db, ch, uploadTimestamp, uploadTimestamp, 1, nil))

		t.Run("gc index count", newItemsCountTest(db.gcIndex, 1))

		t.Run("gc size", newIndexGCSizeTest(db))
	})
}

// TestModeGetSync validates ModeGetSync index values on the provided DB.
func TestModeGetSync(t *testing.T) {
	db := newTestDB(t, nil)

	uploadTimestamp := time.Now().UTC().UnixNano()
	defer setNow(func() (t int64) {
		return uploadTimestamp
	})()

	ch := generateTestRandomChunk()

	_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
	if err != nil {
		t.Fatal(err)
	}

	got, err := db.Get(context.Background(), storage.ModeGetSync, ch.Address(), 0)
	if err != nil {
		t.Fatal(err)
	}

	if !got.Address().Equal(ch.Address()) {
		t.Errorf("got chunk address %x, want %x", got.Address(), ch.Address())
	}

	if !bytes.Equal(got.Data(), ch.Data()) {
		t.Errorf("got chunk data %x, want %x", got.Data(), ch.Data())
	}

	t.Run("retrieve indexes", newRetrieveIndexesTestWithAccess(db, ch, uploadTimestamp, 0))

	t.Run("gc index count", newItemsCountTest(db.gcIndex, 0))

	t.Run("gc size", newIndexGCSizeTest(db))

	t.Run("multi", func(t *testing.T) {
		got, err := db.GetMulti(context.Background(), storage.ModeGetSync, ch.Address())
		if err != nil {
			t.Fatal(err)
		}

		if !got[0].Address().Equal(ch.Address()) {
			t.Errorf("got chunk address %x, want %x", got[0].Address(), ch.Address())
		}

		if !bytes.Equal(got[0].Data(), ch.Data()) {
			t.Errorf("got chunk data %x, want %x", got[0].Data(), ch.Data())
		}
	})
}

// setTestHookUpdateGC sets testHookUpdateGC and
// returns a function that will reset it to the
// value before the change.
func setTestHookUpdateGC(h func()) (reset func()) {
	current := testHookUpdateGC
	reset = func() { testHookUpdateGC = current }
	testHookUpdateGC = h
	return reset
}

// TestSetTestHookUpdateGC tests if setTestHookUpdateGC changes
// testHookUpdateGC function correctly and if its reset function
// resets the original function.
func TestSetTestHookUpdateGC(t *testing.T) {
	// Set the current function after the test finishes.
	defer func(h func()) { testHookUpdateGC = h }(testHookUpdateGC)

	// expected value for the unchanged function
	original := 1
	// expected value for the changed function
	changed := 2

	// this variable will be set with two different functions
	var got int

	// define the original (unchanged) functions
	testHookUpdateGC = func() {
		got = original
	}

	// set got variable
	testHookUpdateGC()

	// test if got variable is set correctly
	if got != original {
		t.Errorf("got hook value %v, want %v", got, original)
	}

	// set the new function
	reset := setTestHookUpdateGC(func() {
		got = changed
	})

	// set got variable
	testHookUpdateGC()

	// test if got variable is set correctly to changed value
	if got != changed {
		t.Errorf("got hook value %v, want %v", got, changed)
	}

	// set the function to the original one
	reset()

	// set got variable
	testHookUpdateGC()

	// test if got variable is set correctly to original value
	if got != original {
		t.Errorf("got hook value %v, want %v", got, original)
	}
}

func TestGetChunks(t *testing.T) {
	db := newTestDB(t, nil)
	// create root chunk and two data chunks referenced in the root chunk
	rootChunk := filetest.GenerateTestRandomFileChunk(boson.ZeroAddress, boson.ChunkSize*2, boson.SectionSize*2)
	_, err := db.put(storage.ModePutUpload, rootChunk.Address(), rootChunk)
	if err != nil {
		t.Fatal(err)
	}

	firstAddress := boson.NewAddress(rootChunk.Data()[8 : boson.SectionSize+8])
	firstChunk := filetest.GenerateTestRandomFileChunk(firstAddress, boson.ChunkSize, boson.ChunkSize)
	_, err = db.put(storage.ModePutUpload, rootChunk.Address(), firstChunk)
	if err != nil {
		t.Fatal(err)
	}

	secondAddress := boson.NewAddress(rootChunk.Data()[boson.SectionSize+8:])
	secondChunk := filetest.GenerateTestRandomFileChunk(secondAddress, boson.ChunkSize, boson.ChunkSize)
	_, err = db.put(storage.ModePutUpload, rootChunk.Address(), secondChunk)
	if err != nil {
		t.Fatal(err)
	}

	chs, _ := db.getChunk(rootChunk.Address(), 2)
	if len(chs) != 3 {
		t.Errorf("chs len %v, want 3", len(chs))
	}
}

func TestChainChunks(t *testing.T) {
	db := newTestDB(t, nil)

	ch := generateTestRandomChunk()
	ch1 := generateTestRandomChunk()

	_, err := db.Put(context.Background(), storage.ModePutChain, ch)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Put(context.Background(), storage.ModePutRequest, ch1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Put(context.Background(), storage.ModePutChain, ch1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Get(context.Background(), storage.ModeGetRequest, ch.Address(), 0)
	if err == nil {
		t.Error("ch is not nil")
	}

	_, err = db.Get(context.Background(), storage.ModeGetChain, ch.Address(), 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Get(context.Background(), storage.ModeGetRequest, ch1.Address(), 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Get(context.Background(), storage.ModeGetChain, ch1.Address(), 0)
	if err != nil {
		t.Fatal(err)
	}

}
