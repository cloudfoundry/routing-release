package metrics_reporter

var (
	totalCurrentQueuedRequests   = Value("TotalCurrentQueuedRequests")
	totalBackendConnectionErrors = Value("TotalBackendConnectionErrors")
	averageQueueTimeMs           = DurationMs("AverageQueueTimeMs")
	averageConnectTimeMs         = DurationMs("AverageConnectTimeMs")

	connectionTime  = ProxyDurationMs("ConnectionTime")
	currentSessions = ProxyValue("CurrentSessions")
)

type MetricsEmitter interface {
	Emit(*MetricsReport)
}

type metricsEmitter struct{}

func NewMetricsEmitter() MetricsEmitter {
	return &metricsEmitter{}
}

func (e *metricsEmitter) Emit(r *MetricsReport) {
	if r != nil {
		totalCurrentQueuedRequests.Send(r.TotalCurrentQueuedRequests)
		totalBackendConnectionErrors.Send(r.TotalBackendConnectionErrors)
		averageQueueTimeMs.Send(r.AverageQueueTimeMs)
		averageConnectTimeMs.Send(r.AverageConnectTimeMs)
		for k, v := range r.ProxyMetrics {
			connectionTime.Send(k.String(), v.ConnectionTime)
			currentSessions.Send(k.String(), v.CurrentSessions)
		}
	}
}
