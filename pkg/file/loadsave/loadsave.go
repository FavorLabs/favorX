package loadsave

import (
	"bytes"
	"context"
	"errors"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/file"
	"github.com/FavorLabs/favorX/pkg/file/joiner"
	"github.com/FavorLabs/favorX/pkg/file/pipeline"
	"github.com/FavorLabs/favorX/pkg/file/pipeline/builder"
	"github.com/FavorLabs/favorX/pkg/storage"
)

var readonlyLoadsaveError = errors.New("readonly manifest loadsaver")

type PutGetter interface {
	storage.Putter
	storage.Getter
}

// loadSave is needed for manifest operations and provides
// simple wrapping over load and save operations using file
// package abstractions. use with caution since Loader will
// load all of the subtrie of a given hash in memory.
type loadSave struct {
	storer     PutGetter
	readMode   storage.ModeGet
	pipelineFn func() pipeline.Interface
}

// New returns a new read-write load-saver.
func New(storer PutGetter, pipelineFn func() pipeline.Interface) file.LoadSaver {
	return &loadSave{
		storer:     storer,
		pipelineFn: pipelineFn,
	}
}

// NewReadonly returns a new read-only load-saver
// which will error on write.
func NewReadonly(storer PutGetter, readMode storage.ModeGet) file.LoadSaver {
	return &loadSave{
		storer:   storer,
		readMode: readMode,
	}
}

func (ls *loadSave) Load(ctx context.Context, ref []byte, index int64) ([]byte, error) {
	j, _, err := joiner.New(ctx, ls.storer, ls.readMode, boson.NewAddress(ref), index)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(nil)
	_, err = file.JoinReadAll(ctx, j, buf)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (ls *loadSave) Save(ctx context.Context, data []byte) ([]byte, error) {
	if ls.pipelineFn == nil {
		return nil, readonlyLoadsaveError
	}

	pipe := ls.pipelineFn()
	address, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(data))
	if err != nil {
		return boson.ZeroAddress.Bytes(), err
	}

	return address.Bytes(), nil
}
