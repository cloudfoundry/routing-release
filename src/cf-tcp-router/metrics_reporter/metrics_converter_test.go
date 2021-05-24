package metrics_reporter_test

import (
	"code.cloudfoundry.org/routing-release/cf-tcp-router/metrics_reporter"
	"code.cloudfoundry.org/routing-release/cf-tcp-router/metrics_reporter/haproxy_client"
	"code.cloudfoundry.org/routing-release/cf-tcp-router/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Metrics Converter", func() {
	var (
		stats   haproxy_client.HaproxyStats
		metrics *metrics_reporter.MetricsReport
	)

	Context("Convert", func() {
		Context("when aggregating multiple proxies", func() {
			BeforeEach(func() {
				stats = haproxy_client.HaproxyStats{
					{ProxyName: "fake_pxname1_9000",
						CurrentQueued:        10,
						ErrorConnecting:      20,
						AverageQueueTimeMs:   30,
						AverageConnectTimeMs: 25,
						CurrentSessions:      15,
						AverageSessionTimeMs: 9,
					},
					{ProxyName: "fake_pxname2_9001",
						CurrentQueued:        20,
						ErrorConnecting:      20,
						AverageQueueTimeMs:   0,
						AverageConnectTimeMs: 40,
						CurrentSessions:      15,
						AverageSessionTimeMs: 9,
					},
				}
				metrics = metrics_reporter.Convert(stats)
			})

			It("aggregates CurrentQueued", func() {
				Expect(metrics.TotalCurrentQueuedRequests).To(Equal(uint64(30)))
			})

			It("aggregates BackendErrors", func() {
				Expect(metrics.TotalBackendConnectionErrors).To(Equal(uint64(40)))
			})

			It("aggregates AverageQueueTime", func() {
				Expect(metrics.AverageQueueTimeMs).To(Equal(uint64(15)))
			})

			It("aggregates AverageConnectTime", func() {
				Expect(metrics.AverageConnectTimeMs).To(Equal(uint64(32)))
			})

			It("gets stats per proxy", func() {
				expectedProxyKey1 := models.RoutingKey{Port: 9000}
				expectedProxyStats1 := metrics_reporter.ProxyStats{
					ConnectionTime:  25,
					CurrentSessions: 15,
				}
				expectedProxyKey2 := models.RoutingKey{Port: 9001}
				expectedProxyStats2 := metrics_reporter.ProxyStats{
					ConnectionTime:  40,
					CurrentSessions: 15,
				}
				Expect(metrics.ProxyMetrics).Should(HaveKeyWithValue(expectedProxyKey1, expectedProxyStats1))
				Expect(metrics.ProxyMetrics).Should(HaveKeyWithValue(expectedProxyKey2, expectedProxyStats2))
			})
		})

		Context("multiple services in single connection", func() {
			BeforeEach(func() {
				stats = haproxy_client.HaproxyStats{
					{
						ProxyName:            "fake_pxname1_9000",
						CurrentQueued:        10,
						ErrorConnecting:      20,
						AverageQueueTimeMs:   30,
						AverageConnectTimeMs: 25,
						CurrentSessions:      15,
						AverageSessionTimeMs: 9,
					},
					{
						ProxyName:            "fake_pxname2_9000",
						CurrentQueued:        20,
						ErrorConnecting:      20,
						AverageQueueTimeMs:   0,
						AverageConnectTimeMs: 40,
						CurrentSessions:      15,
						AverageSessionTimeMs: 9,
					},
				}
				metrics = metrics_reporter.Convert(stats)
			})

			It("aggregates current session per proxy", func() {
				expectedProxyKey1 := models.RoutingKey{Port: 9000}
				expectedProxyStats1 := metrics_reporter.ProxyStats{
					ConnectionTime:  65,
					CurrentSessions: 30,
				}

				Expect(metrics.ProxyMetrics).Should(HaveKeyWithValue(expectedProxyKey1, expectedProxyStats1))
			})
		})

		Context("non numeric port in proxy name", func() {
			BeforeEach(func() {
				stats = haproxy_client.HaproxyStats{
					{
						ProxyName:            "fake_pxname1_BAD",
						CurrentQueued:        10,
						ErrorConnecting:      20,
						AverageQueueTimeMs:   30,
						AverageConnectTimeMs: 25,
						CurrentSessions:      15,
						AverageSessionTimeMs: 9,
					},
					{
						ProxyName:            "fake_pxname2_9001",
						CurrentQueued:        20,
						ErrorConnecting:      20,
						AverageQueueTimeMs:   0,
						AverageConnectTimeMs: 40,
						CurrentSessions:      15,
						AverageSessionTimeMs: 9,
					},
				}

				metrics = metrics_reporter.Convert(stats)
			})

			It("does not report for invalid proxy name", func() {
				expectedProxyKey1 := models.RoutingKey{Port: 9001}
				expectedProxyKey2 := models.RoutingKey{Port: 0}
				expectedProxyStats1 := metrics_reporter.ProxyStats{
					ConnectionTime:  40,
					CurrentSessions: 15,
				}

				Expect(len(metrics.ProxyMetrics)).Should(Equal(1))
				Expect(metrics.ProxyMetrics).Should(HaveKeyWithValue(expectedProxyKey1, expectedProxyStats1))
				Expect(metrics.ProxyMetrics).ShouldNot(HaveKey(expectedProxyKey2))
			})
		})

		Context("not a proxy name", func() {
			BeforeEach(func() {
				stats = haproxy_client.HaproxyStats{
					{
						ProxyName:            "http-in",
						CurrentQueued:        10,
						ErrorConnecting:      20,
						AverageQueueTimeMs:   30,
						AverageConnectTimeMs: 25,
						CurrentSessions:      15,
						AverageSessionTimeMs: 9,
					},
					{
						ProxyName:            "fake_pxname2_9001",
						CurrentQueued:        20,
						ErrorConnecting:      20,
						AverageQueueTimeMs:   0,
						AverageConnectTimeMs: 40,
						CurrentSessions:      15,
						AverageSessionTimeMs: 9,
					},
				}

				metrics = metrics_reporter.Convert(stats)
			})

			It("does not report for invalid proxy name", func() {
				expectedProxyKey1 := models.RoutingKey{Port: 9001}
				expectedProxyKey2 := models.RoutingKey{Port: 0}
				expectedProxyStats1 := metrics_reporter.ProxyStats{
					ConnectionTime:  40,
					CurrentSessions: 15,
				}

				Expect(len(metrics.ProxyMetrics)).Should(Equal(1))
				Expect(metrics.ProxyMetrics).Should(HaveKeyWithValue(expectedProxyKey1, expectedProxyStats1))
				Expect(metrics.ProxyMetrics).ShouldNot(HaveKey(expectedProxyKey2))
			})
		})

		Context("empty haproxy stats", func() {
			BeforeEach(func() {
				stats = haproxy_client.HaproxyStats{}
				metrics = metrics_reporter.Convert(stats)
			})

			It("returns a nil metric report", func() {
				Expect(metrics).Should(BeNil())
			})
		})
	})
})
