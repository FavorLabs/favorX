// Copyright 2019 The FavorX Authors
// This file is part of the FavorX library.
//
// The FavorX library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The FavorX library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the FavorX library. If not, see <http://www.gnu.org/licenses/>.

package localstore

import (
	"context"
	"errors"
	"time"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/shed"
	"github.com/FavorLabs/favorX/pkg/shed/driver"
	"github.com/FavorLabs/favorX/pkg/storage"
)

// GetMulti returns chunks from the database. If one of the chunks is not found
// storage.ErrNotFound will be returned. All required indexes will be updated
// required by the Getter Mode. GetMulti is required to implement chunk.Store
// interface.
func (db *DB) GetMulti(ctx context.Context, mode storage.ModeGet, addrs ...boson.Address) (chunks []boson.Chunk, err error) {
	db.metrics.ModeGetMulti.Inc()
	db.metrics.ModeGetMultiChunks.Add(float64(len(addrs)))
	defer totalTimeMetric(db.metrics.TotalTimeGetMulti, time.Now())

	defer func() {
		if err != nil {
			db.metrics.ModeGetMultiFailure.Inc()
		}
	}()

	out, err := db.getMulti(mode, addrs...)
	if err != nil {
		if errors.Is(err, driver.ErrNotFound) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	chunks = make([]boson.Chunk, len(out))
	for i, ch := range out {
		chunks[i] = boson.NewChunk(boson.NewAddress(ch.Address), ch.Data)
	}
	return chunks, nil
}

// getMulti returns Items from the retrieval index
// and updates other indexes.
func (db *DB) getMulti(mode storage.ModeGet, addrs ...boson.Address) (out []shed.Item, err error) {
	out = make([]shed.Item, len(addrs))
	for i, addr := range addrs {
		out[i].Address = addr.Bytes()
	}

	err = db.retrievalDataIndex.Fill(out)
	if err != nil {
		return nil, err
	}

	switch mode {
	// update the access timestamp and gc index
	case storage.ModeGetRequest:
		db.updateGCItems(out...)

	case storage.ModeGetPin:
		err := db.pinIndex.Fill(out)
		if err != nil {
			return nil, err
		}

	// no updates to indexes
	case storage.ModeGetSync:
	case storage.ModeGetLookup:
	default:
		return out, ErrInvalidMode
	}
	return out, nil
}
