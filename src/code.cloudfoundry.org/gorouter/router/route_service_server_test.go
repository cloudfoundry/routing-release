package router_test

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"code.cloudfoundry.org/gorouter/test_util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/router"
)

var _ = Describe("RouteServicesServer", func() {
	var (
		rss     *router.RouteServicesServer
		handler http.Handler
		errChan chan error
		req     *http.Request
		cfg     *config.Config
	)

	BeforeEach(func() {
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		})

		var err error
		cfg, err = config.DefaultConfig()
		Expect(err).NotTo(HaveOccurred())
		cfg.RouteServicesServerPort = test_util.NextAvailPort()

		req, err = http.NewRequest("GET", "/foo", nil)
		Expect(err).NotTo(HaveOccurred())
	})

	JustBeforeEach(func() {
		var err error
		rss, err = router.NewRouteServicesServer(cfg)
		Expect(err).NotTo(HaveOccurred())

		errChan = make(chan error)

		Expect(rss.Serve(handler, errChan)).To(Succeed())
	})

	AfterEach(func() {
		rss.Stop()
		Eventually(errChan).Should(Receive())
	})

	Describe("Serve", func() {
		It("responds to a TLS request using the client cert", func() {
			resp, err := rss.GetRoundTripper().RoundTrip(req)
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusTeapot))
		})

		It("rejects connections that only offer disallowed ECDSA cipher suites", func() {
			rt := rss.GetRoundTripper()
			clientCfg := rt.TLSClientConfig().Clone()
			// These are the three ciphers that must not be negotiable on the internal port.
			clientCfg.CipherSuites = []uint16{
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			}
			// Force TLS 1.2 so cipher suite negotiation applies.
			clientCfg.MaxVersion = tls.VersionTLS12

			_, err := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.RouteServicesServerPort), clientCfg)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("GetRoundTripper", func() {
		It("hardcodes ECDSA GCM cipher suites in the client config", func() {
			clientCfg := rss.GetRoundTripper().TLSClientConfig()
			Expect(clientCfg.CipherSuites).To(ConsistOf(
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			))
		})
	})

	Describe("ReadHeaderTimeout", func() {
		BeforeEach(func() {
			cfg.ReadHeaderTimeout = 100 * time.Millisecond
		})

		It("closes requests when their header write exceeds ReadHeaderTimeout", func() {
			roundTripper := rss.GetRoundTripper()
			conn, err := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.RouteServicesServerPort), roundTripper.TLSClientConfig())
			Expect(err).NotTo(HaveOccurred())
			defer conn.Close()

			writer := bufio.NewWriter(conn)

			fmt.Fprintf(writer, "GET /some-request HTTP/1.1\r\n")

			// started writing headers
			fmt.Fprintf(writer, "Host: localhost\r\n")
			writer.Flush()

			time.Sleep(300 * time.Millisecond)

			fmt.Fprintf(writer, "User-Agent: CustomClient/1.0\r\n")
			writer.Flush()

			// done
			fmt.Fprintf(writer, "\r\n")
			writer.Flush()

			resp := bufio.NewReader(conn)
			_, err = resp.ReadString('\n')
			Expect(err).To(HaveOccurred())
		})
	})
})
