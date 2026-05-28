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

		req = test_util.NewRequest("GET", "backend.apps.identity", "/", nil)
		req.Host = "backend.apps.identity"
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
			Host: "backend.apps.identity",
		})
		p.Put(ep)
		return p
	}

	Describe("ServeHTTP", func() {
		Context("non-mTLS domain", func() {
			It("passes through for non-mTLS domains", func() {
				req.Host = "regular.example.com"

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeTrue())
				Expect(resp.Code).To(Equal(http.StatusOK))
			})
		})

		Context("Layer 1: Route lookup", func() {
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
				req = test_util.NewRequest("GET", "backend.apps.identity", "/", nil)

				handler.ServeHTTP(resp, req, nextHandler())

				Expect(nextCalled).To(BeFalse())
				Expect(resp.Code).To(Equal(http.StatusInternalServerError))
			})
		})
	})

	Describe("NewMtlsPreAuth", func() {
		Context("when no mTLS domains are configured", func() {
			It("returns NoopHandler", func() {
				logger := test_util.NewTestLogger("test")
				emptyCfg, _ := config.DefaultConfig()
				Expect(emptyCfg.Domains).To(BeEmpty())

				handler := handlers.NewMtlsPreAuth(emptyCfg, logger.Logger)
				Expect(handler).To(BeIdenticalTo(handlers.NoopHandler))
			})
		})

		Context("when mTLS domains are configured", func() {
			It("returns a real handler", func() {
				logger := test_util.NewTestLogger("test")
				// cfg is already configured with domains in BeforeEach
				handler := handlers.NewMtlsPreAuth(cfg, logger.Logger)
				Expect(handler).NotTo(BeIdenticalTo(handlers.NoopHandler))
			})
		})
	})
})
