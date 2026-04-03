package handlers_test

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/urfave/negroni/v3"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/errorwriter"
	"code.cloudfoundry.org/gorouter/handlers"
	"code.cloudfoundry.org/gorouter/routeservice"
	"code.cloudfoundry.org/gorouter/test_util"
)

var _ = Describe("Clientcert", func() {
	var (
		stripCertNoTLS   = true
		noStripCertNoTLS = false
		stripCertTLS     = true
		noStripCertTLS   = false
		stripCertMTLS    = ""
		xfccSanitizeMTLS = "xfcc"
		certSanitizeMTLS = "cert"

		forceDeleteHeader             = func(req *http.Request) (bool, error) { return true, nil }
		dontForceDeleteHeader         = func(req *http.Request) (bool, error) { return false, nil }
		errorForceDeleteHeader        = func(req *http.Request) (bool, error) { return false, errors.New("forceDelete error") }
		errorForceDeleteHeaderTimeout = func(req *http.Request) (bool, error) {
			return false, fmt.Errorf("forceDelete error: %w", routeservice.ErrExpired)
		}
		skipSanitization     = func(req *http.Request) bool { return true }
		dontSkipSanitization = func(req *http.Request) bool { return false }
		errorWriter          = errorwriter.NewPlaintextErrorWriter()

		logger *test_util.TestLogger
	)

	DescribeTable("Client Cert Error Handling", func(forceDeleteHeaderFunc func(*http.Request) (bool, error), skipSanitizationFunc func(*http.Request) bool, errorCase string) {
		logger = test_util.NewTestLogger("")
		cfg, _ := config.DefaultConfig()
		clientCertHandler := handlers.NewClientCert(skipSanitizationFunc, forceDeleteHeaderFunc, config.SANITIZE_SET, cfg, logger.Logger, errorWriter)

		nextHandlerWasCalled := false
		nextHandler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { nextHandlerWasCalled = true })

		n := negroni.New()
		n.Use(clientCertHandler)
		n.UseHandlerFunc(nextHandler)

		req := test_util.NewRequest("GET", "xyz.com", "", nil)
		rw := httptest.NewRecorder()
		clientCertHandler.ServeHTTP(rw, req, nextHandler)
		Expect(logger.TestSink.Lines()[0]).To(MatchRegexp(
			`{"log_level":[0-9]*,"timestamp":[0-9]+[.][0-9]+,"message":"signature-validation-failed".+}`,
		))

		switch errorCase {
		case "forceDeleteError":
			Expect(logger.TestSink.Lines()[0]).To(MatchRegexp(
				`{"log_level":[0-9]*,"timestamp":[0-9]+[.][0-9]+,"message":"signature-validation-failed","data":{"error":"forceDelete error"}`,
			))
			Expect(rw.Code).To(Equal(http.StatusBadGateway))
		case "routeServiceTimeout":
			Expect(rw.Code).To(Equal(http.StatusGatewayTimeout))
		}

		Expect(rw.Result().Header).NotTo(HaveKey("Connection"))
		Expect(rw.Body).To(ContainSubstring("Failed to validate Route Service Signature"))

		Expect(nextHandlerWasCalled).To(BeFalse())
	},
		Entry("forceDelete returns an error", errorForceDeleteHeader, skipSanitization, "forceDeleteError"),
		Entry("forceDelete returns route service timeout error", errorForceDeleteHeaderTimeout, skipSanitization, "routeServiceTimeout"),
	)

	DescribeTable("Client Cert Result", func(forceDeleteHeaderFunc func(*http.Request) (bool, error), skipSanitizationFunc func(*http.Request) bool, forwardedClientCert string, noTLSCertStrip bool, TLSCertStrip bool, mTLSCertStrip string) {
		logger = test_util.NewTestLogger("test")
		cfg, _ := config.DefaultConfig()
		clientCertHandler := handlers.NewClientCert(skipSanitizationFunc, forceDeleteHeaderFunc, forwardedClientCert, cfg, logger.Logger, errorWriter)

		nextReq := &http.Request{}
		nextHandler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { nextReq = r })

		n := negroni.New()
		n.Use(clientCertHandler)
		n.UseHandlerFunc(nextHandler)

		By("when there is no tls connection", func() {
			req := test_util.NewRequest("GET", "xyz.com", "", nil)
			req.Header.Add("X-Forwarded-Client-Cert", "trusted-xfcc-header")
			rw := httptest.NewRecorder()
			clientCertHandler.ServeHTTP(rw, req, nextHandler)

			if noTLSCertStrip {
				Expect(nextReq.Header).NotTo(HaveKey("X-Forwarded-Client-Cert"))
			} else {
				Expect(nextReq.Header["X-Forwarded-Client-Cert"]).To(Equal([]string{
					"trusted-xfcc-header",
				}))
			}
		})

		By("when there is a tls connection with no client certs", func() {
			tlsCert1 := test_util.CreateCert("client_cert.com")
			servertlsConfig := &tls.Config{
				Certificates: []tls.Certificate{tlsCert1},
			}
			tlsConfig := &tls.Config{InsecureSkipVerify: true}

			server := httptest.NewUnstartedServer(n)
			server.TLS = servertlsConfig
			server.StartTLS()
			defer server.Close()

			transport := &http.Transport{TLSClientConfig: tlsConfig}
			client := &http.Client{Transport: transport}

			req, err := http.NewRequest("GET", server.URL, nil)
			Expect(err).NotTo(HaveOccurred())

			req.Header.Add("X-Forwarded-Client-Cert", "trusted-xfcc-header")
			_, err = client.Do(req)
			Expect(err).ToNot(HaveOccurred())

			if TLSCertStrip {
				Expect(nextReq.Header).NotTo(HaveKey("X-Forwarded-Client-Cert"))
			} else {
				Expect(nextReq.Header["X-Forwarded-Client-Cert"]).To(Equal([]string{
					"trusted-xfcc-header",
				}))
			}
		})

		By("when there is a mtls connection with client certs", func() {
			privKey, certDER := test_util.CreateCertDER("client_cert1.com")
			keyPEM, certPEM := test_util.CreateKeyPairFromDER(certDER, privKey)

			tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
			Expect(err).ToNot(HaveOccurred())

			x509Cert, err := x509.ParseCertificate(certDER)
			Expect(err).ToNot(HaveOccurred())

			certPool := x509.NewCertPool()
			certPool.AddCert(x509Cert)

			servertlsConfig := &tls.Config{
				Certificates: []tls.Certificate{tlsCert},
				ClientCAs:    certPool,
				ClientAuth:   tls.RequestClientCert,
			}
			tlsConfig := &tls.Config{
				Certificates: []tls.Certificate{tlsCert},
				RootCAs:      certPool,
			}

			server := httptest.NewUnstartedServer(n)
			server.TLS = servertlsConfig
			server.StartTLS()
			defer server.Close()

			transport := &http.Transport{TLSClientConfig: tlsConfig}
			client := &http.Client{Transport: transport}

			req, err := http.NewRequest("GET", server.URL, nil)
			Expect(err).NotTo(HaveOccurred())

			req.Header.Add("X-Forwarded-Client-Cert", "trusted-xfcc-header")
			_, err = client.Do(req)
			Expect(err).ToNot(HaveOccurred())

			switch mTLSCertStrip {
			case "":
				Expect(nextReq.Header).NotTo(HaveKey("X-Forwarded-Client-Cert"))
			case "xfcc":
				Expect(nextReq.Header["X-Forwarded-Client-Cert"]).To(Equal([]string{"trusted-xfcc-header"}))
			case "cert":
				Expect(nextReq.Header["X-Forwarded-Client-Cert"]).To(ConsistOf(sanitize(certPEM)))
			default:
				Fail("Unexpected mTLSCertStrip case")
			}
		})
	},
		Entry("when forceDeleteHeader, skipSanitization, and config.SANITIZE_SET", forceDeleteHeader, skipSanitization, config.SANITIZE_SET, stripCertNoTLS, stripCertTLS, stripCertMTLS),
		Entry("when forceDeleteHeader, skipSanitization, and config.FORWARD", forceDeleteHeader, skipSanitization, config.FORWARD, stripCertNoTLS, stripCertTLS, stripCertMTLS),
		Entry("when forceDeleteHeader, skipSanitization, and config.ALWAYS_FORWARD", forceDeleteHeader, skipSanitization, config.ALWAYS_FORWARD, stripCertNoTLS, stripCertTLS, stripCertMTLS),
		Entry("when forceDeleteHeader, dontSkipSanitization, and config.SANITIZE_SET", forceDeleteHeader, dontSkipSanitization, config.SANITIZE_SET, stripCertNoTLS, stripCertTLS, stripCertMTLS),
		Entry("when forceDeleteHeader, dontSkipSanitization, and config.FORWARD", forceDeleteHeader, dontSkipSanitization, config.FORWARD, stripCertNoTLS, stripCertTLS, stripCertMTLS),
		Entry("when forceDeleteHeader, dontSkipSanitization, and config.ALWAYS_FORWARD", forceDeleteHeader, dontSkipSanitization, config.ALWAYS_FORWARD, stripCertNoTLS, stripCertTLS, stripCertMTLS),
		Entry("when dontForceDeleteHeader, skipSanitization, and config.SANITIZE_SET", dontForceDeleteHeader, skipSanitization, config.SANITIZE_SET, noStripCertNoTLS, noStripCertTLS, xfccSanitizeMTLS),
		Entry("when dontForceDeleteHeader, skipSanitization, and config.FORWARD", dontForceDeleteHeader, skipSanitization, config.FORWARD, noStripCertNoTLS, noStripCertTLS, xfccSanitizeMTLS),
		Entry("when dontForceDeleteHeader, skipSanitization, and config.ALWAYS_FORWARD", dontForceDeleteHeader, skipSanitization, config.ALWAYS_FORWARD, noStripCertNoTLS, noStripCertTLS, xfccSanitizeMTLS),
		Entry("when dontForceDeleteHeader, dontSkipSanitization, and config.SANITIZE_SET", dontForceDeleteHeader, dontSkipSanitization, config.SANITIZE_SET, stripCertNoTLS, stripCertTLS, certSanitizeMTLS),
		Entry("when dontForceDeleteHeader, dontSkipSanitization, and config.FORWARD", dontForceDeleteHeader, dontSkipSanitization, config.FORWARD, stripCertNoTLS, stripCertTLS, xfccSanitizeMTLS),
		Entry("when dontForceDeleteHeader, dontSkipSanitization, and config.ALWAYS_FORWARD", dontForceDeleteHeader, dontSkipSanitization, config.ALWAYS_FORWARD, noStripCertNoTLS, noStripCertTLS, xfccSanitizeMTLS),
	)
})

