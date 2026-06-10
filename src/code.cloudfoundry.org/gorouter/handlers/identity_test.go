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
		cfg, _ = config.DefaultConfig()
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
			cert := generateTestCert("app:should-not-extract")
			request.Header.Set("X-Forwarded-Client-Cert", buildGoRouterXFCCHeader(cert))
		})

		It("calls next without extracting identity, even when a valid XFCC header is present", func() {
			runHandler()
			Expect(nextCalled).To(BeTrue())
			Expect(requestInfo.CallerIdentity).To(BeNil())
		})
	})

	Context("when host is not an mTLS domain", func() {
		BeforeEach(func() {
			request = test_util.NewRequest("GET", "regular.example.com", "/", nil)
			request.TLS = &tls.ConnectionState{}
			cert := generateTestCert("app:should-not-extract")
			request.Header.Set("X-Forwarded-Client-Cert", buildGoRouterXFCCHeader(cert))
		})

		It("calls next without extracting identity, even when a valid XFCC header is present", func() {
			runHandler()
			Expect(nextCalled).To(BeTrue())
			Expect(requestInfo.CallerIdentity).To(BeNil())
		})
	})

	Context("when RequestInfo is not in context", func() {
		BeforeEach(func() {
			cert := generateTestCert("app:should-not-extract")
			request.Header.Set("X-Forwarded-Client-Cert", buildGoRouterXFCCHeader(cert))
		})

		It("calls next without extracting identity or creating a RequestInfo", func() {
			// No NewRequestInfo middleware in front of the handler, so the
			// context has no RequestInfo. Even with a valid XFCC header on an
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
		Context("when X-Forwarded-Client-Cert header is not present", func() {
			It("calls next handler without setting identity", func() {
				runHandler()
				Expect(nextCalled).To(BeTrue())
				Expect(requestInfo.CallerIdentity).To(BeNil())
			})
		})

		Context("with raw format (*.apps.identity domain)", func() {
			Context("with valid cert containing app GUID in OU", func() {
				BeforeEach(func() {
					cert := generateTestCert("app:test-app-guid-123")
					request.Header.Set("X-Forwarded-Client-Cert", buildGoRouterXFCCHeader(cert))
				})

				It("extracts caller identity with app GUID", func() {
					runHandler()
					Expect(nextCalled).To(BeTrue())
					Expect(requestInfo.CallerIdentity).NotTo(BeNil())
					Expect(requestInfo.CallerIdentity.AppGUID).To(Equal("test-app-guid-123"))
				})
			})

			Context("with cert containing multiple OUs", func() {
				BeforeEach(func() {
					cert := generateTestCertWithMultipleOUs([]string{
						"app:another-app-guid",
						"space:test-space",
						"organization:test-org",
					})
					request.Header.Set("X-Forwarded-Client-Cert", buildGoRouterXFCCHeader(cert))
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

			Context("with invalid base64 data", func() {
				BeforeEach(func() {
					request.Header.Set("X-Forwarded-Client-Cert", "not-valid-base64!!!")
				})

				It("does not set caller identity", func() {
					runHandler()
					Expect(nextCalled).To(BeTrue())
					Expect(requestInfo.CallerIdentity).To(BeNil())
				})
			})

			Context("with valid base64 but invalid certificate data", func() {
				BeforeEach(func() {
					request.Header.Set("X-Forwarded-Client-Cert", base64.StdEncoding.EncodeToString([]byte("not a cert")))
				})

				It("does not set caller identity", func() {
					runHandler()
					Expect(nextCalled).To(BeTrue())
					Expect(requestInfo.CallerIdentity).To(BeNil())
				})
			})

			Context("with cert missing app GUID in OU", func() {
				BeforeEach(func() {
					cert := generateTestCertWithMultipleOUs([]string{"space:some-space"})
					request.Header.Set("X-Forwarded-Client-Cert", buildGoRouterXFCCHeader(cert))
				})

				It("does not set caller identity", func() {
					runHandler()
					Expect(nextCalled).To(BeTrue())
					Expect(requestInfo.CallerIdentity).To(BeNil())
				})
			})

			Context("with cert having empty app GUID", func() {
				BeforeEach(func() {
					cert := generateTestCert("app:")
					request.Header.Set("X-Forwarded-Client-Cert", buildGoRouterXFCCHeader(cert))
				})

				It("does not set caller identity", func() {
					runHandler()
					Expect(nextCalled).To(BeTrue())
					Expect(requestInfo.CallerIdentity).To(BeNil())
				})
			})
		})

		Context("with envoy format (envoy.apps.identity domain)", func() {
			BeforeEach(func() {
				request = test_util.NewRequest("GET", "envoy.apps.identity", "/", nil)
				request.TLS = &tls.ConnectionState{}
			})

			Context("with comma-separated DN format", func() {
				BeforeEach(func() {
					xfccHeader := `Hash=abc123;Subject="CN=instance-id,OU=app:envoy-app-guid,OU=space:envoy-space-guid,OU=organization:envoy-org-guid"`
					request.Header.Set("X-Forwarded-Client-Cert", xfccHeader)
				})

				It("extracts all GUIDs from Subject DN", func() {
					runHandler()
					Expect(nextCalled).To(BeTrue())
					Expect(requestInfo.CallerIdentity).NotTo(BeNil())
					Expect(requestInfo.CallerIdentity.AppGUID).To(Equal("envoy-app-guid"))
					Expect(requestInfo.CallerIdentity.SpaceGUID).To(Equal("envoy-space-guid"))
					Expect(requestInfo.CallerIdentity.OrgGUID).To(Equal("envoy-org-guid"))
				})
			})

			Context("with slash-separated DN format", func() {
				BeforeEach(func() {
					xfccHeader := `Hash=abc123;Subject="/CN=instance-id/OU=app:slash-app-guid/OU=space:slash-space-guid/OU=organization:slash-org-guid"`
					request.Header.Set("X-Forwarded-Client-Cert", xfccHeader)
				})

				It("extracts all GUIDs from Subject DN", func() {
					runHandler()
					Expect(nextCalled).To(BeTrue())
					Expect(requestInfo.CallerIdentity).NotTo(BeNil())
					Expect(requestInfo.CallerIdentity.AppGUID).To(Equal("slash-app-guid"))
					Expect(requestInfo.CallerIdentity.SpaceGUID).To(Equal("slash-space-guid"))
					Expect(requestInfo.CallerIdentity.OrgGUID).To(Equal("slash-org-guid"))
				})
			})

			Context("with only app GUID in Subject", func() {
				BeforeEach(func() {
					xfccHeader := `Hash=def456;Subject="CN=instance,OU=app:only-app-guid"`
					request.Header.Set("X-Forwarded-Client-Cert", xfccHeader)
				})

				It("extracts app GUID", func() {
					runHandler()
					Expect(nextCalled).To(BeTrue())
					Expect(requestInfo.CallerIdentity).NotTo(BeNil())
					Expect(requestInfo.CallerIdentity.AppGUID).To(Equal("only-app-guid"))
					Expect(requestInfo.CallerIdentity.SpaceGUID).To(Equal(""))
					Expect(requestInfo.CallerIdentity.OrgGUID).To(Equal(""))
				})
			})

			Context("with Subject but no app GUID", func() {
				BeforeEach(func() {
					xfccHeader := `Hash=ghi789;Subject="CN=instance,OU=space:some-space,OU=organization:some-org"`
					request.Header.Set("X-Forwarded-Client-Cert", xfccHeader)
				})

				It("does not set caller identity (app GUID required)", func() {
					runHandler()
					Expect(nextCalled).To(BeTrue())
					Expect(requestInfo.CallerIdentity).To(BeNil())
				})
			})

			Context("with missing Subject field", func() {
				BeforeEach(func() {
					xfccHeader := `Hash=jkl012;Cert="some-pem-data"`
					request.Header.Set("X-Forwarded-Client-Cert", xfccHeader)
				})

				It("does not set caller identity", func() {
					runHandler()
					Expect(nextCalled).To(BeTrue())
					Expect(requestInfo.CallerIdentity).To(BeNil())
				})
			})

			Context("with malformed Subject field (missing closing quote)", func() {
				BeforeEach(func() {
					xfccHeader := `Hash=jkl012;Subject="CN=instance,OU=app:test-app`
					request.Header.Set("X-Forwarded-Client-Cert", xfccHeader)
				})

				It("does not set caller identity", func() {
					runHandler()
					Expect(nextCalled).To(BeTrue())
					Expect(requestInfo.CallerIdentity).To(BeNil())
				})
			})

			Context("with empty Subject", func() {
				BeforeEach(func() {
					xfccHeader := `Hash=mno345;Subject=""`
					request.Header.Set("X-Forwarded-Client-Cert", xfccHeader)
				})

				It("does not set caller identity", func() {
					runHandler()
					Expect(nextCalled).To(BeTrue())
					Expect(requestInfo.CallerIdentity).To(BeNil())
				})
			})

			Context("with Subject containing extra whitespace", func() {
				BeforeEach(func() {
					xfccHeader := `Hash=pqr678;Subject="CN=instance, OU=app:whitespace-app-guid, OU=space:whitespace-space-guid"`
					request.Header.Set("X-Forwarded-Client-Cert", xfccHeader)
				})

				It("trims whitespace and extracts GUIDs", func() {
					runHandler()
					Expect(nextCalled).To(BeTrue())
					Expect(requestInfo.CallerIdentity).NotTo(BeNil())
					Expect(requestInfo.CallerIdentity.AppGUID).To(Equal("whitespace-app-guid"))
					Expect(requestInfo.CallerIdentity.SpaceGUID).To(Equal("whitespace-space-guid"))
				})
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
