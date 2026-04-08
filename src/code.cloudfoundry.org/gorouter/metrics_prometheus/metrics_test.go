package metrics_prometheus

import (
	"fmt"
	"io"
	"net/http"
	"time"

	metrics "code.cloudfoundry.org/go-metric-registry"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/route"
)

var m *Metrics
var r *metrics.Registry

var _ = Describe("Metrics", func() {

	Context("sends route metrics", func() {
		var endpoint *route.Endpoint

		BeforeEach(func() {
			var config = config.PrometheusConfig{Port: 0}
			r = NewMetricsRegistry(config)
			m = NewMetrics(r, true)
			endpoint = new(route.Endpoint)
		})
		AfterEach(func() {
			m.perRequestMetricsReporting = true
		})

		It("sends number of nats messages received from each component", func() {
			endpoint.Tags = map[string]string{}
			m.CaptureRegistryMessage(endpoint, route.EndpointUpdated.String())
			expected := fmt.Sprintf("registry_message{action=\"%s\",component=\"\"} 1", route.EndpointUpdated.String())
			Expect(getMetrics(r.Port())).To(ContainSubstring(expected))

			m.CaptureRegistryMessage(endpoint, route.EndpointUpdated.String())
			expected = fmt.Sprintf("registry_message{action=\"%s\",component=\"\"} 2", route.EndpointUpdated.String())
			Expect(getMetrics(r.Port())).To(ContainSubstring(expected))
		})

		It("sends number of route unregistration messages received from each component", func() {
			endpoint.Tags = map[string]string{"component": "uaa"}
			m.CaptureUnregistryMessage(endpoint)
			Expect(getMetrics(r.Port())).To(ContainSubstring("registry_message{component=\"uaa\"} 1"))

			m.CaptureUnregistryMessage(endpoint)
			Expect(getMetrics(r.Port())).To(ContainSubstring("registry_message{component=\"uaa\"} 2"))

			endpoint.Tags = map[string]string{"component": "route-emitter"}
			m.CaptureUnregistryMessage(endpoint)
			Expect(getMetrics(r.Port())).To(ContainSubstring("registry_message{component=\"route-emitter\"} 1"))
		})

		It("sends route statistics", func() {
			m.CaptureRouteStats(11, 100)
			Expect(getMetrics(r.Port())).To(ContainSubstring("total_routes 11"))
			Expect(getMetrics(r.Port())).To(ContainSubstring("ms_since_last_registry_update 100"))

			m.CaptureRouteStats(11, 200)
			Expect(getMetrics(r.Port())).To(ContainSubstring("total_routes 11"))
			Expect(getMetrics(r.Port())).To(ContainSubstring("ms_since_last_registry_update 200"))

			m.CaptureRouteStats(15, 200)
			Expect(getMetrics(r.Port())).To(ContainSubstring("total_routes 15"))
			Expect(getMetrics(r.Port())).To(ContainSubstring("ms_since_last_registry_update 200"))
		})

		It("sends the total routes", func() {
			m.CaptureTotalRoutes(12)
			Expect(getMetrics(r.Port())).To(ContainSubstring("total_routes 12"))
		})

		It("sends the lookup time for routing table", func() {
			m.CaptureLookupTime(time.Duration(95) * time.Microsecond)
			Expect(getMetrics(r.Port())).To(ContainSubstring("route_lookup_time 95000"))
		})

		It("sends the gorouter time per request", func() {
			m.CaptureGorouterTime(1)
			Expect(getMetrics(r.Port())).To(ContainSubstring("gorouter_time 1"))

			m.perRequestMetricsReporting = false
			m.CaptureGorouterTime(1)
			Expect(getMetrics(r.Port())).To(ContainSubstring("gorouter_time 1"))
		})

		It("increments the routes pruned metric", func() {
			m.CaptureRoutesPruned(50)
			Expect(getMetrics(r.Port())).To(ContainSubstring(`routes_pruned 50`))
		})

		It("increments the routes registered metric", func() {
			m.CaptureRoutesRegistered()
			Expect(getMetrics(r.Port())).To(ContainSubstring(`routes_registered 1`))
		})
		It("increments the routes unregistered metric", func() {
			m.CaptureRoutesUnregistered()
			Expect(getMetrics(r.Port())).To(ContainSubstring(`routes_unregistered 1`))
		})

		Describe("captures route registration latency", func() {
			It("properly splits the latencies apart", func() {
				m.CaptureRouteRegistrationLatency(1234 * time.Microsecond)
				m.CaptureRouteRegistrationLatency(134 * time.Microsecond)
				Expect(getMetrics(r.Port())).To(ContainSubstring("route_registration_latency 0.134"))
			})
		})
	})
	Context("sends backend errors metrics", func() {
		BeforeEach(func() {
			var config = config.PrometheusConfig{Port: 0}
			r = NewMetricsRegistry(config)
			m = NewMetrics(r, true)
		})
		AfterEach(func() {
			m.perRequestMetricsReporting = true
		})

		It("increments the bad gateway to backend metric", func() {
			m.CaptureBadGateway()
			Expect(getMetrics(r.Port())).To(ContainSubstring("bad_gateways 1"))

			m.CaptureBadGateway()
			Expect(getMetrics(r.Port())).To(ContainSubstring("bad_gateways 2"))
		})

		It("increments the backend invalid id metric", func() {
			m.CaptureBackendInvalidID()
			Expect(getMetrics(r.Port())).To(ContainSubstring("backend_invalid_id 1"))

			m.CaptureBackendInvalidID()
			Expect(getMetrics(r.Port())).To(ContainSubstring("backend_invalid_id 2"))
		})

		It("increments the backend invalid tls cert metric", func() {
			m.CaptureBackendInvalidTLSCert()
			Expect(getMetrics(r.Port())).To(ContainSubstring("backend_invalid_tls_cert 1"))

			m.CaptureBackendInvalidTLSCert()
			Expect(getMetrics(r.Port())).To(ContainSubstring("backend_invalid_tls_cert 2"))
		})

		It("increments the backend tls handshake failed metric", func() {
			m.CaptureBackendTLSHandshakeFailed()
			Expect(getMetrics(r.Port())).To(ContainSubstring("backend_tls_handshake_failed 1"))

			m.CaptureBackendTLSHandshakeFailed()
			Expect(getMetrics(r.Port())).To(ContainSubstring("backend_tls_handshake_failed 2"))
		})
	})
	Context("sends lookup error metrics", func() {
		BeforeEach(func() {
			var config = config.PrometheusConfig{Port: 0}
			r = NewMetricsRegistry(config)
			m = NewMetrics(r, true)
		})
		AfterEach(func() {
			m.perRequestMetricsReporting = true
		})

		It("increments the bad requests metric", func() {
			m.CaptureBadRequest()
			Expect(getMetrics(r.Port())).To(ContainSubstring("rejected_requests 1"))

			m.CaptureBadRequest()
			Expect(getMetrics(r.Port())).To(ContainSubstring("rejected_requests 2"))
		})

		It("increments the empty content length header metric", func() {
			m.CaptureEmptyContentLengthHeader()
			Expect(getMetrics(r.Port())).To(ContainSubstring("empty_content_length_header 1"))
		})

		It("increments the backend exhausted conns metric", func() {
			m.CaptureBackendExhaustedConns()
			Expect(getMetrics(r.Port())).To(ContainSubstring("backend_exhausted_conns 1"))

			m.CaptureBackendExhaustedConns()
			Expect(getMetrics(r.Port())).To(ContainSubstring("backend_exhausted_conns 2"))
		})
	})
	Context("websocket metrics", func() {
		BeforeEach(func() {
			var config = config.PrometheusConfig{Port: 0}
			r = NewMetricsRegistry(config)
			m = NewMetrics(r, true)
		})
		AfterEach(func() {
			m.perRequestMetricsReporting = true
		})

		It("increments the websocket upgrades metric", func() {
			m.CaptureWebSocketUpdate()
			Expect(getMetrics(r.Port())).To(ContainSubstring("websocket_upgrades 1"))
		})

		It("increments the websocket failures metric", func() {
			m.CaptureWebSocketFailure()
			Expect(getMetrics(r.Port())).To(ContainSubstring("websocket_failures 1"))
		})
	})
	Context("increments the round trip metrics", func() {
		var endpoint *route.Endpoint

		BeforeEach(func() {
			var config = config.PrometheusConfig{Port: 0}
			r = NewMetricsRegistry(config)
			m = NewMetrics(r, true)
			endpoint = new(route.Endpoint)
		})
		AfterEach(func() {
			m.perRequestMetricsReporting = true
		})

		It("increments the total requests metric", func() {
			endpoint.Tags = map[string]string{}
			m.CaptureRoutingRequest(endpoint)
			Expect(getMetrics(r.Port())).To(ContainSubstring("total_requests{component=\"\"} 1"))

			m.CaptureRoutingRequest(endpoint)
			Expect(getMetrics(r.Port())).To(ContainSubstring("total_requests{component=\"\"} 2"))
		})

		It("increments the requests metric for the given component", func() {
			endpoint.Tags = map[string]string{"component": "CloudController"}
			m.CaptureRoutingRequest(endpoint)
			Expect(getMetrics(r.Port())).To(ContainSubstring("total_requests{component=\"CloudController\"} 1"))

			m.CaptureRoutingRequest(endpoint)
			Expect(getMetrics(r.Port())).To(ContainSubstring("total_requests{component=\"CloudController\"} 2"))

			endpoint.Tags["component"] = "UAA"
			m.CaptureRoutingRequest(endpoint)
			Expect(getMetrics(r.Port())).To(ContainSubstring("total_requests{component=\"CloudController\"} 2"))
			Expect(getMetrics(r.Port())).To(ContainSubstring("total_requests{component=\"UAA\"} 1"))

			m.CaptureRoutingRequest(endpoint)
			Expect(getMetrics(r.Port())).To(ContainSubstring("total_requests{component=\"UAA\"} 2"))
		})
	})

	Context("increments the response metrics", func() {
		BeforeEach(func() {
			var config = config.PrometheusConfig{Port: 0}
			r = NewMetricsRegistry(config)
			m = NewMetrics(r, true)
		})
		AfterEach(func() {
			m.perRequestMetricsReporting = true
		})

		It("increments the 2XX response metrics", func() {
			m.CaptureRoutingResponse(200)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses{status_group=\"2xx\"} 1"))

			m.CaptureRoutingResponse(200)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses{status_group=\"2xx\"} 2"))
		})

		It("increments the 3XX response metrics", func() {
			m.CaptureRoutingResponse(304)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses{status_group=\"3xx\"} 1"))

			m.CaptureRoutingResponse(300)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses{status_group=\"3xx\"} 2"))
		})

		It("increments the 4XX response metrics", func() {
			m.CaptureRoutingResponse(401)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses{status_group=\"4xx\"} 1"))

			m.CaptureRoutingResponse(401)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses{status_group=\"4xx\"} 2"))
		})

		It("increments the 5XX response metrics", func() {
			m.CaptureRoutingResponse(500)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses{status_group=\"5xx\"} 1"))

			m.CaptureRoutingResponse(504)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses{status_group=\"5xx\"} 2"))
		})

		It("increments the XXX response metrics", func() {
			m.CaptureRoutingResponse(100)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses{status_group=\"xxx\"} 1"))

			m.CaptureRoutingResponse(100)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses{status_group=\"xxx\"} 2"))
		})

		It("increments the XXX response metrics with null response", func() {
			m.CaptureRoutingResponse(0)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses{status_group=\"xxx\"} 1"))

			m.CaptureRoutingResponse(0)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses{status_group=\"xxx\"} 2"))
		})
	})

	Context("increments the response metrics for route services", func() {
		var response http.Response

		BeforeEach(func() {
			var config = config.PrometheusConfig{Port: 0}
			r = NewMetricsRegistry(config)
			m = NewMetrics(r, true)
			response = http.Response{}
		})
		AfterEach(func() {
			m.perRequestMetricsReporting = true
		})

		It("increments the 2XX route services response metrics", func() {
			response.StatusCode = 200
			m.CaptureRouteServiceResponse(&response)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses_route_services{status_group=\"2xx\"} 1"))

			m.CaptureRouteServiceResponse(&response)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses_route_services{status_group=\"2xx\"} 2"))
		})

		It("increments the 3XX response metrics", func() {
			response.StatusCode = 300
			m.CaptureRouteServiceResponse(&response)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses_route_services{status_group=\"3xx\"} 1"))

			response.StatusCode = 304
			m.CaptureRouteServiceResponse(&response)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses_route_services{status_group=\"3xx\"} 2"))
		})

		It("increments the 4XX response metrics", func() {
			response.StatusCode = 401
			m.CaptureRouteServiceResponse(&response)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses_route_services{status_group=\"4xx\"} 1"))

			m.CaptureRouteServiceResponse(&response)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses_route_services{status_group=\"4xx\"} 2"))
		})

		It("increments the 5XX response metrics", func() {
			response.StatusCode = 500
			m.CaptureRouteServiceResponse(&response)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses_route_services{status_group=\"5xx\"} 1"))

			response.StatusCode = 504
			m.CaptureRouteServiceResponse(&response)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses_route_services{status_group=\"5xx\"} 2"))
		})

		It("increments the XXX response metrics", func() {
			response.StatusCode = 100
			m.CaptureRouteServiceResponse(&response)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses_route_services{status_group=\"xxx\"} 1"))

			m.CaptureRouteServiceResponse(&response)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses_route_services{status_group=\"xxx\"} 2"))
		})

		It("increments the XXX response metrics with null response", func() {
			m.CaptureRouteServiceResponse(nil)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses_route_services{status_group=\"xxx\"} 1"))

			m.CaptureRouteServiceResponse(nil)
			Expect(getMetrics(r.Port())).To(ContainSubstring("responses_route_services{status_group=\"xxx\"} 2"))
		})
	})

	Context("increments the route response metrics", func() {
		var endpoint *route.Endpoint

		BeforeEach(func() {
			var config = config.PrometheusConfig{Port: 0}
			r = NewMetricsRegistry(config)
			m = NewMetrics(r, true)
			endpoint = new(route.Endpoint)
		})
		AfterEach(func() {
			m.perRequestMetricsReporting = true
		})

		It("sends the latency", func() {
			endpoint.Tags = map[string]string{"component": "testComponent"}
			m.CaptureRoutingResponseLatency(endpoint, 0, time.Time{}, 2*time.Millisecond)
			Expect(getMetrics(r.Port())).To(ContainSubstring("latency{component=\"testComponent\"} 2"))
			m.CaptureRoutingResponseLatency(endpoint, 0, time.Time{}, 500*time.Microsecond)
			Expect(getMetrics(r.Port())).To(ContainSubstring("latency{component=\"testComponent\"} 0.5"))
		})

		It("does not send the latency if switched off", func() {
			m.perRequestMetricsReporting = false
			m.CaptureRoutingResponseLatency(endpoint, 0, time.Time{}, 2*time.Millisecond)
			Expect(getMetrics(r.Port())).NotTo(ContainSubstring("\nlatency"))
		})

		It("sends the latency for the given component", func() {
			endpoint.Tags = map[string]string{"component": "CloudController"}
			m.CaptureRoutingResponseLatency(endpoint, 0, time.Time{}, 2*time.Millisecond)
			Expect(getMetrics(r.Port())).To(ContainSubstring("latency{component=\"CloudController\"} 2"))
		})

		It("does not send the latency for the given component if switched off", func() {
			m.perRequestMetricsReporting = false
			endpoint.Tags = map[string]string{"component": "CloudController"}
			m.CaptureRoutingResponseLatency(endpoint, 0, time.Time{}, 2*time.Millisecond)
			Expect(getMetrics(r.Port())).NotTo(ContainSubstring("\nlatency"))
		})
	})

	Context("increments the monitor metrics", func() {
		BeforeEach(func() {
			var config = config.PrometheusConfig{Port: 0}
			r = NewMetricsRegistry(config)
			m = NewMetrics(r, false)
		})

		It("sends fd metric", func() {
			m.CaptureFoundFileDescriptors(10)
			Expect(getMetrics(r.Port())).To(ContainSubstring("file_descriptors 10"))
		})

		It("sends NATS metrics", func() {
			m.CaptureNATSBufferedMessages(100)
			Expect(getMetrics(r.Port())).To(ContainSubstring("buffered_messages 100"))

			m.CaptureNATSDroppedMessages(200)
			Expect(getMetrics(r.Port())).To(ContainSubstring("total_dropped_messages 200"))
		})
	})

	Context("observes http metrics", func() {
		BeforeEach(func() {
			var config = config.PrometheusConfig{Port: 0}
			r = NewMetricsRegistry(config)
			m = NewMetrics(r, true)
		})

		It("sends the latency", func() {
			m.CaptureHTTPLatency(2*time.Second, "")
			Expect(getMetrics(r.Port())).To(ContainSubstring("http_latency_seconds{source_id=\"\"} 2"))

			m.CaptureHTTPLatency(500*time.Millisecond, "some-source")
			m.CaptureHTTPLatency(630*time.Millisecond, "some-source")
			Expect(getMetrics(r.Port())).To(ContainSubstring("http_latency_seconds{source_id=\"some-source\"} 0.63"))
		})
	})

	Context("endpoints_per_pool metric", func() {
		BeforeEach(func() {
			var config = config.PrometheusConfig{Port: 0}
			r = NewMetricsRegistry(config)
			m = NewMetrics(r, true)
		})

		It("reports the number of endpoints per pool with correct labels", func() {
			m.CaptureEndpointsPerPool(5, "routeA", "round_robin")
			metricsOutput := getMetrics(r.Port())
			expected := "endpoints_per_pool{lb_algorithm=\"round_robin\",route=\"routeA\"} 5"
			Expect(metricsOutput).To(ContainSubstring(expected))
		})

		It("updates the value for the same label combination", func() {
			m.CaptureEndpointsPerPool(5, "routeA", "round_robin")
			m.CaptureEndpointsPerPool(7, "routeA", "round_robin")
			metricsOutput := getMetrics(r.Port())
			expected := "endpoints_per_pool{lb_algorithm=\"round_robin\",route=\"routeA\"} 7"
			Expect(metricsOutput).To(ContainSubstring(expected))
		})

		It("reports multiple values for different label combinations", func() {
			m.CaptureEndpointsPerPool(5, "routeA", "round_robin")
			m.CaptureEndpointsPerPool(3, "routeB", "least_conn")
			metricsOutput := getMetrics(r.Port())
			expectedA := "endpoints_per_pool{lb_algorithm=\"round_robin\",route=\"routeA\"} 5"
			expectedB := "endpoints_per_pool{lb_algorithm=\"least_conn\",route=\"routeB\"} 3"
			Expect(metricsOutput).To(ContainSubstring(expectedA))
			Expect(metricsOutput).To(ContainSubstring(expectedB))
		})

		It("deletes the metric for a given route and LB algorithm", func() {
			m.CaptureEndpointsPerPool(5, "routeA", "round_robin")
			Expect(getMetrics(r.Port())).To(ContainSubstring("endpoints_per_pool{lb_algorithm=\"round_robin\",route=\"routeA\"} 5"))

			m.UncaptureEndpointsPerPool("routeA", "round_robin")
			Expect(getMetrics(r.Port())).NotTo(ContainSubstring("endpoints_per_pool{lb_algorithm=\"round_robin\",route=\"routeA\"}"))
		})

		It("does nothing when deleting a non-existent label combination", func() {
			m.CaptureEndpointsPerPool(5, "routeA", "round_robin")

			m.UncaptureEndpointsPerPool("routeX", "round_robin")
			Expect(getMetrics(r.Port())).To(ContainSubstring("endpoints_per_pool{lb_algorithm=\"round_robin\",route=\"routeA\"} 5"))
		})
	})
})

func getMetrics(port string) string {
	addr := fmt.Sprintf("http://127.0.0.1:%s/metrics", port)
	resp, err := http.Get(addr) //nolint:gosec
	if err != nil {
		return ""
	}

	respBytes, err := io.ReadAll(resp.Body)
	Expect(err).ToNot(HaveOccurred())

	return string(respBytes)
}
