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
	"testing"

	"github.com/FavorLabs/favorX/pkg/shed/driver"
	"github.com/FavorLabs/favorX/pkg/storage"
)

// TestModeSetRemove validates ModeSetRemove index values on the provided DB.
func TestModeSetRemove(t *testing.T) {
	for _, tc := range multiChunkTestCases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t, nil)

			chunks := generateTestRandomChunks(tc.count)

			_, err := db.Put(context.Background(), storage.ModePutUpload, chunks...)
			if err != nil {
				t.Fatal(err)
			}

			err = db.Set(context.Background(), storage.ModeSetRemove, chunkAddresses(chunks)...)
			if err != nil {
				t.Fatal(err)
			}

			t.Run("retrieve indexes", func(t *testing.T) {
				for _, ch := range chunks {
					wantErr := driver.ErrNotFound
					_, err := db.retrievalDataIndex.Get(addressToItem(ch.Address()))
					if !errors.Is(err, wantErr) {
						t.Errorf("got error %v, want %v", err, wantErr)
					}

					// access index should not be set
					_, err = db.retrievalAccessIndex.Get(addressToItem(ch.Address()))
					if !errors.Is(err, wantErr) {
						t.Errorf("got error %v, want %v", err, wantErr)
					}
				}

				t.Run("retrieve data index count", newItemsCountTest(db.retrievalDataIndex, 0))

				t.Run("retrieve access index count", newItemsCountTest(db.retrievalAccessIndex, 0))
			})

			t.Run("gc index count", newItemsCountTest(db.gcIndex, 0))

			t.Run("gc size", newIndexGCSizeTest(db))
		})
	}
}
