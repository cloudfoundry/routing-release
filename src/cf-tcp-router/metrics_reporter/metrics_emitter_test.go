package metrics_reporter_test

import (
	"code.cloudfoundry.org/routing-release/cf-tcp-router/metrics_reporter"
	"code.cloudfoundry.org/routing-release/cf-tcp-router/models"
	"github.com/cloudfoundry/dropsonde/metric_sender/fake"
	"github.com/cloudfoundry/dropsonde/metrics"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MetricsEmitter", func() {
	Describe("Emit", func() {
		var (
			sender  *fake.FakeMetricSender
			emitter metrics_reporter.MetricsEmitter
		)

		BeforeEach(func() {
			sender = fake.NewFakeMetricSender()
			metrics.Initialize(sender, nil)
			emitter = metrics_reporter.NewMetricsEmitter()
		})

		Context("when emitting valid metrics report", func() {
			var (
				metricsReport metrics_reporter.MetricsReport
			)

			BeforeEach(func() {
				metricsReport = metrics_reporter.MetricsReport{
					TotalCurrentQueuedRequests:   10,
					TotalBackendConnectionErrors: 1,
					AverageQueueTimeMs:           100,
					AverageConnectTimeMs:         1000,
					ProxyMetrics: map[models.RoutingKey]metrics_reporter.ProxyStats{
						models.RoutingKey{Port: 9000}: metrics_reporter.ProxyStats{
							ConnectionTime:  10,
							CurrentSessions: 50,
						},
						models.RoutingKey{Port: 8000}: metrics_reporter.ProxyStats{
							ConnectionTime:  100,
							CurrentSessions: 500,
						},
					},
				}
			})

			JustBeforeEach(func() {
				emitter.Emit(&metricsReport)
			})

			It("emits TotalCurrentQueuedRequests", func() {
				Eventually(func() fake.Metric {
					return sender.GetValue("TotalCurrentQueuedRequests")
				}).Should(Equal(fake.Metric{Value: float64(10), Unit: "Metric"}))
			})

			It("emits TotalBackendConnectionErrors", func() {
				Eventually(func() fake.Metric {
					return sender.GetValue("TotalBackendConnectionErrors")
				}).Should(Equal(fake.Metric{Value: float64(1), Unit: "Metric"}))
			})

			It("emits AverageQueueTimeMs", func() {
				Eventually(func() fake.Metric {
					return sender.GetValue("AverageQueueTimeMs")
				}).Should(Equal(fake.Metric{Value: float64(100), Unit: "ms"}))
			})

			It("emits AverageConnectTimeMs", func() {
				Eventually(func() fake.Metric {
					return sender.GetValue("AverageConnectTimeMs")
				}).Should(Equal(fake.Metric{Value: float64(1000), Unit: "ms"}))
			})

			It("emits ConnectionTime metrics for each port", func() {
				Eventually(func() fake.Metric {
					return sender.GetValue("9000.ConnectionTime")
				}).Should(Equal(fake.Metric{Value: float64(10), Unit: "ms"}))
				Eventually(func() fake.Metric {
					return sender.GetValue("8000.ConnectionTime")
				}).Should(Equal(fake.Metric{Value: float64(100), Unit: "ms"}))
			})

			It("emits CurrentSessions metrics for each port", func() {
				Eventually(func() fake.Metric {
					return sender.GetValue("9000.CurrentSessions")
				}).Should(Equal(fake.Metric{Value: float64(50), Unit: "Metric"}))
				Eventually(func() fake.Metric {
					return sender.GetValue("8000.CurrentSessions")
				}).Should(Equal(fake.Metric{Value: float64(500), Unit: "Metric"}))
			})
		})

		Context("when nil MetricsReport is passed", func() {
			It("does not emit any metrics", func() {
				emitter.Emit(nil)
				Consistently(func() fake.Metric {
					return sender.GetValue("TotalCurrentQueuedRequests")
				}).Should(Equal(fake.Metric{}))
				Consistently(func() fake.Metric {
					return sender.GetValue("TotalBackendConnectionErrors")
				}).Should(Equal(fake.Metric{}))
				Consistently(func() fake.Metric {
					return sender.GetValue("AverageQueueTimeMs")
				}).Should(Equal(fake.Metric{}))
				Consistently(func() fake.Metric {
					return sender.GetValue("AverageConnectTimeMs")
				}).Should(Equal(fake.Metric{}))
			})
		})
	})
})
