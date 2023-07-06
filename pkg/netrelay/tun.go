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

	"github.com/FavorLabs/favorX/pkg/logging"
	"github.com/FavorLabs/favorX/pkg/netrelay/pb"
	"github.com/FavorLabs/favorX/pkg/p2p"
	"github.com/FavorLabs/favorX/pkg/p2p/protobuf"
	"github.com/inhies/go-bytesize"
	"github.com/net-byte/vtun/common/cache"
	"github.com/net-byte/vtun/common/netutil"
	"github.com/net-byte/vtun/register"
	"github.com/net-byte/water"
)

type WaterIface struct {
	*water.Interface
	rate *RateService
}

func ifaceNew(c water.Config, rs *RateService) (*WaterIface, error) {
	i, e := water.New(c)
	if e != nil {
		return nil, e
	}
	return &WaterIface{Interface: i, rate: rs}, nil
}

func (s *WaterIface) Write(p []byte) (n int, err error) {
	n, err = s.Interface.Write(p)
	if err != nil {
		return
	}
	if s.rate == nil {
		return
	}
	e := s.rate.WaitN(context.TODO(), n)
	if e != nil {
		logging.Errorf("tun iface write rate: %s", e.Error())
	}
	return
}

func (s *WaterIface) Read(p []byte) (n int, err error) {
	n, err = s.Interface.Read(p)
	if err != nil {
		return
	}
	if s.rate == nil {
		return
	}
	e := s.rate.WaitN(context.TODO(), n)
	if e != nil {
		logging.Errorf("tun iface read rate: %s", e.Error())
	}
	return
}

func (s *Service) CreateTun(config TunConfig) {
	c := water.Config{DeviceType: water.TUN}
	c.PlatformSpecificParams = water.PlatformSpecificParams{}
	o := runtime.GOOS
	if o == "windows" {
		c.PlatformSpecificParams.Name = "vtun"
		c.PlatformSpecificParams.Network = []string{config.CIDR}
	}
	if config.RateEnable {
		s.tunRate = NewRate(s.counter, s.storer, config.SpeedMin, config.SpeedMax, config.RateEveryday)
	}
	iface, err := ifaceNew(c, s.tunRate)
	if err != nil {
		s.logger.Errorf("failed to create tun interface:", err)
		panic(err)
	}
	s.logger.Infof("tun interface created %v", iface.Name())
	s.setRoute(config, iface)
	go s.forwardVpn(iface)
	s.iface = iface
	s.tunConfig = config
}

// setRoute sets the system routes
func (s *Service) setRoute(config TunConfig, iface *WaterIface) {
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

func (s *Service) onVpnTun(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
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
		_ = s.forwardStream(stream, s.tunGroup, streamVpnTun)
		return nil
	}

	packet := make([]byte, tunBuffer)
	for {
		n, err := stream.Read(packet)
		if err != nil {
			s.logger.Infof("vpn client closed with %s", p.Address)
			return err
		}
		s.counter.IncrTunInBytes(n)
		b := packet[:n]
		if key := netutil.GetSrcKey(b); key != "" {
			cache.GetCache().Set(key, stream, 24*time.Hour)
			n, err = s.iface.Write(b)
			if err == nil {
				s.counter.IncrTunOutBytes(n)
			}
		}
	}
}

func (s *Service) forwardVpn(iface *WaterIface) {
	packet := make([]byte, tunBuffer)
	for {
		n, err := iface.Read(packet)
		if err != nil {
			s.logger.Errorf("vpn tun read %s", err)
			break
		}
		s.counter.IncrTunInBytes(n)
		b := packet[:n]
		if key := netutil.GetDstKey(b); key != "" {
			if v, ok := cache.GetCache().Get(key); ok {
				n, err = v.(p2p.Stream).Write(b)
				if err != nil {
					cache.GetCache().Delete(key)
					continue
				}
				s.counter.IncrTunOutBytes(n)
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
		_ = s.forwardStream(stream, s.tunGroup, streamVpnRequest)
		return nil
	}

	w, r := protobuf.NewWriterAndReader(stream)
	var req pb.VpnRequest
	err = r.ReadMsgWithContext(ctx, &req)
	if err != nil {
		return err
	}
	s.counter.IncrTunInBytes(req.Size())
	resp := &pb.VpnResponse{}
	switch req.Pattern {
	case "/test":
		resp.Body = "OK"
		err = w.WriteMsgWithContext(ctx, resp)
	case "/register/pick/ip":
		ip, pl := register.PickClientIP(s.tunConfig.CIDR)
		resp.Body = fmt.Sprintf("%v/%v", ip, pl)
		err = w.WriteMsgWithContext(ctx, resp)
	case "/register/delete/ip":
		if req.Ip != "" {
			register.DeleteClientIP(req.Ip)
		}
		resp.Body = "OK"
		err = w.WriteMsgWithContext(ctx, resp)
	case "/register/keepalive/ip":
		if req.Ip != "" {
			register.KeepAliveClientIP(req.Ip)
		}
		resp.Body = "OK"
		err = w.WriteMsgWithContext(ctx, resp)
	case "/register/list/ip":
		resp.Body = strings.Join(register.ListClientIPs(), "\r\n")
		err = w.WriteMsgWithContext(ctx, resp)
	case "/register/prefix/ipv4":
		_, ipv4Net, e := net.ParseCIDR(s.tunConfig.CIDR)
		if e != nil {
			resp.Body = "error"
		} else {
			resp.Body = ipv4Net.String()
		}
		err = w.WriteMsgWithContext(ctx, resp)
	case "/register/prefix/ipv6":
		_, ipv6Net, e := net.ParseCIDR(s.tunConfig.CIDRv6)
		if e != nil {
			resp.Body = "error"
		} else {
			resp.Body = ipv6Net.String()
		}
		err = w.WriteMsgWithContext(ctx, resp)
	default:
		return errors.New("mismatch vpn pattern")
	}
	if err == nil {
		s.counter.IncrTunOutBytes(resp.Size())
	}
	return
}

type TunStatus struct {
	TodayUse  string `json:"todayUse"`
	Available string `json:"available"`
	Speed     string `json:"speed"`
}

func (s *Service) TunStats() TunStatus {
	if s.tunRate == nil {
		return TunStatus{}
	}
	return TunStatus{
		TodayUse:  bytesize.New(float64(s.tunRate.todayUse)).String(),
		Available: bytesize.New(float64(s.tunRate.availableTotal)).String(),
		Speed:     bytesize.New(float64(s.tunRate.currentSpeed)).String() + "/s",
	}
}
