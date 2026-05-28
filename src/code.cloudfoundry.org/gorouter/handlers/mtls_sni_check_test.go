package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/urfave/negroni/v3"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/handlers"
	"code.cloudfoundry.org/gorouter/test_util"
)

var _ = Describe("MtlsSniCheck", func() {
	var (
		handler    negroni.Handler
		cfg        *config.Config
		req        *http.Request
		resp       *httptest.ResponseRecorder
		reqInfo    *handlers.RequestInfo
		nextCalled bool
	)

	BeforeEach(func() {
		logger := test_util.NewTestLogger("mtls-sni-check")
		cfg, _ = config.DefaultConfig()

		certChain := test_util.CreateSignedCertWithRootCA(test_util.CertNames{SANs: test_util.SubjectAltNames{DNS: "test.com"}})
		cfg.Domains = []config.MtlsDomainConfig{
			{
				Domain:     "*.apps.identity",
				XFCCFormat: "envoy",
				CACerts:    string(certChain.CACertPEM),
			},
			{
				Domain:     "exact.mtls.domain",
				XFCCFormat: "envoy",
				CACerts:    string(certChain.CACertPEM),
			},
		}
		err := cfg.Process()
		Expect(err).ToNot(HaveOccurred())

		handler = handlers.NewMtlsSniCheck(cfg, logger.Logger)

		req = test_util.NewRequest("GET", "example.com", "/", nil)
		resp = httptest.NewRecorder()
		reqInfo = &handlers.RequestInfo{}

		ctx := context.WithValue(req.Context(), handlers.RequestInfoCtxKey, reqInfo)
		req = req.WithContext(ctx)

		nextCalled = false
	})

	nextHandler := func() http.HandlerFunc {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusOK)
		})
	}

	setTLSConnState := func(sni, mtlsDomain string, clientCertRequired bool) {
		connState := &handlers.TLSConnState{
			SNI:                sni,
			MtlsDomain:         mtlsDomain,
			ClientCertRequired: clientCertRequired,
		}
		ctx := handlers.SetTLSConnState(req.Context(), connState)
		req = req.WithContext(ctx)
	}

	Describe("ServeHTTP", func() {
		Context("Non-mTLS domain", func() {
			It("passes through for non-mTLS domains without client cert", func() {
				req.Host = "regular.example.com"

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeTrue())
				Expect(resp.Code).To(Equal(http.StatusOK))
			})

			It("passes through for non-mTLS domains with port", func() {
				req.Host = "regular.example.com:8080"

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeTrue())
				Expect(resp.Code).To(Equal(http.StatusOK))
			})

			It("returns 421 when ClientCertRequired but MtlsDomain does not match non-mTLS host", func() {
				req.Host = "regular.example.com"
				setTLSConnState("backend.apps.identity", "*.apps.identity", true)

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeFalse())
				Expect(resp.Code).To(Equal(http.StatusMisdirectedRequest))
			})
		})

		Context("mTLS domain - SNI/Host mismatch check", func() {
			BeforeEach(func() {
				req.Host = "backend.apps.identity"
			})

			It("returns 421 when ClientCertRequired is false", func() {
				setTLSConnState("backend.apps.identity", "", false)

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeFalse())
				Expect(resp.Code).To(Equal(http.StatusMisdirectedRequest))
				Expect(reqInfo.TlsSNI).To(Equal("backend.apps.identity"))
			})

			It("returns 421 when MtlsDomain does not match Host", func() {
				setTLSConnState("backend.apps.identity", "other.domain", true)

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeFalse())
				Expect(resp.Code).To(Equal(http.StatusMisdirectedRequest))
			})

			It("returns 421 when MtlsDomain is wildcard but Host doesn't match", func() {
				req.Host = "attacker.evil.com"
				setTLSConnState("good.apps.identity", "*.apps.identity", true)

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeFalse())
				Expect(resp.Code).To(Equal(http.StatusMisdirectedRequest))
			})

			It("passes when Host matches wildcard MtlsDomain", func() {
				setTLSConnState("backend.apps.identity", "*.apps.identity", true)

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeTrue())
				Expect(resp.Code).To(Equal(http.StatusOK))
			})

			It("passes when Host matches exact MtlsDomain", func() {
				req.Host = "exact.mtls.domain"
				setTLSConnState("exact.mtls.domain", "exact.mtls.domain", true)

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeTrue())
				Expect(resp.Code).To(Equal(http.StatusOK))
			})

			It("sets TlsSNI on reqInfo", func() {
				setTLSConnState("backend.apps.identity", "*.apps.identity", true)

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(reqInfo.TlsSNI).To(Equal("backend.apps.identity"))
			})
		})

		Context("when RequestInfo is missing from context", func() {
			It("returns 500", func() {
				req = test_util.NewRequest("GET", "example.com", "/", nil)

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeFalse())
				Expect(resp.Code).To(Equal(http.StatusInternalServerError))
			})
		})
	})
})

