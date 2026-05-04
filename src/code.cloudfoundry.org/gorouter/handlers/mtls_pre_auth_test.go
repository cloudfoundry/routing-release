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
	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/test_util"
)

var _ = Describe("MtlsPreAuth", func() {
	var (
		handler    negroni.Handler
		cfg        *config.Config
		req        *http.Request
		resp       *httptest.ResponseRecorder
		reqInfo    *handlers.RequestInfo
		nextCalled bool
	)

	BeforeEach(func() {
		logger := test_util.NewTestLogger("mtls-pre-auth")
		cfg, _ = config.DefaultConfig()

		// Configure mTLS domains
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

		handler = handlers.NewMtlsPreAuth(cfg, logger.Logger)

		req = test_util.NewRequest("GET", "example.com", "/", nil)
		resp = httptest.NewRecorder()
		reqInfo = &handlers.RequestInfo{}

		// Add RequestInfo to context
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

	createPool := func(ep *route.Endpoint) *route.EndpointPool {
		p := route.NewPool(&route.PoolOpts{
			Host: "backend.apps.mtls.internal",
		})
		p.Put(ep)
		return p
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
		Context("Layer 0: Non-mTLS domain", func() {
			It("passes through for non-mTLS domains", func() {
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
		})

		Context("Layer 0b: SNI/Host mismatch check", func() {
			BeforeEach(func() {
				req.Host = "backend.apps.identity"
			})

			It("returns 421 when ClientCertRequired is false", func() {
				setTLSConnState("backend.apps.identity", "", false)

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeFalse())
				Expect(resp.Code).To(Equal(http.StatusMisdirectedRequest)) // 421
				Expect(reqInfo.TlsSNI).To(Equal("backend.apps.identity"))
			})

			It("returns 421 when MtlsDomain does not match Host (exact mismatch)", func() {
				setTLSConnState("backend.apps.identity", "other.domain", true)

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeFalse())
				Expect(resp.Code).To(Equal(http.StatusMisdirectedRequest)) // 421
			})

			It("returns 421 when MtlsDomain is wildcard but Host doesn't match", func() {
				req.Host = "attacker.evil.com"
				setTLSConnState("good.apps.identity", "*.apps.identity", true)

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeFalse())
				Expect(resp.Code).To(Equal(http.StatusMisdirectedRequest)) // 421
			})

			It("passes when Host matches exact MtlsDomain", func() {
				req.Host = "exact.mtls.domain"
				setTLSConnState("exact.mtls.domain", "exact.mtls.domain", true)
				reqInfo.RoutePool = createPool(route.NewEndpoint(&route.EndpointOpts{
					AppId: "backend-app",
					Host:  "192.168.1.1",
					Port:  8080,
				}))
				reqInfo.CallerIdentity = &handlers.CallerIdentity{AppGUID: "caller"}

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeTrue())
				Expect(resp.Code).To(Equal(http.StatusOK))
			})

			It("passes when Host matches wildcard MtlsDomain", func() {
				req.Host = "backend.apps.identity"
				setTLSConnState("backend.apps.identity", "*.apps.identity", true)
				reqInfo.RoutePool = createPool(route.NewEndpoint(&route.EndpointOpts{
					AppId: "backend-app",
					Host:  "192.168.1.1",
					Port:  8080,
				}))
				reqInfo.CallerIdentity = &handlers.CallerIdentity{AppGUID: "caller"}

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeTrue())
				Expect(resp.Code).To(Equal(http.StatusOK))
			})

			It("sets TlsSNI on reqInfo", func() {
				setTLSConnState("backend.apps.identity", "*.apps.identity", false)

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(reqInfo.TlsSNI).To(Equal("backend.apps.identity"))
			})
		})

		Context("Layer 1: Route lookup", func() {
			BeforeEach(func() {
				req.Host = "backend.apps.identity"
				setTLSConnState("backend.apps.identity", "*.apps.identity", true)
			})

			It("returns 404 when RoutePool is nil", func() {
				reqInfo.RoutePool = nil

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeFalse())
				Expect(resp.Code).To(Equal(http.StatusNotFound))
			})

			It("returns 404 when RoutePool is empty", func() {
				emptyPool := route.NewPool(&route.PoolOpts{Host: "backend.apps.identity"})
				reqInfo.RoutePool = emptyPool

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeFalse())
				Expect(resp.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("Layer 2: Route policy scope check", func() {
			var endpoint *route.Endpoint

			BeforeEach(func() {
				req.Host = "backend.apps.identity"
				setTLSConnState("backend.apps.identity", "*.apps.identity", true)
			})

			It("passes through when RoutePolicyScope is empty (no enforcement)", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: "", // No enforcement
				})
				reqInfo.RoutePool = createPool(endpoint)

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeTrue())
				Expect(resp.Code).To(Equal(http.StatusOK))
			})

			It("passes through when RoutePolicyScope is empty even without CallerIdentity", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: "", // No enforcement
				})
				reqInfo.RoutePool = createPool(endpoint)
				reqInfo.CallerIdentity = nil

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeTrue())
				Expect(resp.Code).To(Equal(http.StatusOK))
			})
		})

		Context("Identity extraction requirement check", func() {
			var endpoint *route.Endpoint

			BeforeEach(func() {
				req.Host = "backend.apps.identity"
				setTLSConnState("backend.apps.identity", "*.apps.identity", true)
			})

			It("returns 403 when CallerIdentity is nil and enforcement is active", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeOrg,
				})
				reqInfo.RoutePool = createPool(endpoint)
				reqInfo.CallerIdentity = nil

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeFalse())
				Expect(resp.Code).To(Equal(http.StatusForbidden))
			})

			It("sets AuthResult when denying due to missing identity", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeOrg,
				})
				reqInfo.RoutePool = createPool(endpoint)
				reqInfo.CallerIdentity = nil

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(reqInfo.AuthResult).ToNot(BeNil())
				Expect(reqInfo.AuthResult.Outcome).To(Equal("denied"))
				Expect(reqInfo.AuthResult.Rule).To(Equal("identity_extraction"))
				Expect(reqInfo.AuthResult.DeniedReason).To(Equal("certificate does not contain CF identity OU fields"))
			})

			It("sets RouteEndpoint for access log when denying due to missing identity", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeOrg,
				})
				reqInfo.RoutePool = createPool(endpoint)
				reqInfo.CallerIdentity = nil
				reqInfo.RouteEndpoint = nil

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(reqInfo.RouteEndpoint).ToNot(BeNil())
				Expect(reqInfo.RouteEndpoint.ApplicationId).To(Equal("backend-app"))
			})

			It("passes when CallerIdentity is present and enforcement is active", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeOrg,
				})
				reqInfo.RoutePool = createPool(endpoint)
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID:   "caller-app",
					SpaceGUID: "caller-space",
					OrgGUID:   "caller-org",
				}

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeTrue())
				Expect(resp.Code).To(Equal(http.StatusOK))
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
	// This tests the exported behavior via NewMtlsPreAuth handler,
	// since domainMatches is not exported
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
				// Configure deep.sub.apps.identity as a separate mTLS domain
				// so the test can verify that it doesn't match *.apps.identity
				Domain:     "*.sub.apps.identity",
				XFCCFormat: "envoy",
				CACerts:    string(certChain.CACertPEM),
			},
			{
				// Configure apps.identity as an exact match so it's treated as mTLS
				Domain:     "apps.identity",
				XFCCFormat: "envoy",
				CACerts:    string(certChain.CACertPEM),
			},
		}
		err := cfg.Process()
		Expect(err).ToNot(HaveOccurred())
		handler = handlers.NewMtlsPreAuth(cfg, logger.Logger)
	})

	testDomainMatch := func(host, sni, mtlsDomain string, clientCertRequired bool, expectedToPass bool) {
		req := test_util.NewRequest("GET", host, "/", nil)
		resp := httptest.NewRecorder()
		reqInfo := &handlers.RequestInfo{}

		endpoint := route.NewEndpoint(&route.EndpointOpts{
			AppId: "backend-app",
			Host:  "192.168.1.1",
			Port:  8080,
		})
		pool := route.NewPool(&route.PoolOpts{Host: host})
		pool.Put(endpoint)
		reqInfo.RoutePool = pool
		reqInfo.CallerIdentity = &handlers.CallerIdentity{AppGUID: "caller"}

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
			// Wildcard should only match single label, not deep.sub.apps.identity
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
})

var _ = Describe("setRouteEndpointForAccessLog", func() {
	// This is tested indirectly through the handler tests above
	// where we verify RouteEndpoint gets set when identity check fails
})
