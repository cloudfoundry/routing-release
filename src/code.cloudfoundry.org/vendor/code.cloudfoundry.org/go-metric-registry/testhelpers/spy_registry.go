package testhelpers

import (
	"fmt"
	"sort"
	"sync"

	metrics "code.cloudfoundry.org/go-metric-registry"
	"github.com/prometheus/client_golang/prometheus"
)

type SpyMetricsRegistry struct {
	mu           sync.Mutex
	Metrics      map[string]*SpyMetric
	debugMetrics bool
}

func NewMetricsRegistry() *SpyMetricsRegistry {
	return &SpyMetricsRegistry{
		Metrics: make(map[string]*SpyMetric),
	}
}

func (s *SpyMetricsRegistry) RegisterDebugMetrics() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.debugMetrics = true
}

func (s *SpyMetricsRegistry) NewCounter(name, helpText string, opts ...metrics.MetricOption) metrics.Counter {
	s.mu.Lock()
	defer s.mu.Unlock()

	m := newSpyMetric(name, helpText, opts)
	m = s.addMetric(m)

	return m
}

func (s *SpyMetricsRegistry) NewGauge(name, helpText string, opts ...metrics.MetricOption) metrics.Gauge {
	s.mu.Lock()
	defer s.mu.Unlock()

	m := newSpyMetric(name, helpText, opts)
	m = s.addMetric(m)

	return m
}

func (s *SpyMetricsRegistry) NewHistogram(name, helpText string, buckets []float64, opts ...metrics.MetricOption) metrics.Histogram {
	s.mu.Lock()
	defer s.mu.Unlock()

	m := newSpyMetric(name, helpText, opts)
	m.buckets = buckets
	m = s.addMetric(m)

	return m
}

func (p *SpyMetricsRegistry) RemoveGauge(g metrics.Gauge) {
	sm := g.(*SpyMetric)
	p.removeMetric(sm)
}

func (p *SpyMetricsRegistry) RemoveHistogram(h metrics.Histogram) {
	sm := h.(*SpyMetric)
	p.removeMetric(sm)
}

func (p *SpyMetricsRegistry) RemoveCounter(c metrics.Counter) {
	sm := c.(*SpyMetric)
	p.removeMetric(sm)
}

func (s *SpyMetricsRegistry) addMetric(sm *SpyMetric) *SpyMetric {
	n := getMetricName(sm.name, sm.Opts.ConstLabels)

	_, ok := s.Metrics[n]
	if !ok {
		s.Metrics[n] = sm
	}

	return s.Metrics[n]
}

func (s *SpyMetricsRegistry) removeMetric(sm *SpyMetric) {
	n := getMetricName(sm.name, sm.Opts.ConstLabels)

	delete(s.Metrics, n)
}

func (s *SpyMetricsRegistry) GetDebugMetricsEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.debugMetrics
}

func (s *SpyMetricsRegistry) GetMetric(name string, tags map[string]string) *SpyMetric {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := getMetricName(name, tags)

	if m, ok := s.Metrics[n]; ok {
		return m
	}

	panic(fmt.Sprintf("unknown metric: %s", name))
}

// Returns -1 to signify no metric
func (s *SpyMetricsRegistry) GetMetricValue(name string, tags map[string]string) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := getMetricName(name, tags)

	if m, ok := s.Metrics[n]; ok {
		return m.Value()
	}

	return -1
}

func (s *SpyMetricsRegistry) HasMetric(name string, tags map[string]string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := getMetricName(name, tags)
	_, ok := s.Metrics[n]
	return ok
}

func newSpyMetric(name, helpText string, opts []metrics.MetricOption) *SpyMetric {
	sm := &SpyMetric{
		name:     name,
		helpText: helpText,
		Opts: &prometheus.Opts{
			ConstLabels: make(prometheus.Labels),
		},
	}

	for _, o := range opts {
		o(sm.Opts)
	}

	for k := range sm.Opts.ConstLabels {
		sm.keys = append(sm.keys, k)
	}
	sort.Strings(sm.keys)

	return sm
}

type SpyMetric struct {
	mu       sync.Mutex
	value    float64
	buckets  []float64
	name     string
	helpText string

	keys []string
	Opts *prometheus.Opts
}

func (s *SpyMetric) Set(c float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value = c
}

func (s *SpyMetric) Add(c float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value += c
}

func (s *SpyMetric) Observe(c float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value += c
}

func (s *SpyMetric) Value() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value
}

func (s *SpyMetric) HelpText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.helpText
}

func (s *SpyMetric) Buckets() []float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buckets
}

func getMetricName(name string, tags map[string]string) string {
	n := name

	k := make([]string, len(tags))
	for t := range tags {
		k = append(k, t)
	}
	sort.Strings(k)

	for _, key := range k {
		n += fmt.Sprintf("%s_%s", key, tags[key])
	}

	return n
}
