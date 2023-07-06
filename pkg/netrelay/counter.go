package netrelay

import (
	"sync/atomic"
	"time"

	"github.com/gogf/gf/v2/util/gconv"
)

type Counter struct {
	metrics            metrics
	totalTunInBytes    uint64
	totalTunOutBytes   uint64
	totalProxyInBytes  uint64
	totalProxyOutBytes uint64
}

func NewCounter(m metrics) *Counter {
	srv := &Counter{
		metrics: m,
	}
	go srv.tunStats()
	go srv.proxyStats()
	return srv
}

func (s *Counter) IncrProxyInBytes(n int) {
	atomic.AddUint64(&s.totalProxyInBytes, uint64(n))
	s.metrics.ProxyInBytes.Add(float64(n))
}

func (s *Counter) IncrProxyOutBytes(n int) {
	atomic.AddUint64(&s.totalProxyOutBytes, uint64(n))
	s.metrics.ProxyOutBytes.Add(float64(n))
}

func (s *Counter) GetProxyInBytes() uint64 {
	return s.totalProxyInBytes
}

func (s *Counter) GetProxyOutBytes() uint64 {
	return s.totalProxyOutBytes
}

func (s *Counter) IncrTunInBytes(n int) {
	atomic.AddUint64(&s.totalTunInBytes, uint64(n))
	s.metrics.TunInBytes.Add(float64(n))
}

func (s *Counter) IncrTunOutBytes(n int) {
	atomic.AddUint64(&s.totalTunOutBytes, uint64(n))
	s.metrics.TunOutBytes.Add(float64(n))
}

func (s *Counter) GetTunInBytes() uint64 {
	return s.totalTunInBytes
}

func (s *Counter) GetTunOutBytes() uint64 {
	return s.totalTunOutBytes
}

func (s *Counter) SetTun(in, out uint64) {
	atomic.StoreUint64(&s.totalTunInBytes, in)
	s.metrics.TunInBytes.Set(float64(in))
	atomic.StoreUint64(&s.totalTunOutBytes, out)
	s.metrics.TunOutBytes.Set(float64(out))
}

func (s *Counter) proxyStats() {
	lastIn := s.GetProxyInBytes()
	lastOut := s.GetProxyOutBytes()
	t := time.NewTicker(time.Second)
	for {
		select {
		case <-t.C:
			r := s.GetProxyInBytes()
			w := s.GetProxyOutBytes()
			s.metrics.ProxySpeedInBytes.Set(gconv.Float64(r - lastIn))
			s.metrics.ProxySpeedOutBytes.Set(gconv.Float64(w - lastOut))
			lastIn = r
			lastOut = w
		}
	}
}

func (s *Counter) tunStats() {
	lastIn := s.GetTunInBytes()
	lastOut := s.GetTunOutBytes()
	t := time.NewTicker(time.Second)
	for {
		select {
		case <-t.C:
			r := s.GetTunInBytes()
			w := s.GetTunOutBytes()
			s.metrics.TunSpeedInBytes.Set(gconv.Float64(r - lastIn))
			s.metrics.TunSpeedOutBytes.Set(gconv.Float64(w - lastOut))
			lastIn = r
			lastOut = w
		}
	}
}
