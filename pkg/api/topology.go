package api

import (
	"net"
	"net/http"
	"time"

	"github.com/gauss-project/aurorafs/pkg/boson"
	"github.com/gauss-project/aurorafs/pkg/jsonhttp"
	"github.com/gauss-project/aurorafs/pkg/topology"
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

func (s *server) saveConnectedPeers() {
	go func() {
		t := time.NewTicker(time.Second * 10)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				var now []boson.Address
				_ = s.kad.EachPeer(func(address boson.Address, u uint8) (stop, jumpToNext bool, err error) {
					now = append(now, address)
					return false, false, nil
				}, topology.Filter{Reachable: false})
				s.snapshotPeers = now
			}
		}
	}()
}

func (s *server) restore(w http.ResponseWriter, r *http.Request) {
	list := s.snapshotPeers
	for _, v := range list {
		_ = s.route.Connect(r.Context(), v)
	}
	jsonhttp.OK(w, nil)
}
