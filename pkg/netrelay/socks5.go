package netrelay

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"

	"github.com/FavorLabs/favorX/pkg/boson"
	"github.com/FavorLabs/favorX/pkg/p2p"
)

const (
	socks5Version = 0x05

	cmdTCPConnect    = 0x01
	cmdTCPBind       = 0x02
	cmdUDPAssociate  = 0x03
	cmdTorResolve    = 0xF0
	cmdTorResolvePTR = 0xF1

	statusSuccess            = 0x00
	statusNetworkUnreachable = 0x03
	statusHostUnreachable    = 0x04
	statusCmdNotSupport      = 0x07

	// ATYPIPv4 is ipv4 address type
	ATYPIPv4 byte = 0x01 // 4 octets
	// ATYPDomain is domain address type
	ATYPDomain byte = 0x03 // The first octet of the address field contains the number of octets of name that follow, there is no terminating NUL octet.
	// ATYPIPv6 is ipv6 address type
	ATYPIPv6 byte = 0x04 // 16 octets
)

var (
	ErrVersion    = errors.New("invalid Version")
	ErrBadRequest = errors.New("bad Request")
)

// Datagram is the UDP packet
type Datagram struct {
	Rsv     []byte // 0x00 0x00
	Frag    byte
	Atyp    byte
	DstAddr []byte
	DstPort []byte // 2 bytes
	Data    []byte
}

// Address return datagram address like ip:xx
func (d *Datagram) Address() string {
	var s string
	if d.Atyp == ATYPDomain {
		s = bytes.NewBuffer(d.DstAddr[1:]).String()
	} else {
		s = net.IP(d.DstAddr).String()
	}
	p := strconv.Itoa(int(binary.BigEndian.Uint16(d.DstPort)))
	return net.JoinHostPort(s, p)
}

// Bytes return []byte
func (d *Datagram) Bytes() []byte {
	b := make([]byte, 0)
	b = append(b, d.Rsv...)
	b = append(b, d.Frag)
	b = append(b, d.Atyp)
	b = append(b, d.DstAddr...)
	b = append(b, d.DstPort...)
	b = append(b, d.Data...)
	return b
}

func (s *Service) newDatagramFromBytes(bb []byte) (*Datagram, error) {
	n := len(bb)
	minl := 4
	if n < minl {
		return nil, ErrBadRequest
	}
	var addr []byte
	if bb[3] == ATYPIPv4 {
		minl += 4
		if n < minl {
			return nil, ErrBadRequest
		}
		addr = bb[minl-4 : minl]
	} else if bb[3] == ATYPIPv6 {
		minl += 16
		if n < minl {
			return nil, ErrBadRequest
		}
		addr = bb[minl-16 : minl]
	} else if bb[3] == ATYPDomain {
		minl += 1
		if n < minl {
			return nil, ErrBadRequest
		}
		l := bb[4]
		if l == 0 {
			return nil, ErrBadRequest
		}
		minl += int(l)
		if n < minl {
			return nil, ErrBadRequest
		}
		addr = bb[minl-int(l) : minl]
		addr = append([]byte{l}, addr...)
	} else {
		return nil, ErrBadRequest
	}
	minl += 2
	if n <= minl {
		return nil, ErrBadRequest
	}
	port := bb[minl-2 : minl]
	data := bb[minl:]
	d := &Datagram{
		Rsv:     bb[0:2],
		Frag:    bb[2],
		Atyp:    bb[3],
		DstAddr: addr,
		DstPort: port,
		Data:    data,
	}
	s.logger.Debugf("socks5 Got Datagram. data: %#v %#v %#v %#v %#v %#v datagram address: %#v\n", d.Rsv, d.Frag, d.Atyp, d.DstAddr, d.DstPort, d.Data, d.Address())
	return d, nil
}

