package netrelay

import (
	m "github.com/FavorLabs/favorX/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	TunInBytes         prometheus.Gauge
	TunOutBytes        prometheus.Gauge
	TunSpeedInBytes    prometheus.Gauge
	TunSpeedOutBytes   prometheus.Gauge
	ProxyInBytes       prometheus.Gauge
	ProxyOutBytes      prometheus.Gauge
	ProxySpeedInBytes  prometheus.Gauge
	ProxySpeedOutBytes prometheus.Gauge
}

func newMetrics() metrics {
	subsystem := "netrelay"

	return metrics{
		TunInBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      "tun_in_bytes",
			Help:      "Number of tun in bytes",
		}),
		TunOutBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      "tun_out_bytes",
			Help:      "Number of tun out bytes",
		}),
		TunSpeedInBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      "tun_speed_in_bytes",
			Help:      "Number of tun speed in bytes",
		}),
		TunSpeedOutBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      "tun_speed_out_bytes",
			Help:      "Number of tun speed out bytes",
		}),
		ProxyInBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      "proxy_in_bytes",
			Help:      "Number of proxy in bytes",
		}),
		ProxyOutBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      "proxy_out_bytes",
			Help:      "Number of proxy out bytes",
		}),
		ProxySpeedInBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      "proxy_speed_in_bytes",
			Help:      "Number of proxy speed in bytes",
		}),
		ProxySpeedOutBytes: prometheus.NewGauge(prometheus.GaugeOpts{
			Subsystem: subsystem,
			Name:      "proxy_speed_out_bytes",
			Help:      "Number of proxy speed out bytes",
		}),
	}
}

func (s *Service) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
