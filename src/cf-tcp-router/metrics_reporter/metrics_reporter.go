package metrics_reporter

import (
	"os"
	"time"

	"code.cloudfoundry.org/routing-release/cf-tcp-router/metrics_reporter/haproxy_client"
	"code.cloudfoundry.org/routing-release/cf-tcp-router/models"
	"code.cloudfoundry.org/clock"
)

type MetricsReport struct {
	TotalCurrentQueuedRequests   uint64
	TotalBackendConnectionErrors uint64
	AverageQueueTimeMs           uint64
	AverageConnectTimeMs         uint64
	ProxyMetrics                 map[models.RoutingKey]ProxyStats
}

type ProxyStats struct {
	ConnectionTime  uint64
	CurrentSessions uint64
}

type MetricsReporter struct {
	clock          clock.Clock
	emitInterval   time.Duration
	haproxyClient  haproxy_client.HaproxyClient
	metricsEmitter MetricsEmitter
}

func NewMetricsReporter(clock clock.Clock, haproxyClient haproxy_client.HaproxyClient, metricsEmitter MetricsEmitter, interval time.Duration) *MetricsReporter {
	return &MetricsReporter{
		clock:          clock,
		haproxyClient:  haproxyClient,
		metricsEmitter: metricsEmitter,
		emitInterval:   interval,
	}
}

func (r *MetricsReporter) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	ticker := r.clock.NewTicker(r.emitInterval)
	close(ready)
	for {
		select {
		case <-ticker.C():
			r.emitStats()
		case <-signals:
			ticker.Stop()
			return nil
		}
	}
	return nil
}

func (r *MetricsReporter) emitStats() {
	// get stats
	stats := r.haproxyClient.GetStats()

	if len(stats) > 0 {
		// convert to report
		report := Convert(stats)

		// emit to firehose
		r.metricsEmitter.Emit(report)
	}
}
