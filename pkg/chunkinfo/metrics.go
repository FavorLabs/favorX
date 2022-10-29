package chunkinfo

import (
	m "github.com/FavorLabs/favorX/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection

	DiscoverRequestCounter       prometheus.Counter
	PyramidRequestCounter        prometheus.Counter
	DiscoverTotalRetrieved       prometheus.Counter
	PyramidTotalRetrieved        prometheus.Counter
	DiscoverTotalErrors          prometheus.Counter
	PyramidTotalErrors           prometheus.Counter
	PyramidTotalTransferred      prometheus.Counter
	PyramidChunkTransferredError prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "chunkinfo"

	return metrics{
		DiscoverRequestCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      "discover_request_counter",
			Help:      "Number of requests to chunk info.",
		}),
		PyramidRequestCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      "pyramid_request_counter",
			Help:      "Total number of errors while pyramid.",
		}),
		DiscoverTotalErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      "discover_total_errors",
			Help:      "Total number of errors while chunk info.",
		}),
		PyramidTotalErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      "pyramid_total_errors",
			Help:      "Total number of errors while pyramid.",
		}),
		DiscoverTotalRetrieved: prometheus.NewCounter(prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      "discover_total_retrieved",
			Help:      "Total  chunk transferred",
		}),
		PyramidTotalRetrieved: prometheus.NewCounter(prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      "total_retrieved",
			Help:      "total pyramid retrieved.",
		}),

		PyramidTotalTransferred: prometheus.NewCounter(prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      "total_transferred",
			Help:      "Total  chunk transferred",
		}),
		PyramidChunkTransferredError: prometheus.NewCounter(prometheus.CounterOpts{
			Subsystem: subsystem,
			Name:      "chunk_transferred_error",
			Help:      "error chunk transferred from peer.",
		}),
	}
}

func (ci *ChunkInfo) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(ci.metrics)
}
