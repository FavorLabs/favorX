package store

import (
	"context"
	"errors"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/file/pipeline"
	"github.com/FavorLabs/favorX/pkg/storage"
)

var errInvalidData = errors.New("store: invalid data")

type storeWriter struct {
	l    storage.Putter
	mode storage.ModePut
	ctx  context.Context
	next pipeline.ChainWriter
}

// NewStoreWriter returns a storeWriter. It just writes the given data
// to a given storage.Putter.
func NewStoreWriter(ctx context.Context, l storage.Putter, mode storage.ModePut, next pipeline.ChainWriter) pipeline.ChainWriter {
	return &storeWriter{ctx: ctx, l: l, mode: mode, next: next}
}

func (w *storeWriter) ChainWrite(p *pipeline.PipeWriteArgs) error {
	if p.Ref == nil || p.Data == nil {
		return errInvalidData
	}

	c := boson.NewChunk(boson.NewAddress(p.Ref), p.Data)
	_, err := w.l.Put(w.ctx, w.mode, c)
	if err != nil {
		return err
	}

	if w.next == nil {
		return nil
	}

	return w.next.ChainWrite(p)

}

func (w *storeWriter) Sum() ([]byte, error) {
	return w.next.Sum()
}
