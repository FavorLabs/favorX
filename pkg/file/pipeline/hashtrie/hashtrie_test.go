package hashtrie_test

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/file/pipeline"
	"github.com/FavorLabs/favorX/pkg/file/pipeline/bmt"
	"github.com/FavorLabs/favorX/pkg/file/pipeline/hashtrie"
	"github.com/FavorLabs/favorX/pkg/file/pipeline/store"
	"github.com/FavorLabs/favorX/pkg/storage"
	"github.com/FavorLabs/favorX/pkg/storage/mock"
)

var (
	addr boson.Address
	span []byte
	ctx  = context.Background()
	mode = storage.ModePutUpload
)

func init() {
	b := make([]byte, 32)
	b[31] = 0x01
	addr = boson.NewAddress(b)

	span = make([]byte, 8)
	binary.LittleEndian.PutUint64(span, 1)
}

func TestLevels(t *testing.T) {
	var (
		branching = 4
		chunkSize = 128
		hashSize  = 32
	)

	// to create a level wrap we need to do branching^(level-1) writes
	for _, tc := range []struct {
		desc   string
		writes int
	}{
		{
			desc:   "2 at L1",
			writes: 2,
		},
		{
			desc:   "1 at L2, 1 at L1", // dangling chunk
			writes: 16 + 1,
		},
		{
			desc:   "1 at L3, 1 at L2, 1 at L1",
			writes: 64 + 16 + 1,
		},
		{
			desc:   "1 at L3, 2 at L2, 1 at L1",
			writes: 64 + 16 + 16 + 1,
		},
		{
			desc:   "1 at L5, 1 at L1",
			writes: 1024 + 1,
		},
		{
			desc:   "1 at L5, 1 at L3",
			writes: 1024 + 1,
		},
		{
			desc:   "2 at L5, 1 at L1",
			writes: 1024 + 1024 + 1,
		},
		{
			desc:   "3 at L5, 2 at L3, 1 at L1",
			writes: 1024 + 1024 + 1024 + 64 + 64 + 1,
		},
		{
			desc:   "1 at L7, 1 at L1",
			writes: 4096 + 1,
		},
		{
			desc:   "1 at L8", // balanced trie - all good
			writes: 16384,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			s := mock.NewStorer()
			pf := func() pipeline.ChainWriter {
				lsw := store.NewStoreWriter(ctx, s, mode, nil)
				return bmt.NewBmtWriter(lsw)
			}

			ht := hashtrie.NewHashTrieWriter(chunkSize, branching, hashSize, pf)

			for i := 0; i < tc.writes; i++ {
				a := &pipeline.PipeWriteArgs{Ref: addr.Bytes(), Span: span}
				err := ht.ChainWrite(a)
				if err != nil {
					t.Fatal(err)
				}
			}

			ref, err := ht.Sum()
			if err != nil {
				t.Fatal(err)
			}

			rootch, err := s.Get(ctx, storage.ModeGetRequest, boson.NewAddress(ref), 0)
			if err != nil {
				t.Fatal(err)
			}

			//check the span. since write spans are 1 value 1, then expected span == tc.writes
			sp := binary.LittleEndian.Uint64(rootch.Data()[:boson.SpanSize])
			if sp != uint64(tc.writes) {
				t.Fatalf("want span %d got %d", tc.writes, sp)
			}
		})
	}
}

func TestLevels_TrieFull(t *testing.T) {
	var (
		branching = 4
		chunkSize = 128
		hashSize  = 32
		writes    = 16384 // this is to get a balanced trie
		s         = mock.NewStorer()
		pf        = func() pipeline.ChainWriter {
			lsw := store.NewStoreWriter(ctx, s, mode, nil)
			return bmt.NewBmtWriter(lsw)
		}

		ht = hashtrie.NewHashTrieWriter(chunkSize, branching, hashSize, pf)
	)

	// to create a level wrap we need to do branching^(level-1) writes
	for i := 0; i < writes; i++ {
		a := &pipeline.PipeWriteArgs{Ref: addr.Bytes(), Span: span}
		err := ht.ChainWrite(a)
		if err != nil {
			t.Fatal(err)
		}
	}

	a := &pipeline.PipeWriteArgs{Ref: addr.Bytes(), Span: span}
	err := ht.ChainWrite(a)
	if !errors.Is(err, hashtrie.ErrTrieFull) {
		t.Fatal(err)
	}

	// it is questionable whether the writer should go into some
	// corrupt state after the last write which causes the trie full
	// error, in which case we would return an error on Sum()
	_, err = ht.Sum()
	if err != nil {
		t.Fatal(err)
	}
}

// TestRegression is a regression test for the bug
func TestRegression(t *testing.T) {
	var (
		branching = 128
		chunkSize = 4096
		hashSize  = 32
		writes    = 67100000 / 4096
		span      = make([]byte, 8)
		s         = mock.NewStorer()
		pf        = func() pipeline.ChainWriter {
			lsw := store.NewStoreWriter(ctx, s, mode, nil)
			return bmt.NewBmtWriter(lsw)
		}
		ht = hashtrie.NewHashTrieWriter(chunkSize, branching, hashSize, pf)
	)
	binary.LittleEndian.PutUint64(span, 4096)

	for i := 0; i < writes; i++ {
		a := &pipeline.PipeWriteArgs{Ref: addr.Bytes(), Span: span}
		err := ht.ChainWrite(a)
		if err != nil {
			t.Fatal(err)
		}
	}

	ref, err := ht.Sum()
	if err != nil {
		t.Fatal(err)
	}

	rootch, err := s.Get(ctx, storage.ModeGetRequest, boson.NewAddress(ref), 0)
	if err != nil {
		t.Fatal(err)
	}

	sp := binary.LittleEndian.Uint64(rootch.Data()[:boson.SpanSize])
	if sp != uint64(writes*4096) {
		t.Fatalf("want span %d got %d", writes*4096, sp)
	}
}
