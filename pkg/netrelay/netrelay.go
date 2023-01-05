package netrelay

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/FavorLabs/favorX/pkg/address"
	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/jsonhttp"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/multicast"
	"github.com/FavorLabs/favorX/pkg/multicast/model"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/routetab"
)

type NetRelay interface {
	RelayHttpDo(w http.ResponseWriter, r *http.Request, address boson.Address)
}

type Service struct {
	streamer  p2p.Streamer
	logger    logging.Logger
	route     routetab.RouteTab
	groups    []model.ConfigNodeGroup
	multicast multicast.GroupInterface
}

func New(streamer p2p.Streamer, logging logging.Logger, groups []model.ConfigNodeGroup, route routetab.RouteTab, multicast multicast.GroupInterface) *Service {
	return &Service{streamer: streamer, logger: logging, groups: groups, route: route, multicast: multicast}
}

func (s *Service) RelayHttpDo(w http.ResponseWriter, r *http.Request, addr boson.Address) {
	url := strings.ReplaceAll(r.URL.String(), address.RelayPrefixHttp, "")
	var forward []boson.Address
	if boson.ZeroAddress.Equal(addr) {
		urls := strings.Split(url, "/")
		group := urls[1]
		g, err1 := s.multicast.GetGroup(group)
		if err1 != nil {
			jsonhttp.InternalServerError(w, err1)
			return
		}
		nodes := g.Peers()
		if len(nodes.Connected) == 0 && len(nodes.Keep) == 0 {
			jsonhttp.InternalServerError(w, fmt.Sprintf("No corresponding node found of group:%s", group))
			return
		}
		forward = append(forward, nodes.Connected...)
		forward = append(forward, nodes.Keep...)
	} else {
		forward = append(forward, addr)
	}

	var err error
	for _, addr := range forward {
		err = s.copyStream(w, r, addr)
		if err == nil {
			break
		}
	}
	if err != nil {
		jsonhttp.InternalServerError(w, err)
	}
}

func (s *Service) copyStream(w http.ResponseWriter, r *http.Request, addr boson.Address) (err error) {
	var st p2p.Stream
	if s.route.IsNeighbor(addr) {
		st, err = s.streamer.NewStream(r.Context(), addr, nil, protocolName, protocolVersion, streamRelayHttpReqV2)
	} else {
		st, err = s.streamer.NewConnChainRelayStream(r.Context(), addr, nil, protocolName, protocolVersion, streamRelayHttpReqV2)
	}
	if err != nil {
		return fmt.Errorf("new stream %s", err)
	}
	defer func() {
		if err != nil {
			s.logger.Tracef("RelayHttpDoV2 to %s err %s", addr, err)
			_ = st.Reset()
		} else {
			_ = st.Close() // must use .Close instead of .FullClose, otherwise it will lead to goroutine leakage
			s.logger.Tracef("RelayHttpDoV2 to %s stream close", addr)
		}
	}()
	err = r.Write(st)
	if err != nil {
		return err
	}
	if r.Header.Get("Connection") == "Upgrade" && r.Header.Get("Upgrade") == "websocket" {
		w.Header().Set("hijack", "true")
		conn, _, _ := w.(http.Hijacker).Hijack()
		defer conn.Close()
		// response
		respErrCh := make(chan error, 1)
		go func() {
			_, err = io.Copy(conn, st)
			s.logger.Tracef("RelayHttpDoV2 to %s io.copy resp err %v", addr, err)
			respErrCh <- err
		}()
		// request
		reqErrCh := make(chan error, 1)
		go func() {
			_, err = io.Copy(st, conn)
			s.logger.Tracef("RelayHttpDoV2 to %s io.copy req err %v", addr, err)
			reqErrCh <- err
		}()
		select {
		case err = <-respErrCh:
			return err
		case err = <-reqErrCh:
			return err
		}
	} else {
		buf := bufio.NewReader(st)
		resp, err := http.ReadResponse(buf, r)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		// Copy any headers
		for k, v := range resp.Header {
			w.Header().Del(k)
			for _, h := range v {
				w.Header().Add(k, h)
			}
		}
		// Write response status and headers
		w.WriteHeader(resp.StatusCode)

		_, err = io.Copy(w, resp.Body)
		return err
	}
}
