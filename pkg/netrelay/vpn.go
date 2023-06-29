package netrelay

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/multicast/model"
	"github.com/FavorLabs/favorX/pkg/netrelay/pb"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/p2p/protobuf"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/gorilla/mux"
	"github.com/net-byte/vtun/common/netutil"
	"resenje.org/web"
)

type TunConfig struct {
	ServerIP     string
	ServerIPv6   string
	CIDR         string
	CIDRv6       string
	MTU          int
	SpeedMax     uint64
	SpeedMin     uint64
	RateEveryday string
	RateEnable   bool
}

type VpnService struct {
	service *Service
	logger  logging.Logger
}

func (v *VpnService) ws(w http.ResponseWriter, r *http.Request) {
	group := r.Header.Get("group")
	wsConn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		v.logger.Infof("[vpn server] failed to upgrade http %v", err)
		return
	}
	st, err := v.service.createStream(wsConn, group)
	if err != nil {
		_ = wsConn.Close()
		v.logger.Warningf("[vpn server] failed create stream %v", err)
		return
	}
	v.service.toServer(wsConn, st)
}

func (v *VpnService) addObserveGroup(w http.ResponseWriter, r *http.Request) {
	group := r.Header.Get("group")
	list := strings.Split(r.URL.Query().Get("nodes"), ",")
	nodes := make([]boson.Address, 0)
	for _, val := range list {
		address, err := boson.ParseHexAddress(val)
		if err != nil {
			v.writeString(w, err.Error())
			return
		}
		nodes = append(nodes, address)
	}
	err := v.service.multicast.AddGroup([]model.ConfigNodeGroup{{
		Name:               group,
		GType:              1,
		KeepConnectedPeers: len(nodes),
		Nodes:              nodes,
	}})
	if err != nil {
		v.writeString(w, err.Error())
		return
	}
	v.writeString(w, "OK")
}

func (v *VpnService) delObserveGroup(w http.ResponseWriter, r *http.Request) {
	group := r.Header.Get("group")
	err := v.service.multicast.RemoveGroup(group, model.GTypeObserve)
	if err != nil {
		v.writeString(w, err.Error())
		return
	}
	v.writeString(w, "OK")
}

func (v *VpnService) ip(w http.ResponseWriter, r *http.Request) {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = strings.Split(r.RemoteAddr, ":")[0]
	}
	resp := fmt.Sprintf("%v", ip)
	v.writeString(w, resp)
}

func (v *VpnService) pickIP(w http.ResponseWriter, r *http.Request) {
	group := r.Header.Get("group")
	_, resp := v.service.vpnRequest(r.Context(), group, "/register/pick/ip", "")
	v.writeString(w, resp)
}

func (v *VpnService) deleteIP(w http.ResponseWriter, r *http.Request) {
	group := r.Header.Get("group")
	ip := r.URL.Query().Get("ip")
	if ip != "" {
		_, resp := v.service.vpnRequest(r.Context(), group, "/register/delete/ip", ip)
		v.writeString(w, resp)
		return
	}
	v.writeString(w, "OK")
}

func (v *VpnService) keepaliveIP(w http.ResponseWriter, r *http.Request) {
	group := r.Header.Get("group")
	ip := r.URL.Query().Get("ip")
	if ip != "" {
		_, resp := v.service.vpnRequest(r.Context(), group, "/register/keepalive/ip", ip)
		v.writeString(w, resp)
		return
	}
	v.writeString(w, "OK")
}

func (v *VpnService) listIP(w http.ResponseWriter, r *http.Request) {
	group := r.Header.Get("group")
	_, resp := v.service.vpnRequest(r.Context(), group, "/register/list/ip", "")
	v.writeString(w, resp)
}

func (v *VpnService) prefixIPv4(w http.ResponseWriter, r *http.Request) {
	group := r.Header.Get("group")
	_, resp := v.service.vpnRequest(r.Context(), group, "/register/prefix/ipv4", "")
	v.writeString(w, resp)
}

func (v *VpnService) prefixIPv6(w http.ResponseWriter, r *http.Request) {
	group := r.Header.Get("group")
	_, resp := v.service.vpnRequest(r.Context(), group, "/register/prefix/ipv6", "")
	v.writeString(w, resp)
}

func (v *VpnService) test(w http.ResponseWriter, r *http.Request) {
	group := r.Header.Get("group")
	_, resp := v.service.vpnRequest(r.Context(), group, "/test", "")
	v.writeString(w, resp)
}