func (s *Service) socks5ProxyTCP(conn net.Conn) {
	defer conn.Close()
	var b [1]byte
	n, err := conn.Read(b[:])
	if err != nil || n != 1 {
		return
	}
	ms := make([]byte, int(b[0]))
	_, err = conn.Read(ms)
	if err != nil {
		return
	}
	_, err = conn.Write([]byte{socks5Version, 0x00})
	if err != nil {
		return
	}

	var b2 [2]byte
	n, err = conn.Read(b2[:])
	if err != nil || n != 2 {
		return
	}
	cmd := b2[1]
	switch cmd {
	case cmdTCPConnect, cmdTorResolve, cmdTorResolvePTR:
		forward, err := s.getForward(s.proxyGroup)
		if err != nil {
			s.logger.Errorf("socks5 get forward peer err %s", err)
			return
		}
		for _, p := range forward {
			err = s.socks5HandleTCP(conn, p)
			if err == nil {
				break
			}
		}
		if err != nil {
			s.logger.Errorf("socks5 forward err %s", err)
		}
	case cmdTCPBind:
		_, err = NewReply(statusCmdNotSupport, ATYPIPv4, []byte{0x00, 0x00, 0x00, 0x00}, []byte{0x00, 0x00}).WriteTo(conn)
		s.logger.Warningf("socks5 not support tcp bind")
	case cmdUDPAssociate:
		if s.socks5UDPConn == nil || s.socks5UDPAddr == nil {
			_, err = NewReply(statusCmdNotSupport, ATYPIPv4, []byte{0x00, 0x00, 0x00, 0x00}, []byte{0x00, 0x00}).WriteTo(conn)
			s.logger.Warningf("socks5 not support udp associate")
			return
		}
		var r *Request
		r, err = NewRequestFrom(conn)
		if err != nil {
			return
		}
		respFunc := func() {
			var p *Reply
			if r.Atyp == ATYPIPv4 || r.Atyp == ATYPDomain {
				p = NewReply(statusHostUnreachable, ATYPIPv4, []byte{0x00, 0x00, 0x00, 0x00}, []byte{0x00, 0x00})
			} else {
				p = NewReply(statusHostUnreachable, ATYPIPv6, net.IPv6zero, []byte{0x00, 0x00})
			}
			_, _ = p.WriteTo(conn)
		}
		if err != nil {
			respFunc()
			return
		}
		a, addr, port, err := ParseAddress(s.socks5UDPAddr.String())
		if err != nil {
			respFunc()
			return
		}
		_, err = NewReply(statusSuccess, a, addr, port).WriteTo(conn)
		if err != nil {
			return
		}
		io.Copy(io.Discard, conn)
		s.logger.Debugf("socks5 A tcp connection that udp %#v associated closed\n", conn.RemoteAddr())
	default:
		_, err = NewReply(statusCmdNotSupport, ATYPIPv4, []byte{0x00, 0x00, 0x00, 0x00}, []byte{0x00, 0x00}).WriteTo(conn)
		s.logger.Warningf("socks5 unknown command %+v", cmd)
	}
}

func (s *Service) socks5ProxyUDP(src *net.UDPAddr, data []byte) {
	d, err := s.newDatagramFromBytes(data)
	if err != nil {
		return
	}
	if d.Frag != 0x00 {
		return
	}

	forward, err := s.getForward(s.proxyGroup)
	if err != nil {
		s.logger.Errorf("socks5 get forward peer err %s", err)
		return
	}
	for _, p := range forward {
		err = s.socks5HandleUDP(src, d.Address(), data, p)
		if err == nil {
			break
		}
	}
	if err != nil {
		s.logger.Errorf("socks5 udp forward err %s", err)
	}
}

func (s *Service) socks5HandleTCP(conn net.Conn, addr boson.Address) (err error) {
	var st p2p.Stream
	if s.route.IsNeighbor(addr) {
		st, err = s.streamer.NewStream(context.Background(), addr, nil, protocolName, protocolVersion, streamSocks5TCP)
	} else {
		st, err = s.streamer.NewConnChainRelayStream(context.Background(), addr, nil, protocolName, protocolVersion, streamSocks5TCP)
	}
	if err != nil {
		return fmt.Errorf("new stream %s", err)
	}
	defer func() {
		if err != nil {
			s.logger.Tracef("socks5 forward to %s err %s", addr, err)
			_ = st.Reset()
		} else {
			_ = st.Close() // must use .Close instead of .FullClose, otherwise it will lead to goroutine leakage
			s.logger.Tracef("socks5 forward to %s stream close", addr)
		}
	}()
	// response
	respErrCh := make(chan error, 1)
	go func() {
		_, err = io.Copy(st, conn)
		if errors.Is(err, net.ErrClosed) {
			err = nil
		}
		if err != nil {
			s.logger.Tracef("socks5 forward io.copy req to %s err %v", addr, err)
		}
		respErrCh <- err
	}()
	// request
	reqErrCh := make(chan error, 1)
	go func() {
		_, err = io.Copy(conn, st)
		if errors.Is(err, net.ErrClosed) {
			err = nil
		}
		if err != nil {
			s.logger.Tracef("socks5 forward io.copy resp from %s err %v", addr, err)
		}
		reqErrCh <- err
	}()
	select {
	case err = <-respErrCh:
		return err
	case err = <-reqErrCh:
		return err
	}
}