var _ = Describe("domainMatches", func() {
	// domainMatches is tested via the MtlsSniCheck handler since it's unexported
	var (
		handler negroni.Handler
		cfg     *config.Config
	)

	BeforeEach(func() {
		logger := test_util.NewTestLogger("domain-matches")
		cfg, _ = config.DefaultConfig()
		certChain := test_util.CreateSignedCertWithRootCA(test_util.CertNames{SANs: test_util.SubjectAltNames{DNS: "test.com"}})
		cfg.Domains = []config.MtlsDomainConfig{
			{
				Domain:     "*.apps.identity",
				XFCCFormat: "envoy",
				CACerts:    string(certChain.CACertPEM),
			},
			{
				Domain:     "exact.domain.com",
				XFCCFormat: "envoy",
				CACerts:    string(certChain.CACertPEM),
			},
			{
				Domain:     "wrong.domain.com",
				XFCCFormat: "envoy",
				CACerts:    string(certChain.CACertPEM),
			},
			{
				Domain:     "*.different.com",
				XFCCFormat: "envoy",
				CACerts:    string(certChain.CACertPEM),
			},
			{
				Domain:     "backend.appsxidentity",
				XFCCFormat: "envoy",
				CACerts:    string(certChain.CACertPEM),
			},
			{
				Domain:     "*.sub.apps.identity",
				XFCCFormat: "envoy",
				CACerts:    string(certChain.CACertPEM),
			},
			{
				Domain:     "apps.identity",
				XFCCFormat: "envoy",
				CACerts:    string(certChain.CACertPEM),
			},
		}
		err := cfg.Process()
		Expect(err).ToNot(HaveOccurred())
		handler = handlers.NewMtlsSniCheck(cfg, logger.Logger)
	})

	testDomainMatch := func(host, sni, mtlsDomain string, clientCertRequired bool, expectedToPass bool) {
		req := test_util.NewRequest("GET", host, "/", nil)
		resp := httptest.NewRecorder()
		reqInfo := &handlers.RequestInfo{}

		ctx := context.WithValue(req.Context(), handlers.RequestInfoCtxKey, reqInfo)
		connState := &handlers.TLSConnState{
			SNI:                sni,
			MtlsDomain:         mtlsDomain,
			ClientCertRequired: clientCertRequired,
		}
		ctx = handlers.SetTLSConnState(ctx, connState)
		req = req.WithContext(ctx)

		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusOK)
		})

		handler.ServeHTTP(resp, req, next)

		if expectedToPass {
			Expect(nextCalled).To(BeTrue(), "Expected next handler to be called for %s matching %s", host, mtlsDomain)
			Expect(resp.Code).To(Equal(http.StatusOK))
		} else {
			Expect(nextCalled).To(BeFalse(), "Expected 421 for %s not matching %s", host, mtlsDomain)
			Expect(resp.Code).To(Equal(http.StatusMisdirectedRequest))
		}
	}

	Describe("exact match", func() {
		It("matches when hostname equals domain pattern", func() {
			testDomainMatch("exact.domain.com", "exact.domain.com", "exact.domain.com", true, true)
		})

		It("does not match when hostname differs from domain pattern", func() {
			testDomainMatch("wrong.domain.com", "wrong.domain.com", "exact.domain.com", true, false)
		})
	})

	Describe("wildcard match", func() {
		It("matches single-label subdomain", func() {
			testDomainMatch("backend.apps.identity", "backend.apps.identity", "*.apps.identity", true, true)
		})

		It("matches different single-label subdomain", func() {
			testDomainMatch("frontend.apps.identity", "frontend.apps.identity", "*.apps.identity", true, true)
		})

		It("does NOT match multi-label subdomain", func() {
			testDomainMatch("deep.sub.apps.identity", "deep.sub.apps.identity", "*.apps.identity", true, false)
		})

		It("does not match different domain suffix", func() {
			testDomainMatch("backend.different.com", "backend.different.com", "*.apps.identity", true, false)
		})

		It("does not match when suffix is similar but not exact", func() {
			testDomainMatch("backend.appsxidentity", "backend.appsxidentity", "*.apps.identity", true, false)
		})

		It("does not match bare domain without subdomain", func() {
			testDomainMatch("apps.identity", "apps.identity", "*.apps.identity", true, false)
		})
	})

	Describe("with port in hostname", func() {
		It("matches exact domain with port", func() {
			testDomainMatch("exact.domain.com:8080", "exact.domain.com", "exact.domain.com", true, true)
		})

		It("matches wildcard domain with port", func() {
			testDomainMatch("backend.apps.identity:443", "backend.apps.identity", "*.apps.identity", true, true)
		})
	})

	Describe("case-insensitive matching (Thread 15: hostname not lowercased in domainMatches)", func() {
		// Per RFC 1035, DNS hostnames are case-insensitive, so:
		// - "BACKEND.apps.identity" should match "*.apps.identity"
		// - "backend.APPS.IDENTITY" should match "*.apps.identity"

		It("matches when hostname has uppercase subdomain", func() {
			testDomainMatch("BACKEND.apps.identity", "BACKEND.apps.identity", "*.apps.identity", true, true)
		})

		It("matches when hostname has uppercase suffix", func() {
			testDomainMatch("backend.APPS.IDENTITY", "backend.APPS.IDENTITY", "*.apps.identity", true, true)
		})

		It("matches when hostname is fully uppercase", func() {
			testDomainMatch("BACKEND.APPS.IDENTITY", "BACKEND.APPS.IDENTITY", "*.apps.identity", true, true)
		})

		It("matches exact domain with different casing", func() {
			testDomainMatch("EXACT.DOMAIN.COM", "EXACT.DOMAIN.COM", "exact.domain.com", true, true)
		})

		It("matches when MtlsDomain pattern has mixed case (from config)", func() {
			testDomainMatch("backend.apps.identity", "backend.apps.identity", "*.Apps.Identity", true, true)
		})
	})

	Describe("NewMtlsSniCheck", func() {
		Context("when no mTLS domains are configured", func() {
			It("returns NoopHandler", func() {
				logger := test_util.NewTestLogger("test")
				emptyCfg, _ := config.DefaultConfig()
				Expect(emptyCfg.Domains).To(BeEmpty())

				handler := handlers.NewMtlsSniCheck(emptyCfg, logger.Logger)
				Expect(handler).To(BeIdenticalTo(handlers.NoopHandler))
			})
		})

		Context("when mTLS domains are configured", func() {
			It("returns a real handler", func() {
				logger := test_util.NewTestLogger("test")
				// cfg is already configured with domains in BeforeEach
				handler := handlers.NewMtlsSniCheck(cfg, logger.Logger)
				Expect(handler).NotTo(BeIdenticalTo(handlers.NoopHandler))
			})
		})
	})
})
