package handlers_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/urfave/negroni/v3"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/handlers"
	"code.cloudfoundry.org/gorouter/test_util"
)

var _ = Describe("CfIdentity", func() {
	var (
		handler     negroni.Handler
		cfg         *config.Config
		nextCalled  bool
		nextHandler http.HandlerFunc
		recorder    *httptest.ResponseRecorder
		request     *http.Request
		requestInfo *handlers.RequestInfo
		runHandler  func()
	)

	BeforeEach(func() {
		cfg, _ = test_util.DefaultConfigWithValidNats()
		certChain := test_util.CreateSignedCertWithRootCA(test_util.CertNames{SANs: test_util.SubjectAltNames{DNS: "test.com"}})
		cfg.Domains = []config.MtlsDomainConfig{
			{
				Domain:     "*.apps.identity",
				XFCCFormat: "raw",
				CACerts:    string(certChain.CACertPEM),
			},
			{
				Domain:     "envoy.apps.identity",
				XFCCFormat: "envoy",
				CACerts:    string(certChain.CACertPEM),
			},
		}
		err := cfg.Process()
		Expect(err).ToNot(HaveOccurred())

		handler = handlers.NewCfIdentity(cfg)
		nextCalled = false
		recorder = httptest.NewRecorder()

		nextHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			request = r
		})

		request = test_util.NewRequest("GET", "backend.apps.identity", "/", nil)
		request.TLS = &tls.ConnectionState{}

		// runHandler wires the handler into a negroni chain behind NewRequestInfo
		// so that a RequestInfo is present in the context. The terminal handler
		// captures the resulting RequestInfo so tests can assert whether the
		// handler set (or deliberately did not set) CallerIdentity.
		runHandler = func() {
			reqInfoHandler := handlers.NewRequestInfo()
			n := negroni.New()
			n.Use(reqInfoHandler)
			n.Use(handler)
			n.UseHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				request = r
				var err error
				requestInfo, err = handlers.ContextRequestInfo(r)
				Expect(err).NotTo(HaveOccurred())
			})

			n.ServeHTTP(recorder, request)
		}
	})

	Context("when TLS is not used", func() {
		BeforeEach(func() {
			request.TLS = nil
		})

		It("calls next without extracting identity", func() {
			runHandler()
			Expect(nextCalled).To(BeTrue())
			Expect(requestInfo.CallerIdentity).To(BeNil())
		})
	})

	Context("when host is not an mTLS domain", func() {
		BeforeEach(func() {
			request = test_util.NewRequest("GET", "regular.example.com", "/", nil)
			request.TLS = &tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{generateTestCert("app:should-not-extract")},
			}
		})

		It("calls next without extracting identity, even with a peer certificate present", func() {
			runHandler()
			Expect(nextCalled).To(BeTrue())
			Expect(requestInfo.CallerIdentity).To(BeNil())
		})
	})

	Context("when RequestInfo is not in context", func() {
		BeforeEach(func() {
			request.TLS.PeerCertificates = []*x509.Certificate{generateTestCert("app:should-not-extract")}
		})

		It("calls next without extracting identity or creating a RequestInfo", func() {
			// No NewRequestInfo middleware in front of the handler, so the
			// context has no RequestInfo. Even with a valid peer cert on an
			// mTLS domain, the handler must bail out cleanly without panicking
			// and without storing any identity.
			handler.ServeHTTP(recorder, request, nextHandler)

			Expect(nextCalled).To(BeTrue())
			Expect(recorder.Code).To(Equal(http.StatusOK))
			_, err := handlers.ContextRequestInfo(request)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when RequestInfo is in context", func() {
		Context("when there is no TLS peer certificate", func() {
			It("calls next handler without setting identity", func() {
				runHandler()
				Expect(nextCalled).To(BeTrue())
				Expect(requestInfo.CallerIdentity).To(BeNil())
			})
		})

		Context("with a valid TLS peer certificate containing app GUID in OU", func() {
			BeforeEach(func() {
				request.TLS.PeerCertificates = []*x509.Certificate{generateTestCert("app:test-app-guid-123")}
			})

			It("extracts caller identity with app GUID", func() {
				runHandler()
				Expect(nextCalled).To(BeTrue())
				Expect(requestInfo.CallerIdentity).NotTo(BeNil())
				Expect(requestInfo.CallerIdentity.AppGUID).To(Equal("test-app-guid-123"))
			})
		})

		Context("with a TLS peer certificate containing multiple OUs", func() {
			BeforeEach(func() {
				request.TLS.PeerCertificates = []*x509.Certificate{generateTestCertWithMultipleOUs([]string{
					"app:another-app-guid",
					"space:test-space",
					"organization:test-org",
				})}
			})

			It("extracts all GUIDs", func() {
				runHandler()
				Expect(nextCalled).To(BeTrue())
				Expect(requestInfo.CallerIdentity).NotTo(BeNil())
				Expect(requestInfo.CallerIdentity.AppGUID).To(Equal("another-app-guid"))
				Expect(requestInfo.CallerIdentity.SpaceGUID).To(Equal("test-space"))
				Expect(requestInfo.CallerIdentity.OrgGUID).To(Equal("test-org"))
			})
		})

		Context("with a TLS peer certificate missing app GUID in OU", func() {
			BeforeEach(func() {
				request.TLS.PeerCertificates = []*x509.Certificate{generateTestCertWithMultipleOUs([]string{"space:some-space"})}
			})

			It("does not set caller identity", func() {
				runHandler()
				Expect(nextCalled).To(BeTrue())
				Expect(requestInfo.CallerIdentity).To(BeNil())
			})
		})

		Context("with a TLS peer certificate having empty app GUID", func() {
			BeforeEach(func() {
				request.TLS.PeerCertificates = []*x509.Certificate{generateTestCert("app:")}
			})

			It("does not set caller identity", func() {
				runHandler()
				Expect(nextCalled).To(BeTrue())
				Expect(requestInfo.CallerIdentity).To(BeNil())
			})
		})

		// Regression tests for TNZGOV-18955 / TNZGOV-18956: CallerIdentity
		// must always come from the TLS-verified peer certificate, never from
		// the client-controlled X-Forwarded-Client-Cert header, regardless of
		// what that header contains or how it got there (a forged header
		// smuggled in via a Host-header parser mismatch, or a header forwarded
		// verbatim because an operator configured forwarded_client_cert to
		// forward/always_forward).
		Context("with a forged X-Forwarded-Client-Cert header claiming a different identity than the TLS peer cert", func() {
			BeforeEach(func() {
				request.TLS.PeerCertificates = []*x509.Certificate{generateTestCert("app:real-app-guid")}
				forgedCert := generateTestCertWithMultipleOUs([]string{
					"app:victim-app-guid",
					"space:victim-space-guid",
					"organization:victim-org-guid",
				})
				request.Header.Set("X-Forwarded-Client-Cert", buildGoRouterXFCCHeader(forgedCert))
			})

			It("derives identity from the TLS peer certificate and ignores the header entirely", func() {
				runHandler()
				Expect(nextCalled).To(BeTrue())
				Expect(requestInfo.CallerIdentity).NotTo(BeNil())
				Expect(requestInfo.CallerIdentity.AppGUID).To(Equal("real-app-guid"))
				Expect(requestInfo.CallerIdentity.SpaceGUID).To(BeEmpty())
				Expect(requestInfo.CallerIdentity.OrgGUID).To(BeEmpty())
			})
		})

		Context("with a crafted envoy-format X-Forwarded-Client-Cert header and no TLS peer certificate", func() {
			BeforeEach(func() {
				request = test_util.NewRequest("GET", "envoy.apps.identity", "/", nil)
				request.TLS = &tls.ConnectionState{}
				xfccHeader := `Hash=abc123;Subject="CN=instance-id,OU=app:envoy-app-guid,OU=space:envoy-space-guid,OU=organization:envoy-org-guid"`
				request.Header.Set("X-Forwarded-Client-Cert", xfccHeader)
			})

			It("does not set caller identity, since no verified peer certificate is present", func() {
				runHandler()
				Expect(nextCalled).To(BeTrue())
				Expect(requestInfo.CallerIdentity).To(BeNil())
			})
		})
	})

	Describe("NewCfIdentity", func() {
		Context("when no mTLS domains are configured", func() {
			It("returns NoopHandler", func() {
				emptyCfg, _ := config.DefaultConfig()
				Expect(emptyCfg.Domains).To(BeEmpty())

				handler := handlers.NewCfIdentity(emptyCfg)
				Expect(handler).To(BeIdenticalTo(handlers.NoopHandler))
			})
		})

		Context("when mTLS domains are configured", func() {
			It("returns a real handler", func() {
				// cfg is already configured with domains in BeforeEach
				handler := handlers.NewCfIdentity(cfg)
				Expect(handler).NotTo(BeIdenticalTo(handlers.NoopHandler))
			})
		})
	})
})

// Helper functions for generating test certificates

func generateTestCert(ou string) *x509.Certificate {
	return generateTestCertWithMultipleOUs([]string{ou})
}

func generateTestCertWithMultipleOUs(ous []string) *x509.Certificate {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	Expect(err).NotTo(HaveOccurred())

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:         "test-instance",
			OrganizationalUnit: ous,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	Expect(err).NotTo(HaveOccurred())

	cert, err := x509.ParseCertificate(certDER)
	Expect(err).NotTo(HaveOccurred())

	return cert
}

// buildGoRouterXFCCHeader produces the format that GoRouter's clientcert.go uses:
// raw base64 without PEM markers (produced by sanitize() function)
func buildGoRouterXFCCHeader(cert *x509.Certificate) string {
	return base64.StdEncoding.EncodeToString(cert.Raw)
}
