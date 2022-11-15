package api

import (
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/cac"
	"github.com/FavorLabs/favorX/pkg/jsonhttp"
	"github.com/FavorLabs/favorX/pkg/sctx"
	"github.com/FavorLabs/favorX/pkg/storage"
	"github.com/FavorLabs/favorX/pkg/tracing"
	"github.com/gorilla/mux"
	"io"
	"net/http"
)

func (s *server) bufferUploadHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger)

	data, err := io.ReadAll(r.Body)
	if err != nil {
		if jsonhttp.HandleBodyReadError(err, w) {
			return
		}
		s.logger.Debugf("buffer: read buffer data error: %v", err)
		s.logger.Error("buffer: read buffer data error")
		jsonhttp.InternalServerError(w, "cannot read data")
		return
	}
	if len(data) > boson.ChunkSize {
		s.logger.Debugf("buffer upload: chunk data exceeds %d bytes", boson.ChunkSize+boson.SpanSize)
		s.logger.Error("buffer upload: chunk data error")
		jsonhttp.RequestEntityTooLarge(w, "payload too large")
		return
	}
	ch, err := cac.New(data)
	if err != nil {
		logger.Debugf("buffer upload: split write all: %v", err)
		logger.Error("buffer upload: split write all")
		jsonhttp.InternalServerError(w, nil)
		return
	}
	r = r.WithContext(sctx.SetRootHash(r.Context(), ch.Address()))
	ctx := r.Context()
	has, err := s.storer.Has(ctx, storage.ModeHasChunk, ch.Address())
	if err != nil {
		s.logger.Debugf("buffer upload: store has: %v", err)
		s.logger.Error("buffer upload: store has")
		jsonhttp.InternalServerError(w, "storage error")
		return
	}
	if has {
		s.logger.Error("buffer upload: chunk already exists")
		jsonhttp.Conflict(w, "chunk already exists")
		return
	}
	_, err = s.storer.Put(ctx, storage.ModePutChain, ch)
	if err != nil {
		s.logger.Debugf("buffer upload: chunk write error: %v", err)
		s.logger.Error("buffer upload: chunk write error")
		jsonhttp.BadRequest(w, "chunk write error")
		return
	}

	jsonhttp.Created(w, bytesPostResponse{
		Reference: ch.Address(),
	})
}

func (s *server) bufferGetHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger).Logger
	targets := r.URL.Query().Get("targets")
	if targets != "" {
		r = r.WithContext(sctx.SetTargets(r.Context(), targets))
	}
	nameOrHex := mux.Vars(r)["address"]
	address, err := s.resolveNameOrAddress(nameOrHex)
	if err != nil {
		logger.Debugf("buffer: parse address %s: %v", nameOrHex, err)
		logger.Error("buffer: parse address error")
		jsonhttp.NotFound(w, nil)
		return
	}

	r = r.WithContext(sctx.SetRootHash(r.Context(), address))
	ch, err := s.storer.Get(r.Context(), storage.ModeGetChain, address, 0)
	if err != nil {
		s.logger.Debugf("buffer upload: chunk read error: %v", err)
		s.logger.Error("buffer upload: chunk read error")
		jsonhttp.BadRequest(w, "chunk read error")
		jsonhttp.NotFound(w, err)
		return
	}

	jsonhttp.Created(w, ch.Data()[boson.SpanSize:])
}
