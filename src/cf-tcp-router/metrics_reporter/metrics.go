package metrics_reporter

import "github.com/cloudfoundry/dropsonde/metrics"

type Value string

func (name Value) Send(value uint64) {
	metrics.SendValue(string(name), float64(value), "Metric")
}

type ProxyValue string

func (name ProxyValue) Send(proxyName string, value uint64) {
	metrics.SendValue(proxyName+"."+string(name), float64(value), "Metric")
}

type ProxyDurationMs string

func (name ProxyDurationMs) Send(proxyName string, duration uint64) {
	metrics.SendValue(proxyName+"."+string(name), float64(duration), "ms")
}

type DurationMs string

func (name DurationMs) Send(duration uint64) {
	metrics.SendValue(string(name), float64(duration), "ms")
}
