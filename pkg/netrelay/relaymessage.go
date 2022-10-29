package netrelay

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/FavorLabs/favorX/pkg/address"
	"github.com/FavorLabs/favorX/pkg/multicast/model"
	"github.com/FavorLabs/favorX/pkg/p2p"
)

const (
	protocolName         = "netrelay"
	protocolVersion      = "2.0.0"
	streamRelayHttpReqV2 = "httpreqv2" // v2 http proxy support ws
)

func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    protocolName,
		Version: protocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    streamRelayHttpReqV2,
				Handler: s.onRelayHttpReqV2,
			},
		},
	}
}

func (s *Service) getDomainAddrWithScheme(scheme, groupName, domainName string) (string, bool) {
	for _, v := range s.groups {
		if v.Name == groupName {
			var agents []model.ConfigNetDomain
			switch scheme {
			case "ws", "wss":
				agents = v.AgentWS
			case "http", "https":
				agents = v.AgentHttp
			}
			for _, domain := range agents {
				if domain.Domain == domainName {
					return domain.Addr, true
				}
			}
		}
	}
	s.logger.Errorf("domain %v not found in group %s", domainName, groupName)
	return "", false
}

func (s *Service) onRelayHttpReqV2(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	defer func() {
		if err != nil {
			s.logger.Tracef("onRelayHttpReqV2 from %s err %s", p.Address, err)
			_ = stream.Reset()
		} else {
			_ = stream.Close() // must use .Close instead of .FullClose, otherwise it will lead to goroutine leakage
			s.logger.Tracef("onRelayHttpReqV2 from %s stream close", p.Address)
		}
	}()

	buf := bufio.NewReader(stream)
	req, err := http.ReadRequest(buf)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	defer req.Body.Close()

	url := strings.ReplaceAll(req.URL.String(), address.RelayPrefixHttp, "")
	urls := strings.Split(url, "/")

	var reqWS bool
	if req.Header.Get("Connection") == "Upgrade" && req.Header.Get("Upgrade") == "websocket" {
		req.URL.Scheme = "ws"
		reqWS = true
	} else {
		req.URL.Scheme = "http"
	}
	addr, ok := s.getDomainAddrWithScheme(req.URL.Scheme, urls[1], urls[2])
	if !ok {
		return errors.New("domain parse err")
	}
	req.URL, err = req.URL.Parse(addr + strings.ReplaceAll(url, "/"+urls[1]+"/"+urls[2], ""))
	if err != nil {
		return err
	}
	s.logger.Infof("onRelayHttpReqV2 from %s request to %s", p.Address, req.URL)

	req.Host = req.URL.Host
	req.RequestURI = req.URL.RequestURI()

	if !reqWS {
		resp, err := http.DefaultTransport.RoundTrip(req)
		if err != nil {
			_, _ = stream.Write([]byte(err.Error()))
			return err
		}
		// resp.Write writes whatever response we obtained for our
		// request back to the stream.
		return resp.Write(stream)
	} else {
		var remoteConn net.Conn
		switch req.URL.Scheme {
		case "ws":
			remoteConn, err = net.Dial("tcp", req.URL.Host)
		case "wss":
			remoteConn, err = tls.Dial("tcp", req.URL.Host, &tls.Config{
				InsecureSkipVerify: true,
			})
		}
		if err != nil {
			_, _ = stream.Write([]byte(err.Error()))
			return err
		}
		defer remoteConn.Close()
		b, _ := httputil.DumpRequest(req, false)
		_, err = remoteConn.Write(b)
		if err != nil {
			_, _ = stream.Write([]byte(err.Error()))
			return err
		}
		// response
		respErrCh := make(chan error, 1)
		go func() {
			_, err = io.Copy(stream, remoteConn)
			s.logger.Tracef("onRelayHttpReqV2 from %s io.copy resp err %v", p.Address, err)
			respErrCh <- err
		}()
		// request
		reqErrCh := make(chan error, 1)
		go func() {
			_, err = io.Copy(remoteConn, stream)
			s.logger.Tracef("onRelayHttpReqV2 from %s io.copy req err %v", p.Address, err)
			reqErrCh <- err
		}()
		select {
		case err = <-respErrCh:
			return err
		case err = <-reqErrCh:
			return err
		}
	}
}
