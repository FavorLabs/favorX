package api

import (
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/file/pipeline/builder"
	"github.com/FavorLabs/favorX/pkg/jsonhttp"
	"github.com/FavorLabs/favorX/pkg/tracing"
	"github.com/gorilla/mux"
	"net/http"
	"strings"
)

type bytesPostResponse struct {
	Reference boson.Address `json:"reference"`
}

// bytesUploadHandler handles upload of raw binary data of arbitrary length.
func (s *server) bytesUploadHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger)

	// Add the tag to the context
	ctx := r.Context()

	pipe := builder.NewPipelineBuilder(ctx, s.storer, requestModePut(r), requestEncrypt(r))
	addr, err := builder.FeedPipeline(ctx, pipe, r.Body)
	if err != nil {
		logger.Debugf("bytes upload: split write all: %v", err)
		logger.Error("bytes upload: split write all")
		jsonhttp.InternalServerError(w, nil)
		return
	}

	if strings.ToLower(r.Header.Get(PinHeader)) == StringTrue {
		if err := s.pinning.CreatePin(ctx, addr, false); err != nil {
			logger.Debugf("bytes upload: creation of pin for %q failed: %v", addr, err)
			logger.Error("bytes upload: creation of pin failed")
			jsonhttp.InternalServerError(w, nil)
			return
		}
	}

	jsonhttp.Created(w, bytesPostResponse{
		Reference: addr,
	})
}

// bytesGetHandler handles retrieval of raw binary data of arbitrary length.
func (s *server) bytesGetHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger).Logger
	nameOrHex := mux.Vars(r)["address"]

	address, err := s.resolveNameOrAddress(nameOrHex)
	if err != nil {
		logger.Debugf("bytes: parse address %s: %v", nameOrHex, err)
		logger.Error("bytes: parse address error")
		jsonhttp.NotFound(w, nil)
		return
	}

	additionalHeaders := http.Header{
		"Content-Type": {"application/octet-stream"},
	}

	s.downloadHandler(w, r, address, address, 0, additionalHeaders, true)
}
