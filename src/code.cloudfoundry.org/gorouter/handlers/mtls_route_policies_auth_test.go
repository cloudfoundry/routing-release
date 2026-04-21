package handlers_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/gorouter/handlers"
	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/test_util"
)

var _ = Describe("MtlsRoutePoliciesAuth", func() {
	var (
		handler  *handlers.MtlsRoutePoliciesAuth
		endpoint *route.Endpoint
		reqInfo  *handlers.RequestInfo
		pool     *route.EndpointPool
	)

	BeforeEach(func() {
		logger := test_util.NewTestLogger("mtls-route-policies-auth")
		handler = handlers.NewMtlsRoutePoliciesAuth(logger.Logger)
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
			It("returns nil (no enforcement)", func() {
				reqInfo.RoutePool = nil
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId: "backend-app",
					Host:  "192.168.1.1",
					Port:  8080,
				})

				err := handler.Check(endpoint, reqInfo)
				Expect(err).To(BeNil())
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
			It("returns nil (identity check should have failed earlier)", func() {
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
				Expect(err).To(BeNil())
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

				authErr, ok := err.(*handlers.AuthError)
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

				authErr, ok := err.(*handlers.AuthError)
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

				authErr, ok := err.(*handlers.AuthError)
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

				authErr, ok := err.(*handlers.AuthError)
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

				authErr, ok := err.(*handlers.AuthError)
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
			})
		})
	})
})