func (s *Service) onSocks5TCP(_ context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.Close() // must use .Close instead of .FullClose, otherwise it will lead to goroutine leakage
		}
	}()

	if s.proxyGroup != "" {
		// forward
		s.forwardStream(stream, s.proxyGroup, streamSocks5TCP)
		return nil
	}

	var b [2048]byte
	n, err := stream.Read(b[:])
	if err != nil {
		return err
	}
	if n < 2 {
		return errors.New("failed to read request")
	}
	port := strconv.Itoa(int(binary.BigEndian.Uint16(b[n-2 : n])))
	var addr string
	switch b[1] {
	case ATYPIPv4, ATYPIPv6:
		addr = net.JoinHostPort(net.IP(b[2:n-2]).String(), port)
	case ATYPDomain:
		addr = net.JoinHostPort(string(b[3:n-2]), port)
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		_, err = stream.Write([]byte{socks5Version, statusNetworkUnreachable, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return err
	}
	_, err = stream.Write([]byte{socks5Version, statusSuccess, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	if err != nil {
		return err
	}
	// response
	respErrCh := make(chan error, 1)
	go func() {
		_, err = io.Copy(conn, stream)
		if errors.Is(err, net.ErrClosed) {
			err = nil
		}
		if err != nil {
			s.logger.Debugf("on socks5 io.copy req from %s err %s", p.Address, err)
		}
		respErrCh <- err
	}()
	// request
	reqErrCh := make(chan error, 1)
	go func() {
		_, err = io.Copy(stream, conn)
		if errors.Is(err, net.ErrClosed) {
			err = nil
		}
		if err != nil {
			s.logger.Debugf("on socks5 io.copy resp to %s err %s", p.Address, err)
		}
		reqErrCh <- err
	}()
	select {
	case err = <-respErrCh:
		return err
	case err = <-reqErrCh:
		return err
	}
}

func (s *Service) socks5HandleUDP(src *net.UDPAddr, dst string, data []byte, addr boson.Address) (err error) {
	var st p2p.Stream
	if s.route.IsNeighbor(addr) {
		st, err = s.streamer.NewStream(context.Background(), addr, nil, protocolName, protocolVersion, streamSocks5UDP)
	} else {
		st, err = s.streamer.NewConnChainRelayStream(context.Background(), addr, nil, protocolName, protocolVersion, streamSocks5UDP)
	}
	if err != nil {
		return fmt.Errorf("new stream %s", err)
	}
	defer func() {
		if err != nil {
			s.logger.Tracef("socks5 udp forward to %s err %s", addr, err)
			_ = st.Reset()
		} else {
			_ = st.Close() // must use .Close instead of .FullClose, otherwise it will lead to goroutine leakage
			s.logger.Tracef("socks5 udp forward to %s stream close", addr)
		}
	}()
	a, adr, port, err := ParseAddress(dst)
	if err != nil {
		return err
	}
	_, err = st.Write(data)
	if err != nil {
		return err
	}
	var b [65507]byte
	n, e := st.Read(b[:])
	if e != nil {
		return e
	}
	d1 := NewDatagram(a, adr, port, b[0:n])
	_, _ = s.socks5UDPConn.WriteToUDP(d1.Bytes(), src)
	return
}

func (s *Service) onSocks5UDP(_ context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	defer func() {
		if err != nil {
			s.logger.Tracef("onSocks5 udp from %s err %s", p.Address, err)
			_ = stream.Reset()
		} else {
			_ = stream.Close() // must use .Close instead of .FullClose, otherwise it will lead to goroutine leakage
			s.logger.Tracef("onSocks5 udp from %s stream close", p.Address)
		}
	}()

	if s.proxyGroup != "" {
		// forward
		s.forwardStream(stream, s.proxyGroup, streamSocks5UDP)
		return nil
	}

	b := make([]byte, 65507)
	n, err := stream.Read(b[:])
	if err != nil {
		return err
	}
	d, err := s.newDatagramFromBytes(b[:n])
	if err != nil {
		return
	}
	if d.Frag != 0x00 {
		return
	}
	dst := d.Address()
	raddr, err := net.ResolveUDPAddr("udp", dst)
	if err != nil {
		return err
	}
	rc, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return err
	}
	_, err = rc.Write(d.Data)
	if err != nil {
		return err
	}
	n, e := rc.Read(b[:])
	if e != nil {
		return e
	}
	_, err = stream.Write(b[0:n])
	return err
}

func NewDatagram(atyp byte, dstaddr []byte, dstport []byte, data []byte) *Datagram {
	if atyp == ATYPDomain {
		dstaddr = append([]byte{byte(len(dstaddr))}, dstaddr...)
	}
	return &Datagram{
		Rsv:     []byte{0x00, 0x00},
		Frag:    0x00,
		Atyp:    atyp,
		DstAddr: dstaddr,
		DstPort: dstport,
		Data:    data,
	}
}

// ParseAddress format address x.x.x.x:xx to raw address.
// addr contains domain length
func ParseAddress(address string) (a byte, addr []byte, port []byte, err error) {
	var h, p string
	h, p, err = net.SplitHostPort(address)
	if err != nil {
		return
	}
	ip := net.ParseIP(h)
	if ip4 := ip.To4(); ip4 != nil {
		a = ATYPIPv4
		addr = []byte(ip4)
	} else if ip6 := ip.To16(); ip6 != nil {
		a = ATYPIPv6
		addr = []byte(ip6)
	} else {
		a = ATYPDomain
		addr = []byte{byte(len(h))}
		addr = append(addr, []byte(h)...)
	}
	i, _ := strconv.Atoi(p)
	port = make([]byte, 2)
	binary.BigEndian.PutUint16(port, uint16(i))
	return
}

// Request is the request packet
type Request struct {
	Rsv     byte // 0x00
	Atyp    byte
	DstAddr []byte
	DstPort []byte // 2 bytes
}

func NewRequestFrom(r io.Reader) (*Request, error) {
	bb := make([]byte, 2)
	if _, err := io.ReadFull(r, bb); err != nil {
		return nil, err
	}
	var addr []byte
	if bb[1] == ATYPIPv4 {
		addr = make([]byte, 4)
		if _, err := io.ReadFull(r, addr); err != nil {
			return nil, err
		}
	} else if bb[3] == ATYPIPv6 {
		addr = make([]byte, 16)
		if _, err := io.ReadFull(r, addr); err != nil {
			return nil, err
		}
	} else if bb[3] == ATYPDomain {
		dal := make([]byte, 1)
		if _, err := io.ReadFull(r, dal); err != nil {
			return nil, err
		}
		if dal[0] == 0 {
			return nil, ErrBadRequest
		}
		addr = make([]byte, int(dal[0]))
		if _, err := io.ReadFull(r, addr); err != nil {
			return nil, err
		}
		addr = append(dal, addr...)
	} else {
		return nil, ErrBadRequest
	}
	port := make([]byte, 2)
	if _, err := io.ReadFull(r, port); err != nil {
		return nil, err
	}
	return &Request{
		Rsv:     bb[0],
		Atyp:    bb[1],
		DstAddr: addr,
		DstPort: port,
	}, nil
}

// Address return request address like ip:xx
func (r *Request) Address() string {
	var s string
	if r.Atyp == ATYPDomain {
		s = bytes.NewBuffer(r.DstAddr[1:]).String()
	} else {
		s = net.IP(r.DstAddr).String()
	}
	p := strconv.Itoa(int(binary.BigEndian.Uint16(r.DstPort)))
	return net.JoinHostPort(s, p)
}

// Reply is the reply packet
type Reply struct {
	Ver  byte
	Rep  byte
	Rsv  byte // 0x00
	Atyp byte
	// CONNECT socks server's address which used to connect to dst addr
	// BIND ...
	// UDP socks server's address which used to connect to dst addr
	BndAddr []byte
	// CONNECT socks server's port which used to connect to dst addr
	// BIND ...
	// UDP socks server's port which used to connect to dst addr
	BndPort []byte // 2 bytes
}

// NewReply return reply packet can be written into client, bndaddr should not have domain length
func NewReply(rep byte, atyp byte, bndaddr []byte, bndport []byte) *Reply {
	if atyp == ATYPDomain {
		bndaddr = append([]byte{byte(len(bndaddr))}, bndaddr...)
	}
	return &Reply{
		Ver:     socks5Version,
		Rep:     rep,
		Rsv:     0x00,
		Atyp:    atyp,
		BndAddr: bndaddr,
		BndPort: bndport,
	}
}

// WriteTo write reply packet into client
func (r *Reply) WriteTo(w io.Writer) (int64, error) {
	var n int
	i, err := w.Write([]byte{r.Ver, r.Rep, r.Rsv, r.Atyp})
	n = n + i
	if err != nil {
		return int64(n), err
	}
	i, err = w.Write(r.BndAddr)
	n = n + i
	if err != nil {
		return int64(n), err
	}
	i, err = w.Write(r.BndPort)
	n = n + i
	if err != nil {
		return int64(n), err
	}
	return int64(n), nil
}

// UDPExchange used to store client address and remote connection
type UDPExchange struct {
	ClientAddr *net.UDPAddr
	RemoteConn *net.UDPConn
}

func (s *Service) forwardStream(src p2p.Stream, group, streamName string) error {
	forward, err := s.getForward(group)
	if err != nil {
		s.logger.Errorf("socks5 get forward peer err %s", err)
		return err
	}
	var st p2p.Stream
	for _, p := range forward {
		if s.route.IsNeighbor(p) {
			st, err = s.streamer.NewStream(context.Background(), p, nil, protocolName, protocolVersion, streamName)
		} else {
			st, err = s.streamer.NewConnChainRelayStream(context.Background(), p, nil, protocolName, protocolVersion, streamName)
		}
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}
	defer st.Close()
	go io.Copy(src, st)
	io.Copy(st, src)
	return nil
}
