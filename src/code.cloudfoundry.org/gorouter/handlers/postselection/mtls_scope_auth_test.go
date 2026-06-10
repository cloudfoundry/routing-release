package postselection_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/handlers"
	"code.cloudfoundry.org/gorouter/handlers/postselection"
	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/test_util"
)

var _ = Describe("MtlsScopeAuth", func() {
	var (
		handler  postselection.PostSelectionHandler
		endpoint *route.Endpoint
		reqInfo  *handlers.RequestInfo
		pool     *route.EndpointPool
		cfg      *config.Config
	)

	BeforeEach(func() {
		logger := test_util.NewTestLogger("mtls-scope-auth")
		cfg, _ = config.DefaultConfig()
		// Configure a domain so the handler is active
		certChain := test_util.CreateSignedCertWithRootCA(test_util.CertNames{SANs: test_util.SubjectAltNames{DNS: "test.com"}})
		cfg.Domains = []config.MtlsDomainConfig{
			{
				Domain:  "*.apps.mtls.internal",
				CACerts: string(certChain.CACertPEM),
			},
		}
		cfg.Process()
		handler = postselection.NewMtlsScopeAuth(cfg, logger.Logger)
		reqInfo = &handlers.RequestInfo{}
	})

	createPool := func(ep *route.Endpoint) *route.EndpointPool {
		p := route.NewPool(&route.PoolOpts{
			Host: "backend.apps.mtls.internal",
		})
		p.Put(ep)
		return p
	}

	Describe("Check", func() {
		Context("when RoutePool is nil", func() {
			It("denies with AuthError (defense in depth)", func() {
				reqInfo.RoutePool = nil
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId: "backend-app",
					Host:  "192.168.1.1",
					Port:  8080,
				})

				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				authErr, ok := err.(*postselection.AuthError)
				Expect(ok).To(BeTrue())
				Expect(authErr.Rule).To(Equal("internal_error"))
				Expect(authErr.Reason).To(Equal("route pool missing during authorization"))
			})
		})

		Context("when RoutePolicyScope is empty", func() {
			It("returns nil (no enforcement active)", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: "", // No enforcement
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool

				err := handler.Check(endpoint, reqInfo)
				Expect(err).To(BeNil())
			})
		})

		Context("when CallerIdentity is nil", func() {
			It("returns AuthError (defense in depth)", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeOrg,
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = nil

				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				authErr, ok := err.(*postselection.AuthError)
				Expect(ok).To(BeTrue())
				Expect(authErr.Rule).To(Equal("domain:no_caller_identity"))
				Expect(authErr.Reason).To(Equal("no caller identity present"))
				Expect(authErr.HTTPStatus).To(Equal(http.StatusForbidden))
			})
		})

		// ── Scope: any ────────────────────────────────────────────────

		Context("with scope=any", func() {
			It("allows any authenticated caller", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeAny,
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "any-caller-app",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).To(BeNil())
				Expect(reqInfo.AuthResult).NotTo(BeNil())
				Expect(reqInfo.AuthResult.Outcome).To(Equal("allowed"))
				Expect(reqInfo.AuthResult.Rule).To(Equal("domain:scope=any"))
			})
		})

		// ── Scope: org ────────────────────────────────────────────────

		Context("with scope=org", func() {
			It("allows caller from same org", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					Tags:             map[string]string{"organization_id": "org-123"},
					RoutePolicyScope: route.RoutePolicyScopeOrg,
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "caller-app",
					OrgGUID: "org-123",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).To(BeNil())
			})

			It("denies caller from different org with AuthError", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					Tags:             map[string]string{"organization_id": "org-123"},
					RoutePolicyScope: route.RoutePolicyScopeOrg,
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "caller-app",
					OrgGUID: "org-456", // Different org
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				authErr, ok := err.(*postselection.AuthError)
				Expect(ok).To(BeTrue(), "error should be AuthError")
				Expect(authErr.Rule).To(Equal("domain:scope=org:post-selection"))
				Expect(authErr.Reason).To(ContainSubstring("caller org org-456 does not match selected backend org org-123"))
				Expect(authErr.HTTPStatus).To(Equal(http.StatusForbidden))
			})

			It("denies caller when endpoint has no organization_id tag", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					Tags:             map[string]string{}, // No org tag
					RoutePolicyScope: route.RoutePolicyScopeOrg,
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "caller-app",
					OrgGUID: "org-123",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				authErr, ok := err.(*postselection.AuthError)
				Expect(ok).To(BeTrue())
				Expect(authErr.Rule).To(Equal("domain:scope=org:post-selection"))
				Expect(authErr.Reason).To(ContainSubstring("caller org org-123 does not match selected backend org "))
			})

			It("denies caller when caller has no org", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					Tags:             map[string]string{"organization_id": "org-123"},
					RoutePolicyScope: route.RoutePolicyScopeOrg,
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "caller-app",
					OrgGUID: "", // No org
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				authErr, ok := err.(*postselection.AuthError)
				Expect(ok).To(BeTrue())
				Expect(authErr.Rule).To(Equal("domain:scope=org:post-selection"))
			})
		})

		// ── Scope: space ──────────────────────────────────────────────

		Context("with scope=space", func() {
			It("allows caller from same space", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					Tags:             map[string]string{"space_id": "space-abc"},
					RoutePolicyScope: route.RoutePolicyScopeSpace,
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID:   "caller-app",
					SpaceGUID: "space-abc",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).To(BeNil())
			})

			It("denies caller from different space with AuthError", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					Tags:             map[string]string{"space_id": "space-abc"},
					RoutePolicyScope: route.RoutePolicyScopeSpace,
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID:   "caller-app",
					SpaceGUID: "space-xyz", // Different space
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				authErr, ok := err.(*postselection.AuthError)
				Expect(ok).To(BeTrue())
				Expect(authErr.Rule).To(Equal("domain:scope=space:post-selection"))
				Expect(authErr.Reason).To(ContainSubstring("caller space space-xyz does not match selected backend space space-abc"))
				Expect(authErr.HTTPStatus).To(Equal(http.StatusForbidden))
			})

			It("denies caller when endpoint has no space_id tag", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					Tags:             map[string]string{}, // No space tag
					RoutePolicyScope: route.RoutePolicyScopeSpace,
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID:   "caller-app",
					SpaceGUID: "space-abc",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				authErr, ok := err.(*postselection.AuthError)
				Expect(ok).To(BeTrue())
				Expect(authErr.Rule).To(Equal("domain:scope=space:post-selection"))
			})

			It("denies caller when caller has no space", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					Tags:             map[string]string{"space_id": "space-abc"},
					RoutePolicyScope: route.RoutePolicyScopeSpace,
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID:   "caller-app",
					SpaceGUID: "", // No space
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				authErr, ok := err.(*postselection.AuthError)
				Expect(ok).To(BeTrue())
				Expect(authErr.Rule).To(Equal("domain:scope=space:post-selection"))
			})
		})

		// ── Scope: unknown ───────────────────────────────────────────

		Context("with unknown scope", func() {
			It("denies request with AuthError", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: "unknown-scope-value",
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "caller-app",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				authErr, ok := err.(*postselection.AuthError)
				Expect(ok).To(BeTrue())
				Expect(authErr.Rule).To(Equal("domain:scope=unknown:post-selection"))
				Expect(authErr.Reason).To(ContainSubstring("unknown route policy scope"))
				Expect(authErr.HTTPStatus).To(Equal(http.StatusForbidden))
			})
		})

		// ── Shared route scenario: intermittent 403s ─────────────────

		Context("shared route with scope=space (intermittent 403s)", func() {
			It("allows request when selected endpoint matches caller's space", func() {
				// Endpoint from space-abc
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app-1",
					Host:             "192.168.1.1",
					Port:             8080,
					Tags:             map[string]string{"space_id": "space-abc"},
					RoutePolicyScope: route.RoutePolicyScopeSpace,
				})

				// Pool contains endpoints from multiple spaces (shared route)
				pool = route.NewPool(&route.PoolOpts{
					Host: "shared.apps.mtls.internal",
				})
				pool.Put(endpoint)

				// Another endpoint from space-xyz
				endpoint2 := route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app-2",
					Host:             "192.168.1.2",
					Port:             8080,
					Tags:             map[string]string{"space_id": "space-xyz"},
					RoutePolicyScope: route.RoutePolicyScopeSpace,
				})
				pool.Put(endpoint2)

				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID:   "caller-app",
					SpaceGUID: "space-abc",
				}

				// Check against endpoint from space-abc (matches caller)
				err := handler.Check(endpoint, reqInfo)
				Expect(err).To(BeNil())
			})

			It("denies request when selected endpoint is from different space (intermittent 403)", func() {
				// Endpoint from space-xyz (will be selected)
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app-2",
					Host:             "192.168.1.2",
					Port:             8080,
					Tags:             map[string]string{"space_id": "space-xyz"},
					RoutePolicyScope: route.RoutePolicyScopeSpace,
				})

				// Pool contains endpoints from multiple spaces (shared route)
				pool = route.NewPool(&route.PoolOpts{
					Host: "shared.apps.mtls.internal",
				})

				// Endpoint from space-abc
				endpoint1 := route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app-1",
					Host:             "192.168.1.1",
					Port:             8080,
					Tags:             map[string]string{"space_id": "space-abc"},
					RoutePolicyScope: route.RoutePolicyScopeSpace,
				})
				pool.Put(endpoint1)
				pool.Put(endpoint)

				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID:   "caller-app",
					SpaceGUID: "space-abc", // Caller from space-abc
				}

				// Check against endpoint from space-xyz (selected, doesn't match)
				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				authErr, ok := err.(*postselection.AuthError)
				Expect(ok).To(BeTrue())
				Expect(authErr.Rule).To(Equal("domain:scope=space:post-selection"))
				Expect(authErr.Reason).To(ContainSubstring("caller space space-abc does not match selected backend space space-xyz"))
			})
		})
	})

	Describe("NewMtlsScopeAuth", func() {
		Context("when no mTLS domains are configured", func() {
			It("returns NoopPostSelectionHandler", func() {
				logger := test_util.NewTestLogger("test")
				emptyCfg, _ := config.DefaultConfig()
				Expect(emptyCfg.Domains).To(BeEmpty())

				handler := postselection.NewMtlsScopeAuth(emptyCfg, logger.Logger)
				Expect(handler).To(BeIdenticalTo(postselection.NoopPostSelectionHandler))
			})
		})

		Context("when mTLS domains are configured", func() {
			It("returns a real handler", func() {
				logger := test_util.NewTestLogger("test")
				// cfg is already configured with domains in BeforeEach
				handler := postselection.NewMtlsScopeAuth(cfg, logger.Logger)
				Expect(handler).NotTo(BeIdenticalTo(postselection.NoopPostSelectionHandler))
			})
		})
	})
})
