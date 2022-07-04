package debugapi

import (
	favor "github.com/FavorLabs/favorX"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

func newMetricsRegistry() (r *prometheus.Registry) {
	r = prometheus.NewRegistry()

	// register standard metrics
	r.MustRegister(
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{
			Namespace: "system",
		}),
		collectors.NewGoCollector(),
		prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "system",
			Name:      "info",
			Help:      "file system information.",
			ConstLabels: prometheus.Labels{
				"version": favor.Version,
			},
		}),
	)

	return r
}

func (s *Service) MustRegisterMetrics(cs ...prometheus.Collector) {
	s.metricsRegistry.MustRegister(cs...)
}
