package debugapi_test

import (
	favor "github.com/FavorLabs/favorX"
	"github.com/FavorLabs/favorX/pkg/debugapi"
	"github.com/FavorLabs/favorX/pkg/jsonhttp/jsonhttptest"
	"net/http"
	"testing"
)

func TestHealth(t *testing.T) {
	testServer := newTestServer(t, testServerOptions{})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/health", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(debugapi.StatusResponse{
			Status:  "ok",
			Version: favor.Version,
		}),
	)
}

func TestReadiness(t *testing.T) {
	testServer := newTestServer(t, testServerOptions{})

	jsonhttptest.Request(t, testServer.Client, http.MethodGet, "/readiness", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(debugapi.StatusResponse{
			Status:  "ok",
			Version: favor.Version,
		}),
	)
}