func sanitize(cert []byte) string {
	s := string(cert)
	r := strings.NewReplacer("-----BEGIN CERTIFICATE-----", "",
		"-----END CERTIFICATE-----", "",
		"\n", "")
	return r.Replace(s)
}

var _ = Describe("Clientcert mTLS Domain XFCC Format", func() {
	var (
		dontForceDeleteHeader = func(req *http.Request) (bool, error) { return false, nil }
		dontSkipSanitization  = func(req *http.Request) bool { return false }
		errorWriter           = errorwriter.NewPlaintextErrorWriter()
		logger                *test_util.TestLogger
	)

	Describe("Envoy XFCC Format", func() {
		It("uses Envoy format when configured for mTLS domain", func() {
			logger = test_util.NewTestLogger("test")

			// Create instance identity cert with Diego format OUs
			certChain := test_util.CreateInstanceIdentityCert(test_util.InstanceIdentityCertNames{
				CommonName: "instance-id-123",
				AppGUID:    "app-guid-456",
				SpaceGUID:  "space-guid-789",
				OrgGUID:    "org-guid-abc",
			})

			// Configure mTLS domain with Envoy format
			cfg, err := config.DefaultConfig()
			Expect(err).NotTo(HaveOccurred())

			cfg.Domains = []config.MtlsDomainConfig{{
				Domain:              "*.apps.mtls.internal",
				CACerts:             string(certChain.CACertPEM),
				ForwardedClientCert: config.SANITIZE_SET,
				XFCCFormat:          config.XFCC_FORMAT_ENVOY,
			}}
			err = cfg.Process()
			Expect(err).NotTo(HaveOccurred())

			clientCertHandler := handlers.NewClientCert(dontSkipSanitization, dontForceDeleteHeader, config.SANITIZE_SET, cfg, logger.Logger, errorWriter)

			var capturedXFCC string
			nextHandler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				capturedXFCC = r.Header.Get("X-Forwarded-Client-Cert")
			})

			n := negroni.New()
			n.Use(clientCertHandler)
			n.UseHandlerFunc(nextHandler)

			// Setup mTLS test server
			tlsCert, err := tls.X509KeyPair(certChain.CertPEM, certChain.PrivKeyPEM)
			Expect(err).ToNot(HaveOccurred())

			certPool := x509.NewCertPool()
			certPool.AddCert(certChain.CACert)

			serverTLSConfig := &tls.Config{
				Certificates: []tls.Certificate{tlsCert},
				ClientCAs:    certPool,
				ClientAuth:   tls.RequestClientCert,
			}

			server := httptest.NewUnstartedServer(n)
			server.TLS = serverTLSConfig
			server.StartTLS()
			defer server.Close()

			// Create client with mTLS cert
			clientTLSConfig := &tls.Config{
				Certificates:       []tls.Certificate{tlsCert},
				RootCAs:            certPool,
				InsecureSkipVerify: true, // Test server uses 127.0.0.1 which isn't in cert SANs
			}

			transport := &http.Transport{TLSClientConfig: clientTLSConfig}
			client := &http.Client{Transport: transport}

			// Make request to mTLS domain
			req, err := http.NewRequest("GET", server.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Host = "myapp.apps.mtls.internal"

			_, err = client.Do(req)
			Expect(err).ToNot(HaveOccurred())

			// Verify Envoy format: Hash=<sha256>;Subject="<DN>"
			Expect(capturedXFCC).To(HavePrefix("Hash="))
			Expect(capturedXFCC).To(ContainSubstring(";Subject=\""))

			// Verify Subject contains OUs
			Expect(capturedXFCC).To(ContainSubstring("OU=app:app-guid-456"))
			Expect(capturedXFCC).To(ContainSubstring("OU=space:space-guid-789"))
			Expect(capturedXFCC).To(ContainSubstring("OU=organization:org-guid-abc"))
			Expect(capturedXFCC).To(ContainSubstring("CN=instance-id-123"))

			// Verify it doesn't contain base64-encoded cert (which would be much longer)
			Expect(len(capturedXFCC)).To(BeNumerically("<", 500)) // Envoy format is ~300 bytes
		})

		It("uses raw format when configured for mTLS domain", func() {
			logger = test_util.NewTestLogger("test")

			// Create instance identity cert
			certChain := test_util.CreateInstanceIdentityCert(test_util.InstanceIdentityCertNames{
				CommonName: "instance-id-123",
				AppGUID:    "app-guid-456",
			})

			// Configure mTLS domain with raw format (default)
			cfg, err := config.DefaultConfig()
			Expect(err).NotTo(HaveOccurred())

			cfg.Domains = []config.MtlsDomainConfig{{
				Domain:              "*.apps.mtls.internal",
				CACerts:             string(certChain.CACertPEM),
				ForwardedClientCert: config.SANITIZE_SET,
				XFCCFormat:          config.XFCC_FORMAT_RAW,
			}}
			err = cfg.Process()
			Expect(err).NotTo(HaveOccurred())

			clientCertHandler := handlers.NewClientCert(dontSkipSanitization, dontForceDeleteHeader, config.SANITIZE_SET, cfg, logger.Logger, errorWriter)

			var capturedXFCC string
			nextHandler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				capturedXFCC = r.Header.Get("X-Forwarded-Client-Cert")
			})

			n := negroni.New()
			n.Use(clientCertHandler)
			n.UseHandlerFunc(nextHandler)

			// Setup mTLS test server
			tlsCert, err := tls.X509KeyPair(certChain.CertPEM, certChain.PrivKeyPEM)
			Expect(err).ToNot(HaveOccurred())

			certPool := x509.NewCertPool()
			certPool.AddCert(certChain.CACert)

			serverTLSConfig := &tls.Config{
				Certificates: []tls.Certificate{tlsCert},
				ClientCAs:    certPool,
				ClientAuth:   tls.RequestClientCert,
			}

			server := httptest.NewUnstartedServer(n)
			server.TLS = serverTLSConfig
			server.StartTLS()
			defer server.Close()

			// Create client with mTLS cert
			clientTLSConfig := &tls.Config{
				Certificates:       []tls.Certificate{tlsCert},
				RootCAs:            certPool,
				InsecureSkipVerify: true, // Test server uses 127.0.0.1 which isn't in cert SANs
			}

			transport := &http.Transport{TLSClientConfig: clientTLSConfig}
			client := &http.Client{Transport: transport}

			// Make request to mTLS domain
			req, err := http.NewRequest("GET", server.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			req.Host = "myapp.apps.mtls.internal"

			_, err = client.Do(req)
			Expect(err).ToNot(HaveOccurred())

			// Verify raw format: base64-encoded certificate (no "Hash=" or "Subject=")
			Expect(capturedXFCC).NotTo(HavePrefix("Hash="))
			Expect(capturedXFCC).NotTo(ContainSubstring("Subject="))

			// Raw format is base64-encoded cert, much larger
			Expect(len(capturedXFCC)).To(BeNumerically(">", 1000))
		})

		It("defaults to raw format when xfcc_format is not specified", func() {
			logger = test_util.NewTestLogger("test")

			certChain := test_util.CreateInstanceIdentityCert(test_util.InstanceIdentityCertNames{
				CommonName: "instance-id-123",
				AppGUID:    "app-guid-456",
			})

			// Configure mTLS domain without xfcc_format
			cfg, err := config.DefaultConfig()
			Expect(err).NotTo(HaveOccurred())

			cfg.Domains = []config.MtlsDomainConfig{{
				Domain:              "*.apps.mtls.internal",
				CACerts:             string(certChain.CACertPEM),
				ForwardedClientCert: config.SANITIZE_SET,
				// XFCCFormat not set - should default to "raw"
			}}
			err = cfg.Process()
			Expect(err).NotTo(HaveOccurred())

			// After Process(), XFCCFormat should be set to "raw"
			Expect(cfg.Domains[0].XFCCFormat).To(Equal(config.XFCC_FORMAT_RAW))
		})
	})
})
