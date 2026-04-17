package handlers_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/gorouter/handlers"
	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/test_util"
)

var _ = Describe("MtlsAccessRulesAuth", func() {
	var (
		handler  *handlers.MtlsAccessRulesAuth
		endpoint *route.Endpoint
		reqInfo  *handlers.RequestInfo
		pool     *route.EndpointPool
	)

	BeforeEach(func() {
		logger := test_util.NewTestLogger("mtls-access-rules-auth")
		handler = handlers.NewMtlsAccessRulesAuth(logger.Logger)
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

		Context("when AccessScope is empty", func() {
			It("returns nil (no enforcement active)", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:       "backend-app",
					Host:        "192.168.1.1",
					Port:        8080,
					AccessScope: "", // No enforcement
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
					AppId:       "backend-app",
					Host:        "192.168.1.1",
					Port:        8080,
					AccessScope: route.AccessScopeOrg,
					AccessRules: []string{"cf:any"},
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = nil

				err := handler.Check(endpoint, reqInfo)
				Expect(err).To(BeNil())
			})
		})

		Context("when no access rules are configured", func() {
			It("denies with AuthError (default deny)", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:       "backend-app",
					Host:        "192.168.1.1",
					Port:        8080,
					AccessScope: route.AccessScopeOrg,
					AccessRules: []string{}, // No rules = default deny
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "caller-app",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				mtlsErr, ok := err.(*handlers.AuthError)
				Expect(ok).To(BeTrue())
				Expect(mtlsErr.Rule).To(Equal("route:no_access_rules"))
				Expect(mtlsErr.Reason).To(Equal("route has no access rules configured"))
				Expect(mtlsErr.HTTPStatus).To(Equal(http.StatusForbidden))
			})
		})

		// ── Access rule: cf:any ───────────────────────────────────────

		Context("with access rule cf:any", func() {
			It("allows any authenticated caller", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:       "backend-app",
					Host:        "192.168.1.1",
					Port:        8080,
					AccessScope: route.AccessScopeAny,
					AccessRules: []string{"cf:any"},
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

		// ── Access rule: cf:app:<guid> ────────────────────────────────

		Context("with access rule cf:app:<guid>", func() {
			It("allows caller with matching app GUID", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:       "backend-app",
					Host:        "192.168.1.1",
					Port:        8080,
					AccessScope: route.AccessScopeAny,
					AccessRules: []string{"cf:app:allowed-app-123"},
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
					AppId:       "backend-app",
					Host:        "192.168.1.1",
					Port:        8080,
					AccessScope: route.AccessScopeAny,
					AccessRules: []string{"cf:app:allowed-app-123"},
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "other-app-456",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				mtlsErr, ok := err.(*handlers.AuthError)
				Expect(ok).To(BeTrue())
				Expect(mtlsErr.Rule).To(Equal("route:access_rules"))
				Expect(mtlsErr.Reason).To(ContainSubstring("caller app other-app-456 not in access_rules"))
			})
		})

		// ── Access rule: cf:space:<guid> ──────────────────────────────

		Context("with access rule cf:space:<guid>", func() {
			It("allows caller from matching space", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:       "backend-app",
					Host:        "192.168.1.1",
					Port:        8080,
					AccessScope: route.AccessScopeAny,
					AccessRules: []string{"cf:space:allowed-space-abc"},
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
					AppId:       "backend-app",
					Host:        "192.168.1.1",
					Port:        8080,
					AccessScope: route.AccessScopeAny,
					AccessRules: []string{"cf:space:allowed-space-abc"},
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID:   "caller-app",
					SpaceGUID: "other-space-xyz",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				mtlsErr, ok := err.(*handlers.AuthError)
				Expect(ok).To(BeTrue())
				Expect(mtlsErr.Rule).To(Equal("route:access_rules"))
			})
		})

		// ── Access rule: cf:org:<guid> ────────────────────────────────

		Context("with access rule cf:org:<guid>", func() {
			It("allows caller from matching org", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:       "backend-app",
					Host:        "192.168.1.1",
					Port:        8080,
					AccessScope: route.AccessScopeAny,
					AccessRules: []string{"cf:org:allowed-org-123"},
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
					AppId:       "backend-app",
					Host:        "192.168.1.1",
					Port:        8080,
					AccessScope: route.AccessScopeAny,
					AccessRules: []string{"cf:org:allowed-org-123"},
				})
				pool = createPool(endpoint)
				reqInfo.RoutePool = pool
				reqInfo.CallerIdentity = &handlers.CallerIdentity{
					AppGUID: "caller-app",
					OrgGUID: "other-org-456",
				}

				err := handler.Check(endpoint, reqInfo)
				Expect(err).NotTo(BeNil())

				mtlsErr, ok := err.(*handlers.AuthError)
				Expect(ok).To(BeTrue())
				Expect(mtlsErr.Rule).To(Equal("route:access_rules"))
			})
		})

		// ── Multiple access rules ─────────────────────────────────────

		Context("with multiple access rules", func() {
			It("allows caller matching first rule", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:       "backend-app",
					Host:        "192.168.1.1",
					Port:        8080,
					AccessScope: route.AccessScopeAny,
					AccessRules: []string{
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
					AppId:       "backend-app",
					Host:        "192.168.1.1",
					Port:        8080,
					AccessScope: route.AccessScopeAny,
					AccessRules: []string{
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
					AppId:       "backend-app",
					Host:        "192.168.1.1",
					Port:        8080,
					AccessScope: route.AccessScopeAny,
					AccessRules: []string{
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
					AppId:       "backend-app",
					Host:        "192.168.1.1",
					Port:        8080,
					AccessScope: route.AccessScopeAny,
					AccessRules: []string{
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

				mtlsErr, ok := err.(*handlers.AuthError)
				Expect(ok).To(BeTrue())
				Expect(mtlsErr.Rule).To(Equal("route:access_rules"))
			})
		})

		// ── Edge cases ────────────────────────────────────────────────

		Context("edge cases", func() {
			It("handles whitespace in access rules", func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:       "backend-app",
					Host:        "192.168.1.1",
					Port:        8080,
					AccessScope: route.AccessScopeAny,
					AccessRules: []string{"  cf:any  "}, // Whitespace
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
					AppId:       "backend-app",
					Host:        "192.168.1.1",
					Port:        8080,
					AccessScope: route.AccessScopeAny,
					AccessRules: []string{
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
