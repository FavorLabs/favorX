package debugapi

import (
	"net/http"

	favor "github.com/FavorLabs/favorX"
	"github.com/FavorLabs/favorX/pkg/jsonhttp"
	"github.com/inhies/go-bytesize"
	"github.com/net-byte/vtun/common/counter"
	"github.com/net-byte/vtun/register"
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

func (s *Service) tunRegisterList(w http.ResponseWriter, r *http.Request) {
	list := register.ListClientIPs()

	jsonhttp.OK(w, struct {
		Total int      `json:"total"`
		List  []string `json:"list"`
	}{
		Total: len(list),
		List:  list,
	})
}

func (s *Service) tunStats(w http.ResponseWriter, r *http.Request) {
	up := bytesize.New(float64(counter.GetReadBytes())).String()
	down := bytesize.New(float64(counter.GetWrittenBytes())).String()

	jsonhttp.OK(w, struct {
		Up   string `json:"up"`
		Down string `json:"down"`
	}{
		Up:   up,
		Down: down,
	})
}