func (v *VpnService) writeString(w http.ResponseWriter, s string) {
	n, _ := w.Write([]byte(s))
	if n > 0 {
		v.service.counter.IncrTunOutBytes(n)
	}
}

func (s *Service) NewVpnService() http.Handler {
	srv := &VpnService{
		service: s,
		logger:  s.logger,
	}

	router := mux.NewRouter()
	router.HandleFunc("/observe/add/group", srv.addObserveGroup)
	router.HandleFunc("/observe/delete/group", srv.delObserveGroup)
	router.HandleFunc("/ws", srv.ws)
	router.HandleFunc("/ip", srv.ip)
	router.HandleFunc("/test", srv.test)
	router.HandleFunc("/register/prefix/ipv4", srv.prefixIPv4)
	router.HandleFunc("/register/prefix/ipv6", srv.prefixIPv6)
	router.HandleFunc("/register/list/ip", srv.listIP)
	router.HandleFunc("/register/keepalive/ip", srv.keepaliveIP)
	router.HandleFunc("/register/delete/ip", srv.deleteIP)
	router.HandleFunc("/register/pick/ip", srv.pickIP)

	return web.ChainHandlers(web.FinalHandler(router))
}

// toServer sends data to server
func (s *Service) toServer(wsConn net.Conn, st p2p.Stream) {
	defer wsConn.Close()
	for {
		b, op, err := wsutil.ReadClientData(wsConn)
		if err != nil {
			s.logger.Infof("vpn read src %s", err)
			break
		}
		n := len(b)
		s.counter.IncrTunInBytes(n)
		if op == ws.OpText {
			_ = wsutil.WriteServerMessage(wsConn, op, b)
		} else if op == ws.OpBinary {
			if key := netutil.GetSrcKey(b); key != "" {
				n, err = st.Write(b)
				if err != nil {
					s.logger.Warningf("vpn write packet to dst %s", err)
					break
				}
				s.counter.IncrTunOutBytes(n)
			}
		}
	}
}

func (s *Service) toClient(wsConn net.Conn, st p2p.Stream) {
	packet := make([]byte, tunBuffer)
	for {
		n, err := st.Read(packet)
		if err != nil {
			_ = st.Close()
			break
		}
		s.counter.IncrTunInBytes(n)
		b := packet[:n]
		if key := netutil.GetDstKey(b); key != "" {
			err = wsutil.WriteServerBinary(wsConn, b)
			if err != nil {
				s.logger.Warningf("vpn write packet to src %s", err)
				_ = wsConn.Close()
				break
			}
			s.counter.IncrTunOutBytes(n)
		}
	}
}

func (s *Service) createStream(wsConn net.Conn, group string) (st p2p.Stream, err error) {
	forward, err := s.getForward(group)
	if err != nil {
		s.logger.Errorf("get group(%s) peer err %s", group, err)
		return
	}
	for _, peer := range forward {
		if s.route.IsNeighbor(peer) {
			st, err = s.streamer.NewStream(context.Background(), peer, nil, protocolName, protocolVersion, streamVpnTun)
		} else {
			st, err = s.streamer.NewConnChainRelayStream(context.Background(), peer, nil, protocolName, protocolVersion, streamVpnTun)
		}
		if err == nil {
			go s.toClient(wsConn, st)
			break
		}
	}
	return
}

func (s *Service) vpnRequest(ctx context.Context, group, path, ip string) (err error, respBody string) {
	forward, err := s.getForward(group)
	if err != nil {
		return err, ""
	}
	var st p2p.Stream
	for _, peer := range forward {
		if s.route.IsNeighbor(peer) {
			st, err = s.streamer.NewStream(context.Background(), peer, nil, protocolName, protocolVersion, streamVpnRequest)
		} else {
			st, err = s.streamer.NewConnChainRelayStream(context.Background(), peer, nil, protocolName, protocolVersion, streamVpnRequest)
		}
		if err == nil {
			w, r := protobuf.NewWriterAndReader(st)
			data := &pb.VpnRequest{
				Pattern: path,
				Ip:      ip,
			}
			err = w.WriteMsgWithContext(ctx, data)
			if err == nil {
				s.counter.IncrTunOutBytes(data.Size())
				var resp pb.VpnResponse
				err = r.ReadMsgWithContext(ctx, &resp)
				if err == nil {
					s.counter.IncrTunInBytes(resp.Size())
					return nil, resp.Body
				}
			}
		}
	}
	return errors.New("failed"), ""
}
