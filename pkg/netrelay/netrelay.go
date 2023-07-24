package netrelay

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
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
	"github.com/FavorLabs/favorX/pkg/storage"
)

const keyPrefix = "netrelay_"

type NetRelay interface {
	RelayHttpDo(w http.ResponseWriter, r *http.Request, address boson.Address)
}

type Service struct {
	streamer      p2p.Streamer
	logger        logging.Logger
	route         routetab.RouteTab
	groups        []model.ConfigNodeGroup
	multicast     multicast.GroupInterface
	socks5UDPConn *net.UDPConn
	socks5UDPAddr *net.UDPAddr
	proxyGroup    string
	iface         *WaterIface
	tunGroup      string
	tunConfig     TunConfig
	tunRate       *RateService
	metrics       metrics
	storer        storage.StateStorer
	counter       *Counter
}

func New(streamer p2p.Streamer, storer storage.StateStorer, logging logging.Logger, groups []model.ConfigNodeGroup, route routetab.RouteTab, multicast multicast.GroupInterface) *Service {
	srv := &Service{
		streamer:  streamer,
		logger:    logging,
		groups:    groups,
		route:     route,
		multicast: multicast,
		metrics:   newMetrics(),
		storer:    storer,
	}
	srv.counter = NewCounter(srv.metrics)
	return srv
}

func (s *Service) RelayHttpDo(w http.ResponseWriter, r *http.Request, addr boson.Address) {
	url := strings.ReplaceAll(r.URL.String(), address.RelayPrefixHttp, "")
	var forward []boson.Address
	if boson.ZeroAddress.Equal(addr) {
		urls := strings.Split(url, "/")
		group := urls[1]
		nodes, err1 := s.multicast.GetGroupPeers(group)
		if err1 != nil {
			jsonhttp.InternalServerError(w, err1)
			return
		}

		if len(nodes.Connected) == 0 && len(nodes.Keep) == 0 {
			jsonhttp.InternalServerError(w, fmt.Sprintf("No corresponding node found of group:%s", group))
			return
		}
		forward = append(forward, nodes.Connected...)
		forward = append(forward, nodes.Keep...)
	} else {
		forward = append(forward, addr)
	}

	var (
		body []byte
		err  error
	)

	for k, addr := range forward {
		if k == 0 {
			body, err = io.ReadAll(r.Body)
			if err != nil {
				break
			}
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
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
			if errors.Is(err, net.ErrClosed) {
				err = nil
			}
			if err != nil {
				s.logger.Tracef("RelayHttpDoV2 io.copy resp from %s err %v", addr, err)
			}
			respErrCh <- err
		}()
		// request
		reqErrCh := make(chan error, 1)
		go func() {
			_, err = io.Copy(st, conn)
			if errors.Is(err, net.ErrClosed) {
				err = nil
			}
			if err != nil {
				s.logger.Tracef("RelayHttpDoV2 io.copy req to %s err %v", addr, err)
			}
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

func (s *Service) getForward(group string) (forward []boson.Address, err error) {
	nodes, err := s.multicast.GetGroupPeers(group)
	if err != nil {
		return
	}

	if len(nodes.Connected) == 0 && len(nodes.Keep) == 0 {
		err = fmt.Errorf("no peers in group %s", group)
		return
	}
	forward = append(forward, nodes.Connected...)
	forward = append(forward, nodes.Keep...)
	return
}

func (s *Service) SetProxyGroup(group string) error {
	_, err := s.multicast.GetGroupPeers(group)
	if err != nil {
		return fmt.Errorf("proxy group %s notfound", group)
	}
	s.proxyGroup = group
	return nil
}

func (s *Service) SetTunGroup(group string) error {
	_, err := s.multicast.GetGroupPeers(group)
	if err != nil {
		return fmt.Errorf("tun forward group %s notfound", group)
	}
	s.tunGroup = group
	return nil
}

func (s *Service) StartProxyTCP(addr, natAddr string) *net.TCPListener {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		panic(err)
	}
	if natAddr != "" {
		addr = fmt.Sprintf(":%s", port)
	}
	ipaddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		panic(err)
	}
	if natAddr == "" {
		if host == "" || ipaddr.IP.Equal(net.IPv4zero) || ipaddr.IP.Equal(net.IPv6zero) {
			panic("proxy cannot use zero address listening")
		}
	}
	localServer, err := net.ListenTCP("tcp", ipaddr)
	if err != nil {
		s.logger.Errorf("proxy listen tcp %s err", addr, err.Error())
		panic(err)
	}
	s.logger.Infof("proxy listen tcp %s", localServer.Addr())
	go func() {
		for {
			conn, e := localServer.Accept()
			if e != nil {
				if errors.Is(e, net.ErrClosed) {
					s.logger.Info("proxy tcp closed")
					break
				}
				s.logger.Warningf("proxy tcp accept err", e.Error())
				continue
			}
			s.parseFirst(conn)
		}
	}()
	return localServer
}

func (s *Service) StartProxyUDP(addr, natAddr string) *net.UDPConn {
	ipaddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		panic(err)
	}
	if natAddr != "" {
		s.socks5UDPAddr, err = net.ResolveUDPAddr("udp", natAddr)
		if err != nil {
			panic(err)
		}
	} else {
		s.socks5UDPAddr = ipaddr
	}

	s.socks5UDPConn, err = net.ListenUDP("udp", ipaddr)
	if err != nil {
		s.logger.Errorf("socks5 listen udp %s err", addr, err.Error())
		panic(err)
	}

	s.logger.Infof("proxy enabled socks5 udp %s", s.socks5UDPConn.LocalAddr())
	go func() {
		for {
			b := make([]byte, 65507)
			n, src, e := s.socks5UDPConn.ReadFromUDP(b)
			if e != nil {
				if errors.Is(e, net.ErrClosed) {
					s.logger.Info("proxy socks5 udp closed")
					break
				}
				s.logger.Warningf("proxy socks5 udp read err", e.Error())
				continue
			}
			s.counter.IncrProxyInBytes(n)
			s.logger.Debugf("proxy socks5(udp) got from %s", src)
			go s.socks5ProxyUDP(src, b[0:n])
		}
	}()
	return s.socks5UDPConn
}

