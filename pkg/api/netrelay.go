package api

import (
	"net/http"

	"github.com/FavorLabs/favorX/pkg/boson"
)

func (s *server) relayDo(w http.ResponseWriter, r *http.Request) {
	s.netRelay.RelayHttpDo(w, r, boson.ZeroAddress)
}
