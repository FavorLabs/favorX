// Package netstore provides an abstraction layer over the
// File local storage layer that leverages connectivity
// with other peers in order to retrieve chunks from the network that cannot
// be found locally.
package netstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/chunkinfo"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/retrieval"
	"github.com/FavorLabs/favorX/pkg/sctx"
	"github.com/FavorLabs/favorX/pkg/storage"
)

type Store struct {
	storage.Storer
	retrieval retrieval.Interface
	logger    logging.Logger
	chunkInfo chunkinfo.Interface
	addr      boson.Address
	// recoveryCallback recovery.Callback // this is the callback to be executed when a chunk fails to be retrieved
}

var (
	ErrRecoveryAttempt = errors.New("failed to retrieve chunk, recovery initiated")
)

// New returns a new NetStore that wraps a given Storer.
func New(s storage.Storer, r retrieval.Interface, logger logging.Logger, addr boson.Address) *Store {
	return &Store{Storer: s, retrieval: r, logger: logger, addr: addr}
}

func (s *Store) SetChunkInfo(chunkInfo chunkinfo.Interface) {
	s.chunkInfo = chunkInfo
}

// Get retrieves a given chunk address.
// It will request a chunk from the network whenever it cannot be found locally.
func (s *Store) Get(ctx context.Context, mode storage.ModeGet, addr boson.Address, index int64) (ch boson.Chunk, err error) {
	rootHash := sctx.GetRootHash(ctx)
	ch, err = s.Storer.Get(ctx, mode, addr, index)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			if rootHash.IsZero() {
				return nil, err
			}
			// request from network
			ch, err = s.retrieval.RetrieveChunk(ctx, rootHash, addr, index)
			if err != nil {
				return nil, ErrRecoveryAttempt
			}
			return ch, nil
		}

		return nil, fmt.Errorf("netstore get: %w", err)
	}
	if !rootHash.IsZero() {
		_ = s.chunkInfo.OnRetrieved(ctx, rootHash, index, s.addr)
	}

	return ch, nil
}
