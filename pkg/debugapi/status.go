package debugapi

import (
	"net/http"

	favor "github.com/FavorLabs/favorX"
	"github.com/gauss-project/aurorafs/pkg/jsonhttp"
)

type statusResponse struct {
	Status       string `json:"status"`
	Version      string `json:"version"`
	FullNode     bool   `json:"fullNode"`
	BootNodeMode bool   `json:"bootNodeMode"`
	Auth         bool   `json:"auth"`
}

func (s *Service) statusHandler(w http.ResponseWriter, r *http.Request) {
	jsonhttp.OK(w, statusResponse{
		Status:       "ok",
		Version:      favor.Version,
		FullNode:     s.nodeOptions.NodeMode.IsFull(),
		BootNodeMode: s.nodeOptions.NodeMode.IsBootNode(),
		Auth:         s.restricted,
	})
}
