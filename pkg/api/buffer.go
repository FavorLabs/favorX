package api

import (
	"fmt"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/cac"
	"github.com/FavorLabs/favorX/pkg/file/joiner"
	"github.com/FavorLabs/favorX/pkg/jsonhttp"
	"github.com/FavorLabs/favorX/pkg/sctx"
	"github.com/FavorLabs/favorX/pkg/storage"
	"github.com/FavorLabs/favorX/pkg/tracing"
	"github.com/ethersphere/langos"
	"github.com/gorilla/mux"
	"io"
	"net/http"
	"strings"
	"time"
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

	j := ch.Data()[boson.SpanSize:]
	w.Write(j)
	w.WriteHeader(200)
	return
}

func (s *server) downloadChainHandler(w http.ResponseWriter, r *http.Request, rootCid boson.Address, additionalHeaders http.Header) {
	targets := r.URL.Query().Get("targets")
	if targets != "" {
		r = r.WithContext(sctx.SetTargets(r.Context(), targets))
	}
	r = r.WithContext(sctx.SetRootLen(r.Context(), 1))
	reader, l, err := joiner.New(r.Context(), s.storer, storage.ModeGetChain, rootCid, 0)
	if err != nil {
		s.logger.Debugf("buffer upload: chunk read error: %v", err)
		s.logger.Error("buffer upload: chunk read error")
		jsonhttp.BadRequest(w, "chunk read error")
		jsonhttp.NotFound(w, err)
		return
	}

	// include additional headers
	for name, values := range additionalHeaders {
		w.Header().Set(name, strings.Join(values, "; "))
	}

	// http cache policy
	w.Header().Set("Cache-Control", "no-store")

	w.Header().Set("Content-Length", fmt.Sprintf("%d", l))
	w.Header().Set("Decompressed-Content-Length", fmt.Sprintf("%d", l))
	w.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")
	if targets != "" {
		w.Header().Set(TargetsRecoveryHeader, targets)
	}
	http.ServeContent(w, r, "", time.Now(), langos.NewBufferedLangos(reader, lookaheadBufferSize(l)))
}