func (s *Service) parseFirst(conn net.Conn) {
	var b [1]byte
	n, err := conn.Read(b[:])
	if err != nil || n != 1 {
		return
	}
	s.counter.IncrProxyInBytes(n)
	if b[0] == 0x05 {
		s.logger.Debugf("proxy socks5(tcp) got from %s", conn.RemoteAddr())
		go s.socks5ProxyTCP(conn)
	} else {
		s.logger.Debugf("proxy http(s) got from %s", conn.RemoteAddr())
		go s.httpProxyStart(conn, []byte{b[0]})
	}
}

func (s *Service) httpProxyStart(conn net.Conn, first []byte) {
	forward, err := s.getForward(s.proxyGroup)
	if err != nil {
		s.logger.Errorf("proxy http(s) get forward peer err %s", err)
		return
	}
	for _, addr := range forward {
		err = s.copyStreamHttpProxy(first, conn, addr)
		if err == nil {
			break
		}
	}
}

func (s *Service) copyStreamHttpProxy(first []byte, conn net.Conn, addr boson.Address) (err error) {
	var st p2p.Stream
	if s.route.IsNeighbor(addr) {
		st, err = s.streamer.NewStream(context.Background(), addr, nil, protocolName, protocolVersion, streamHttpProxy)
	} else {
		st, err = s.streamer.NewConnChainRelayStream(context.Background(), addr, nil, protocolName, protocolVersion, streamHttpProxy)
	}
	if err != nil {
		return fmt.Errorf("new stream %s", err)
	}
	defer func() {
		if err != nil {
			s.logger.Tracef("proxy http(s) to %s err %s", addr, err)
			_ = st.Reset()
		} else {
			_ = st.Close() // must use .Close instead of .FullClose, otherwise it will lead to goroutine leakage
			s.logger.Tracef("proxy http(s) to %s stream close", addr)
		}
	}()
	n, err := st.Write(first)
	if err != nil {
		return err
	}
	s.counter.IncrProxyOutBytes(n)
	defer conn.Close()
	go s.copyIO(st, conn, streamHttpProxy)
	s.copyIO(conn, st, streamHttpProxy)
	return nil
}
