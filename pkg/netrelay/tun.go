package netrelay

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/FavorLabs/favorX/pkg/netrelay/pb"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/p2p/protobuf"
	"github.com/net-byte/vtun/common/cache"
	"github.com/net-byte/vtun/common/counter"
	"github.com/net-byte/vtun/common/netutil"
	"github.com/net-byte/vtun/register"
	"github.com/net-byte/water"
)

// CreateTun creates a tun interface
func (s *Service) CreateTun(config TunConfig) (iface *water.Interface) {
	c := water.Config{DeviceType: water.TUN}
	c.PlatformSpecificParams = water.PlatformSpecificParams{}
	os := runtime.GOOS
	if os == "windows" {
		c.PlatformSpecificParams.Name = "vtun"
		c.PlatformSpecificParams.Network = []string{config.CIDR}
	}

	iface, err := water.New(c)
	if err != nil {
		s.logger.Errorf("failed to create tun interface:", err)
		panic(err)
	}
	s.logger.Infof("tun interface created %v", iface.Name())
	s.setRoute(config, iface)
	go s.forwardVpn(iface)
	s.iface = iface
	s.tunConfig = config
	return iface
}

// setRoute sets the system routes
func (s *Service) setRoute(config TunConfig, iface *water.Interface) {
	ip, _, err := net.ParseCIDR(config.CIDR)
	if err != nil {
		log.Panicf("tun error cidr %v", config.CIDR)
	}
	_, _, err = net.ParseCIDR(config.CIDRv6)
	if err != nil {
		log.Panicf("tun error ipv6 cidr %v", config.CIDRv6)
	}
	physicalIface := netutil.GetInterface()
	if physicalIface == "" {
		s.logger.Errorf("get physical interface failed, please set nat for vpn by yourself")
	}
	o := runtime.GOOS
	if o == "linux" {
		netutil.ExecCmd("sysctl", "net.ipv4.ip_forward=1")
		netutil.ExecCmd("sysctl", "net.ipv6.conf.all.forwarding=1")
		netutil.ExecCmd("/sbin/ip", "link", "set", "dev", iface.Name(), "mtu", strconv.Itoa(config.MTU))
		netutil.ExecCmd("/sbin/ip", "addr", "add", config.CIDR, "dev", iface.Name())
		netutil.ExecCmd("/sbin/ip", "-6", "addr", "add", config.CIDRv6, "dev", iface.Name())
		netutil.ExecCmd("/sbin/ip", "link", "set", "dev", iface.Name(), "up")
		if physicalIface != "" {
			netutil.ExecCmd("iptables", "-t", "nat", "-A", "POSTROUTING", "-o", physicalIface, "-j", "MASQUERADE")
			netutil.ExecCmd("iptables", "-t", "nat", "-A", "POSTROUTING", "-o", iface.Name(), "-j", "MASQUERADE")
			netutil.ExecCmd("iptables", "-A", "INPUT", "-i", physicalIface, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
			netutil.ExecCmd("iptables", "-A", "INPUT", "-i", iface.Name(), "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
			netutil.ExecCmd("iptables", "-A", "FORWARD", "-j", "ACCEPT")
		}
	} else if o == "darwin" {
		netutil.ExecCmd("sysctl", "net.inet.ip.forwarding=1")
		netutil.ExecCmd("sysctl", "net.inet6.ip6.forwarding=1")
		netutil.ExecCmd("ifconfig", iface.Name(), "mtu", strconv.Itoa(config.MTU))
		netutil.ExecCmd("ifconfig", iface.Name(), "inet", ip.To4().String(), config.ServerIP)
		netutil.ExecCmd("ifconfig", iface.Name(), "inet6", "add", config.CIDRv6)
		netutil.ExecCmd("ifconfig", iface.Name(), "up")
		netutil.ExecCmd("route", "add", config.CIDR, "-interface", iface.Name())
		if physicalIface != "" {
			nat4 := fmt.Sprintf("nat on %s inet from %s:network to any -> (%s)\n", physicalIface, iface.Name(), physicalIface)
			nat6 := fmt.Sprintf("nat on %s inet6 from %s:network to any -> (%s)\n", physicalIface, iface.Name(), physicalIface)
			filename := "/tmp/pf-nat.conf"
			file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0755)
			if err != nil {
				panic(err)
			}
			_, err = file.WriteString(nat4)
			if err != nil {
				panic(err)
			}
			_, err = file.WriteString(nat6)
			if err != nil {
				panic(err)
			}
			netutil.ExecCmd("pfctl", "-d")
			netutil.ExecCmd("pfctl", "-ef", filename)
		}
	}
}

func (s *Service) onVpnTun(_ context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	defer func() {
		if err != nil {
			s.logger.Tracef("onVpnTun from %s err %s", p.Address, err)
			_ = stream.Reset()
		} else {
			_ = stream.Close() // must use .Close instead of .FullClose, otherwise it will lead to goroutine leakage
			s.logger.Tracef("onVpnTun from %s stream close", p.Address)
		}
	}()
	if s.tunGroup != "" {
		s.forwardStream(stream, s.tunGroup, streamVpnTun)
		return nil
	}

	packet := make([]byte, 64*1024)
	for {
		n, err := stream.Read(packet)
		if err != nil {
			return err
		}
		b := packet[:n]
		if key := netutil.GetSrcKey(b); key != "" {
			cache.GetCache().Set(key, stream, 24*time.Hour)
			counter.IncrReadBytes(n)
			s.iface.Write(b)
		}
	}
}

func (s *Service) forwardVpn(iface *water.Interface) {
	packet := make([]byte, 64*1024)
	for {
		n, err := iface.Read(packet)
		if err != nil {
			s.logger.Errorf("vpn tun read %s", err)
			break
		}
		b := packet[:n]
		if key := netutil.GetDstKey(b); key != "" {
			if v, ok := cache.GetCache().Get(key); ok {
				_, err = v.(p2p.Stream).Write(b)
				if err != nil {
					cache.GetCache().Delete(key)
					continue
				}
				counter.IncrWrittenBytes(n)
			}
		}
	}
}

func (s *Service) onVpnRequest(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	defer func() {
		if err != nil {
			s.logger.Tracef("onVpnRequest from %s err %s", p.Address, err)
			_ = stream.Reset()
		} else {
			_ = stream.Close() // must use .Close instead of .FullClose, otherwise it will lead to goroutine leakage
			s.logger.Tracef("onVpnRequest from %s stream close", p.Address)
		}
	}()

	if s.tunGroup != "" {
		s.forwardStream(stream, s.tunGroup, streamVpnRequest)
		return nil
	}

	w, r := protobuf.NewWriterAndReader(stream)
	var req pb.VpnRequest
	err = r.ReadMsgWithContext(ctx, &req)
	if err != nil {
		return err
	}
	switch req.Pattern {
	case "/test":
		err = w.WriteMsgWithContext(ctx, &pb.VpnResponse{Body: "OK"})
	case "/register/pick/ip":
		ip, pl := register.PickClientIP(s.tunConfig.CIDR)
		resp := fmt.Sprintf("%v/%v", ip, pl)
		err = w.WriteMsgWithContext(ctx, &pb.VpnResponse{Body: resp})
	case "/register/delete/ip":
		if req.Ip != "" {
			register.DeleteClientIP(req.Ip)
		}
		err = w.WriteMsgWithContext(ctx, &pb.VpnResponse{Body: "OK"})
	case "/register/keepalive/ip":
		if req.Ip != "" {
			register.KeepAliveClientIP(req.Ip)
		}
		err = w.WriteMsgWithContext(ctx, &pb.VpnResponse{Body: "OK"})
	case "/register/list/ip":
		resp := strings.Join(register.ListClientIPs(), "\r\n")
		err = w.WriteMsgWithContext(ctx, &pb.VpnResponse{Body: resp})
	case "/register/prefix/ipv4":
		_, ipv4Net, e := net.ParseCIDR(s.tunConfig.CIDR)
		var resp string
		if e != nil {
			resp = "error"
		} else {
			resp = ipv4Net.String()
		}
		err = w.WriteMsgWithContext(ctx, &pb.VpnResponse{Body: resp})
	case "/register/prefix/ipv6":
		_, ipv6Net, e := net.ParseCIDR(s.tunConfig.CIDRv6)
		var resp string
		if e != nil {
			resp = "error"
		} else {
			resp = ipv6Net.String()
		}
		err = w.WriteMsgWithContext(ctx, &pb.VpnResponse{Body: resp})
	default:
		return errors.New("mismatch vpn pattern")
	}
	return
}
