package integration

import (
	"crypto/tls"
	"fmt"
	"strconv"

	"code.cloudfoundry.org/routing-release/gorouter/common/health"

	"code.cloudfoundry.org/routing-release/gorouter/accesslog"
	"code.cloudfoundry.org/routing-release/gorouter/config"
	"code.cloudfoundry.org/routing-release/gorouter/errorwriter"
	"code.cloudfoundry.org/routing-release/gorouter/metrics"
	"code.cloudfoundry.org/routing-release/gorouter/proxy"
	"code.cloudfoundry.org/routing-release/gorouter/registry"
	"code.cloudfoundry.org/routing-release/gorouter/route"
	"code.cloudfoundry.org/routing-release/gorouter/router"
	"code.cloudfoundry.org/routing-release/gorouter/routeservice"
	"code.cloudfoundry.org/routing-release/gorouter/test_util"
	"code.cloudfoundry.org/routing-release/gorouter/varz"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	schemaFakes "code.cloudfoundry.org/routing-release/gorouter/accesslog/schema/fakes"
	"code.cloudfoundry.org/routing-release/gorouter/metrics/fakes"
)

var _ = Describe("AccessLogRecord", func() {
	Measure("Register", func(b Benchmarker) {
		sender := new(fakes.MetricSender)
		batcher := new(fakes.MetricBatcher)
		metricsReporter := &metrics.MetricsReporter{Sender: sender, Batcher: batcher}
		ls := &schemaFakes.FakeLogSender{}
		logger := test_util.NewTestZapLogger("test")
		c, err := config.DefaultConfig()
		Expect(err).ToNot(HaveOccurred())
		r := registry.NewRouteRegistry(logger, c, new(fakes.FakeRouteRegistryReporter))
		combinedReporter := &metrics.CompositeReporter{VarzReporter: varz.NewVarz(r), ProxyReporter: metricsReporter}
		accesslog, err := accesslog.CreateRunningAccessLogger(logger, ls, c)
		Expect(err).ToNot(HaveOccurred())

		ew := errorwriter.NewPlaintextErrorWriter()

		rss, err := router.NewRouteServicesServer()
		Expect(err).ToNot(HaveOccurred())
		var h *health.Health
		proxy.NewProxy(logger, accesslog, ew, c, r, combinedReporter, &routeservice.RouteServiceConfig{},
			&tls.Config{}, &tls.Config{}, h, rss.GetRoundTripper())

		b.Time("RegisterTime", func() {
			for i := 0; i < 1000; i++ {
				str := strconv.Itoa(i)
				r.Register(
					route.Uri(fmt.Sprintf("bench.%s.%s", test_util.LocalhostDNS, str)),
					route.NewEndpoint(&route.EndpointOpts{
						Host:                    "localhost",
						Port:                    uint16(i),
						StaleThresholdInSeconds: -1,
						UseTLS:                  false,
					}),
				)
			}
		})
	}, 10)
})
