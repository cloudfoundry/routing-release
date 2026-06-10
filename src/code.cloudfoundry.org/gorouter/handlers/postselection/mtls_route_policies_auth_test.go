package postselection_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/handlers"
	"code.cloudfoundry.org/gorouter/handlers/postselection"
	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/test_util"
)

var _ = Describe("MtlsRoutePoliciesAuth", func() {
	var (
		handler  postselection.PostSelectionHandler
		endpoint *route.Endpoint
		reqInfo  *handlers.RequestInfo
		pool     *route.EndpointPool
		cfg      *config.Config
		logger   *test_util.TestLogger
	)

	BeforeEach(func() {
		logger = test_util.NewTestLogger("mtls-route-policies-auth")
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
		handler = postselection.NewMtlsRoutePoliciesAuth(cfg, logger.Logger)
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

			It("skips enforcement when caller has identity", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: "", // No enforcement configured
					RoutePolicies:    []string{"cf:any"},
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID:   "caller-app",
					SpaceGUID: "caller-space",
					OrgGUID:   "caller-org",
				}

				// Even though caller has identity and route has policies,
				// enforcement is skipped because RoutePolicyScope is empty
				// (domain not configured for route policy enforcement)
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
					RoutePolicies:    []string{"cf:any"},
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = nil

				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				authErr, ok := err.(*postselection.AuthError)
				Expect(ok).To(BeTrue())
				Expect(authErr.Rule).To(Equal("route:no_caller_identity"))
				Expect(authErr.Reason).To(Equal("no caller identity present"))
				Expect(authErr.HTTPStatus).To(Equal(http.StatusForbidden))
			})
		})

		Context("when no route policies are configured", func() {
			It("denies with AuthError (default deny)", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeOrg,
					RoutePolicies:    []string{}, // No sources = default deny
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
				Expect(authErr.Rule).To(Equal("route:no_route_policies"))
				Expect(authErr.Reason).To(Equal("route has no route policies configured"))
				Expect(authErr.HTTPStatus).To(Equal(http.StatusForbidden))
			})

			It("denies with AuthError when RoutePolicies is nil", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeOrg,
					RoutePolicies:    nil, // Nil = default deny
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
				Expect(authErr.Rule).To(Equal("route:no_route_policies"))
				Expect(authErr.Reason).To(Equal("route has no route policies configured"))
				Expect(authErr.HTTPStatus).To(Equal(http.StatusForbidden))
			})
		})

		// ── Route policy: cf:any ───────────────────────────────────────

		Context("with route policy cf:any", func() {
			It("allows any authenticated caller", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeAny,
					RoutePolicies:    []string{"cf:any"},
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "random-caller-app",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).To(BeNil())
				Expect(reqInfo.AuthResult.Rule).To(Equal("route:cf:any"))
			})
		})

		// ── Route policy: cf:app:<guid> ────────────────────────────────

		Context("with route policy cf:app:<guid>", func() {
			It("allows caller with matching app GUID", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeAny,
					RoutePolicies:    []string{"cf:app:allowed-app-123"},
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "allowed-app-123",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).To(BeNil())
				Expect(reqInfo.AuthResult.Rule).To(Equal("route:cf:app:allowed-app-123"))
			})

			It("denies caller with different app GUID", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeAny,
					RoutePolicies:    []string{"cf:app:allowed-app-123"},
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "other-app-456",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				authErr, ok := err.(*postselection.AuthError)
				Expect(ok).To(BeTrue())
				Expect(authErr.Rule).To(Equal("route:route_policies"))
				Expect(authErr.Reason).To(ContainSubstring("caller app other-app-456 not in route_policies"))
			})
		})

		// ── Route policy: cf:space:<guid> ──────────────────────────────

		Context("with route policy cf:space:<guid>", func() {
			It("allows caller from matching space", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeAny,
					RoutePolicies:    []string{"cf:space:allowed-space-abc"},
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID:   "caller-app",
					SpaceGUID: "allowed-space-abc",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).To(BeNil())
				Expect(reqInfo.AuthResult.Rule).To(Equal("route:cf:space:allowed-space-abc"))
			})

			It("denies caller from different space", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeAny,
					RoutePolicies:    []string{"cf:space:allowed-space-abc"},
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID:   "caller-app",
					SpaceGUID: "other-space-xyz",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				authErr, ok := err.(*postselection.AuthError)
				Expect(ok).To(BeTrue())
				Expect(authErr.Rule).To(Equal("route:route_policies"))
			})
		})

		// ── Route policy: cf:org:<guid> ────────────────────────────────

		Context("with route policy cf:org:<guid>", func() {
			It("allows caller from matching org", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeAny,
					RoutePolicies:    []string{"cf:org:allowed-org-123"},
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "caller-app",
					OrgGUID: "allowed-org-123",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).To(BeNil())
				Expect(reqInfo.AuthResult.Rule).To(Equal("route:cf:org:allowed-org-123"))
			})

			It("denies caller from different org", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeAny,
					RoutePolicies:    []string{"cf:org:allowed-org-123"},
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "caller-app",
					OrgGUID: "other-org-456",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				authErr, ok := err.(*postselection.AuthError)
				Expect(ok).To(BeTrue())
				Expect(authErr.Rule).To(Equal("route:route_policies"))
			})
		})

		// ── Multiple route policies ─────────────────────────────────────

		Context("with multiple route policies", func() {
			It("allows caller matching first rule", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeAny,
					RoutePolicies: []string{
						"cf:app:app-1",
						"cf:app:app-2",
						"cf:space:space-abc",
					},
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "app-1",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).To(BeNil())
				Expect(reqInfo.AuthResult.Rule).To(Equal("route:cf:app:app-1"))
			})

			It("allows caller matching second rule", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeAny,
					RoutePolicies: []string{
						"cf:app:app-1",
						"cf:app:app-2",
						"cf:space:space-abc",
					},
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "app-2",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).To(BeNil())
				Expect(reqInfo.AuthResult.Rule).To(Equal("route:cf:app:app-2"))
			})

			It("allows caller matching third rule", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeAny,
					RoutePolicies: []string{
						"cf:app:app-1",
						"cf:app:app-2",
						"cf:space:space-abc",
					},
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID:   "some-other-app",
					SpaceGUID: "space-abc",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).To(BeNil())
				Expect(reqInfo.AuthResult.Rule).To(Equal("route:cf:space:space-abc"))
			})

			It("denies caller matching no rules", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeAny,
					RoutePolicies: []string{
						"cf:app:app-1",
						"cf:app:app-2",
						"cf:space:space-abc",
					},
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID:   "unrelated-app",
					SpaceGUID: "unrelated-space",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				authErr, ok := err.(*postselection.AuthError)
				Expect(ok).To(BeTrue())
				Expect(authErr.Rule).To(Equal("route:route_policies"))
			})
		})

		// ── Pool-level policies (route shared across backends) ──
		//
		// Route policies are stored on the pool, which holds the most
		// up-to-date view for the route. Authorization must use the
		// pool-level policies regardless of which endpoint was selected,
		// because per-endpoint copies can be stale when a route is shared
		// across backends with differing policies.

		Context("when endpoints on the same route have different route policies", func() {
			var endpoint1, endpoint2 *route.Endpoint

			BeforeEach(func() {
				// First endpoint allows only app-1
				endpoint1 = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app-1",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeAny,
					RoutePolicies:    []string{"cf:app:allowed-app-1"},
				})

				// Second endpoint allows only app-2
				endpoint2 = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app-2",
					Host:             "192.168.1.2",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeAny,
					RoutePolicies:    []string{"cf:app:allowed-app-2"},
				})

				pool = route.NewPool(&route.PoolOpts{
					Host: "shared.apps.mtls.internal",
				})
				pool.Put(endpoint1)
				pool.Put(endpoint2)
				reqInfo.RoutePool = pool

				// Pool-level policies come from the last-registered endpoint.
				Expect(pool.RoutePolicies()).To(Equal([]string{"cf:app:allowed-app-2"}))
			})

			It("authorizes using pool-level policies regardless of the selected endpoint", func() {
				// Caller matches the pool-level policy (app-2).
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "allowed-app-2",
				}

				// Even when routed to endpoint1 (whose stale per-endpoint
				// policy only allows app-1), authorization uses the
				// pool-level policy and allows the caller.
				err := handler.Check(endpoint1, reqInfo)
				Expect(err).To(BeNil(), "should allow when pool-level policy matches caller")
				Expect(reqInfo.AuthResult.Rule).To(Equal("route:cf:app:allowed-app-2"))

				// Routing to endpoint2 uses the same pool-level policy.
				reqInfo.AuthResult = nil
				err = handler.Check(endpoint2, reqInfo)
				Expect(err).To(BeNil(), "should allow regardless of the selected endpoint")
				Expect(reqInfo.AuthResult.Rule).To(Equal("route:cf:app:allowed-app-2"))
			})

			It("does not authorize using stale per-endpoint policies", func() {
				// Caller matches only endpoint1's stale per-endpoint policy
				// (app-1), which is no longer the pool-level policy.
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "allowed-app-1",
				}

				// Denied: the pool-level policy only allows app-2, even
				// though the selected endpoint's own policy would allow app-1.
				err := handler.Check(endpoint1, reqInfo)
				Expect(err).NotTo(BeNil(), "should deny when only the stale per-endpoint policy matches")

				authErr, ok := err.(*postselection.AuthError)
				Expect(ok).To(BeTrue())
				Expect(authErr.Rule).To(Equal("route:route_policies"))
			})
		})

		// ── Edge cases ────────────────────────────────────────────────

		Context("edge cases", func() {
			It("handles whitespace in route policies", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeAny,
					RoutePolicies:    []string{"  cf:any  "}, // Whitespace
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "caller-app",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).To(BeNil())
				Expect(reqInfo.AuthResult.Rule).To(Equal("route:cf:any"))
			})

			It("skips malformed rules and evaluates valid ones", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:            "backend-app",
					Host:             "192.168.1.1",
					Port:             8080,
					RoutePolicyScope: route.RoutePolicyScopeAny,
					RoutePolicies: []string{
						"invalid-rule",
						"cf:app:allowed-app",
					},
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "allowed-app",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).To(BeNil())
				Expect(reqInfo.AuthResult.Rule).To(Equal("route:cf:app:allowed-app"))

				// Malformed rules are skipped, but must be logged at warn
				// level so operators can detect misconfigured route policies
				// instead of having them silently ignored.
				Eventually(logger).Should(gbytes.Say("malformed-route-policy"))
				Eventually(logger).Should(gbytes.Say("invalid-rule"))
			})
		})
	})

	Describe("NewMtlsRoutePoliciesAuth", func() {
		Context("when no mTLS domains are configured", func() {
			It("returns NoopPostSelectionHandler", func() {
				logger := test_util.NewTestLogger("test")
				emptyCfg, _ := config.DefaultConfig()
				Expect(emptyCfg.Domains).To(BeEmpty())

				handler := postselection.NewMtlsRoutePoliciesAuth(emptyCfg, logger.Logger)
				Expect(handler).To(BeIdenticalTo(postselection.NoopPostSelectionHandler))
			})
		})

		Context("when mTLS domains are configured", func() {
			It("returns a real handler", func() {
				logger := test_util.NewTestLogger("test")
				cfg, _ := config.DefaultConfig()
				certChain := test_util.CreateSignedCertWithRootCA(test_util.CertNames{SANs: test_util.SubjectAltNames{DNS: "test.com"}})
				cfg.Domains = []config.MtlsDomainConfig{
					{
						Domain:  "*.apps.mtls.internal",
						CACerts: string(certChain.CACertPEM),
					},
				}
				cfg.Process()

				handler := postselection.NewMtlsRoutePoliciesAuth(cfg, logger.Logger)
				Expect(handler).NotTo(BeIdenticalTo(postselection.NoopPostSelectionHandler))
			})
		})
	})
})
