package api

import (
	"net"
	"net/http"

	"github.com/gauss-project/aurorafs/pkg/boson"
	"github.com/gauss-project/aurorafs/pkg/jsonhttp"
)

func (s *server) apiPort(w http.ResponseWriter, r *http.Request) {
	type out struct {
		DebugApiPort string `json:"debugApiPort"`
		RpcWsPort    string `json:"rpcWsPort"`
	}

	_, p1, err := net.SplitHostPort(s.DebugApiAddr)
	if err != nil {
		jsonhttp.InternalServerError(w, err)
		return
	}
	_, p2, err := net.SplitHostPort(s.RPCWSAddr)
	if err != nil {
		jsonhttp.InternalServerError(w, err)
		return
	}
	resp := &out{
		DebugApiPort: p1,
		RpcWsPort:    p2,
	}

	jsonhttp.OK(w, resp)
}

func (s *server) restore(w http.ResponseWriter, r *http.Request) {
	_ = s.kad.EachKnownPeer(func(address boson.Address, u uint8) (stop, jumpToNext bool, err error) {
		ss := s.kad.SnapshotAddr(address)
		if ss.ConnectionTotalDuration > 0 {
			_ = s.route.Connect(r.Context(), address)
		}
		return false, false, nil
	})
	jsonhttp.OK(w, nil)
}
