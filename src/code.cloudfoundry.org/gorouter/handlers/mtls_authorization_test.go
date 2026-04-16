package handlers_test

import (
	"context"
	"log/slog"
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

// NOTE: These tests are for the DEPRECATED MtlsAuthorization handler.
// The handler is now split into:
//   - MtlsPreAuth (pre-selection checks) - tested here
//   - MtlsScopeAuth + MtlsAccessRulesAuth (post-selection checks) - see mtls_scope_auth_test.go and mtls_access_rules_auth_test.go
// These tests remain to ensure the pre-selection behavior still works correctly.
var _ = Describe("MtlsAuthorization", func() {
	var (
		handler     negroni.Handler
		cfg         *config.Config
		logger      *test_util.TestLogger
		nextCalled  bool
		nextHandler http.HandlerFunc
		recorder    *httptest.ResponseRecorder
		request     *http.Request
	)

	// createPool builds a pool with a single endpoint carrying the given opts.
	createPoolWithEndpoint := func(endpoint *route.Endpoint) *route.EndpointPool {
		pool := route.NewPool(&route.PoolOpts{
			Host:                   "backend.apps.identity",
			Logger:                 slog.Default(),
			LoadBalancingAlgorithm: config.LOAD_BALANCE_RR,
		})
		pool.Put(endpoint)
		return pool
	}

	// injectTLSConnState returns a middleware that injects a TLSConnState into
	// the request context, simulating what router.go does for real TLS connections.
	injectTLSConnState := func(state *handlers.TLSConnState) negroni.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
			ctx := handlers.SetTLSConnState(r.Context(), state)
			next(w, r.WithContext(ctx))
		}
	}

	// buildChain constructs a negroni chain: RequestInfo → tlsState → extra → handler → next.
	buildChain := func(tlsState *handlers.TLSConnState, extra negroni.HandlerFunc) *negroni.Negroni {
		n := negroni.New()
		n.Use(handlers.NewRequestInfo())
		if tlsState != nil {
			n.Use(injectTLSConnState(tlsState))
		}
		if extra != nil {
			n.UseFunc(extra)
		}
		n.Use(handler)
		n.UseHandlerFunc(nextHandler)
		return n
	}

	// validTLSState returns a TLSConnState that passes the SNI/Host check for
	// the given host (matching mTLS domain backend.apps.identity).
	validTLSState := func(host string) *handlers.TLSConnState {
		return &handlers.TLSConnState{
			SNI:                host,
			MtlsDomain:         host,
			ClientCertRequired: true,
		}
	}

	BeforeEach(func() {
		logger = test_util.NewTestLogger("mtls-authorization")
		cfg, _ = config.DefaultConfig()

		_, caCertPEM := test_util.CreateKeyPair("test-ca")

		// Configure a single mTLS domain using the RFC field name "Domains".
		cfg.Domains = []config.MtlsDomainConfig{
			{
				Domain:              "*.apps.identity",
				CACerts:             string(caCertPEM),
				ForwardedClientCert: config.SANITIZE_SET,
			},
		}
		err := cfg.Process()
		Expect(err).NotTo(HaveOccurred())

		handler = handlers.NewMtlsAuthorization(cfg, logger.Logger)
		nextCalled = false
		recorder = httptest.NewRecorder()

		nextHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
		})
	})

	Context("when RequestInfo is not in context", func() {
		BeforeEach(func() {
			request = test_util.NewRequest("GET", "backend.apps.identity", "/", nil)
		})

		It("returns 500 Internal Server Error", func() {
			handler.ServeHTTP(recorder, request, nextHandler)

			Expect(nextCalled).To(BeFalse())
			Expect(recorder.Code).To(Equal(http.StatusInternalServerError))
		})
	})

	Context("when request is NOT on an mTLS domain", func() {
		BeforeEach(func() {
			request = test_util.NewRequest("GET", "regular.example.com", "/", nil)
		})

		It("calls next handler without any checks", func() {
			buildChain(nil, nil).ServeHTTP(recorder, request)

			Expect(nextCalled).To(BeTrue())
			Expect(recorder.Code).To(Equal(http.StatusOK))
		})
	})

	Context("when request IS on an mTLS domain", func() {
		BeforeEach(func() {
			request = test_util.NewRequest("GET", "backend.apps.identity", "/", nil)
		})

		// ── SNI / Host checks ────────────────────────────────────────────────────

		Context("SNI/Host mismatch checks (421)", func() {
			It("returns 421 when no TLS connection state is present (plain HTTP connection)", func() {
				// No TLS state injected — zero-value connState means ClientCertRequired=false.
				buildChain(nil, nil).ServeHTTP(recorder, request)

				Expect(nextCalled).To(BeFalse())
				Expect(recorder.Code).To(Equal(http.StatusMisdirectedRequest))
			})

			It("returns 421 when TLS was done but ClientCertRequired is false", func() {
				state := &handlers.TLSConnState{
					SNI:                "backend.apps.identity",
					MtlsDomain:         "",
					ClientCertRequired: false,
				}
				buildChain(state, nil).ServeHTTP(recorder, request)

				Expect(nextCalled).To(BeFalse())
				Expect(recorder.Code).To(Equal(http.StatusMisdirectedRequest))
			})

			It("returns 421 when SNI domain differs from Host (mTLS bypass attempt)", func() {
				// Client connected with SNI for a different domain.
				state := &handlers.TLSConnState{
					SNI:                "other.apps.identity",
					MtlsDomain:         "other.apps.identity",
					ClientCertRequired: true,
				}
				buildChain(state, nil).ServeHTTP(recorder, request)

				Expect(nextCalled).To(BeFalse())
				Expect(recorder.Code).To(Equal(http.StatusMisdirectedRequest))
			})

			It("returns 421 when client connected to regular domain but Host is mTLS domain", func() {
				state := &handlers.TLSConnState{
					SNI:                "regular.example.com",
					MtlsDomain:         "",
					ClientCertRequired: false,
				}
				buildChain(state, nil).ServeHTTP(recorder, request)

				Expect(nextCalled).To(BeFalse())
				Expect(recorder.Code).To(Equal(http.StatusMisdirectedRequest))
			})

			It("sets TlsSNI on reqInfo for access logging", func() {
				state := &handlers.TLSConnState{
					SNI:                "regular.example.com",
					MtlsDomain:         "",
					ClientCertRequired: false,
				}
				var capturedReqInfo *handlers.RequestInfo
				extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
					ri, _ := handlers.ContextRequestInfo(r)
					capturedReqInfo = ri
					next(w, r)
				})
				buildChain(state, extra).ServeHTTP(recorder, request)

				Expect(recorder.Code).To(Equal(http.StatusMisdirectedRequest))
				Expect(capturedReqInfo.TlsSNI).To(Equal("regular.example.com"))
			})
		})

		// ── Route pool checks ────────────────────────────────────────────────────

		Context("when no route pool is set", func() {
			It("returns 404 Not Found", func() {
				buildChain(validTLSState("backend.apps.identity"), nil).ServeHTTP(recorder, request)

				Expect(nextCalled).To(BeFalse())
				Expect(recorder.Code).To(Equal(http.StatusNotFound))
			})
		})

		// ── No enforcement (AccessScope empty) ───────────────────────────────────

		Context("when pool has no AccessScope (enforcement not active)", func() {
			It("forwards the request without authorization checks", func() {
				endpoint := route.NewEndpoint(&route.EndpointOpts{
					AppId:             "backend-app-id",
					Host:              "192.168.1.1",
					Port:              8080,
					PrivateInstanceId: "instance-id",
					// AccessScope is empty — no enforcement.
				})
				pool := createPoolWithEndpoint(endpoint)

				extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
					ri, _ := handlers.ContextRequestInfo(r)
					ri.RoutePool = pool
					next(w, r)
				})
				buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

				Expect(nextCalled).To(BeTrue())
				Expect(recorder.Code).To(Equal(http.StatusOK))
			})
		})

		// ── Enforcement active ───────────────────────────────────────────────────

		Context("when enforcement is active (AccessScope is set)", func() {
			Context("when caller identity is missing (cert has no CF identity OUs)", func() {
				It("returns 403 Forbidden and sets RTR log fields", func() {
					endpoint := route.NewEndpoint(&route.EndpointOpts{
						AppId:       "backend-app-id",
						Host:        "192.168.1.1",
						Port:        8080,
						AccessScope: route.AccessScopeAny,
						AccessRules: []string{"cf:any"},
					})
					pool := createPoolWithEndpoint(endpoint)

					var capturedReqInfo *handlers.RequestInfo
					extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						ri, _ := handlers.ContextRequestInfo(r)
						ri.RoutePool = pool
						// CallerIdentity intentionally not set.
						capturedReqInfo = ri
						next(w, r)
					})
					buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

					Expect(nextCalled).To(BeFalse())
					Expect(recorder.Code).To(Equal(http.StatusForbidden))
					Expect(capturedReqInfo.MtlsAuth).To(Equal("denied"))
					Expect(capturedReqInfo.MtlsRule).To(Equal("identity_extraction"))
				})
			})

			// ── Default deny ────────────────────────────────────────────────────

			Context("when enforcement is active but NO access rules are configured", func() {
				It("returns 403 Forbidden (default deny)", func() {
					endpoint := route.NewEndpoint(&route.EndpointOpts{
						AppId:       "backend-app-id",
						Host:        "192.168.1.1",
						Port:        8080,
						AccessScope: route.AccessScopeAny,
						// AccessRules is empty — default deny.
					})
					pool := createPoolWithEndpoint(endpoint)

					extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						ri, _ := handlers.ContextRequestInfo(r)
						ri.RoutePool = pool
						ri.CallerIdentity = &handlers.CallerIdentity{
							AppGUID:   "some-app",
							SpaceGUID: "some-space",
							OrgGUID:   "some-org",
						}
						next(w, r)
					})
					buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

					Expect(nextCalled).To(BeFalse())
					Expect(recorder.Code).To(Equal(http.StatusForbidden))
				})

				It("sets MtlsRule to route:no_access_rules and sets RouteEndpoint for logging", func() {
					endpoint := route.NewEndpoint(&route.EndpointOpts{
						AppId:       "backend-app-id",
						Host:        "192.168.1.1",
						Port:        8080,
						AccessScope: route.AccessScopeAny,
					})
					pool := createPoolWithEndpoint(endpoint)

					var capturedReqInfo *handlers.RequestInfo
					extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						ri, _ := handlers.ContextRequestInfo(r)
						ri.RoutePool = pool
						ri.CallerIdentity = &handlers.CallerIdentity{AppGUID: "caller-app"}
						capturedReqInfo = ri
						next(w, r)
					})
					buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

					Expect(recorder.Code).To(Equal(http.StatusForbidden))
					Expect(capturedReqInfo.MtlsAuth).To(Equal("denied"))
					Expect(capturedReqInfo.MtlsRule).To(Equal("route:no_access_rules"))
					Expect(capturedReqInfo.RouteEndpoint).NotTo(BeNil())
					Expect(capturedReqInfo.RouteEndpoint.ApplicationId).To(Equal("backend-app-id"))
				})
			})

			// ── Scope boundary: any ──────────────────────────────────────────────

			Context("with scope=any", func() {
				It("allows any authenticated caller that has a matching access rule", func() {
					endpoint := route.NewEndpoint(&route.EndpointOpts{
						AppId:       "backend-app-id",
						Host:        "192.168.1.1",
						Port:        8080,
						AccessScope: route.AccessScopeAny,
						AccessRules: []string{"cf:any"},
					})
					pool := createPoolWithEndpoint(endpoint)

					extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						ri, _ := handlers.ContextRequestInfo(r)
						ri.RoutePool = pool
						ri.CallerIdentity = &handlers.CallerIdentity{AppGUID: "random-app"}
						next(w, r)
					})
					buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

					Expect(nextCalled).To(BeTrue())
					Expect(recorder.Code).To(Equal(http.StatusOK))
				})
			})

			// ── Scope boundary: org ──────────────────────────────────────────────

			Context("with scope=org", func() {
				var pool *route.EndpointPool

				BeforeEach(func() {
					endpoint := route.NewEndpoint(&route.EndpointOpts{
						AppId:       "backend-app-id",
						Host:        "192.168.1.1",
						Port:        8080,
						Tags:        map[string]string{"organization_id": "allowed-org"},
						AccessScope: route.AccessScopeOrg,
						AccessRules: []string{"cf:any"},
					})
					pool = createPoolWithEndpoint(endpoint)
				})

				It("allows a caller from the same org", func() {
					extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						ri, _ := handlers.ContextRequestInfo(r)
						ri.RoutePool = pool
						ri.CallerIdentity = &handlers.CallerIdentity{
							AppGUID: "caller-app",
							OrgGUID: "allowed-org",
						}
						next(w, r)
					})
					buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

					Expect(nextCalled).To(BeTrue())
					Expect(recorder.Code).To(Equal(http.StatusOK))
				})

				It("denies a caller from a different org", func() {
					extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						ri, _ := handlers.ContextRequestInfo(r)
						ri.RoutePool = pool
						ri.CallerIdentity = &handlers.CallerIdentity{
							AppGUID: "caller-app",
							OrgGUID: "other-org",
						}
						next(w, r)
					})
					buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

					Expect(nextCalled).To(BeFalse())
					Expect(recorder.Code).To(Equal(http.StatusForbidden))
				})

				It("sets MtlsRule to domain:scope=org on denial", func() {
					var capturedReqInfo *handlers.RequestInfo
					extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						ri, _ := handlers.ContextRequestInfo(r)
						ri.RoutePool = pool
						ri.CallerIdentity = &handlers.CallerIdentity{
							AppGUID: "caller-app",
							OrgGUID: "other-org",
						}
						capturedReqInfo = ri
						next(w, r)
					})
					buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

					Expect(capturedReqInfo.MtlsAuth).To(Equal("denied"))
					Expect(capturedReqInfo.MtlsRule).To(Equal("domain:scope=org"))
				})
			})

			// ── Scope boundary: space ────────────────────────────────────────────

			Context("with scope=space", func() {
				var pool *route.EndpointPool

				BeforeEach(func() {
					endpoint := route.NewEndpoint(&route.EndpointOpts{
						AppId:       "backend-app-id",
						Host:        "192.168.1.1",
						Port:        8080,
						Tags:        map[string]string{"space_id": "allowed-space"},
						AccessScope: route.AccessScopeSpace,
						AccessRules: []string{"cf:any"},
					})
					pool = createPoolWithEndpoint(endpoint)
				})

				It("allows a caller from the same space", func() {
					extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						ri, _ := handlers.ContextRequestInfo(r)
						ri.RoutePool = pool
						ri.CallerIdentity = &handlers.CallerIdentity{
							AppGUID:   "caller-app",
							SpaceGUID: "allowed-space",
						}
						next(w, r)
					})
					buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

					Expect(nextCalled).To(BeTrue())
					Expect(recorder.Code).To(Equal(http.StatusOK))
				})

				It("denies a caller from a different space", func() {
					extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						ri, _ := handlers.ContextRequestInfo(r)
						ri.RoutePool = pool
						ri.CallerIdentity = &handlers.CallerIdentity{
							AppGUID:   "caller-app",
							SpaceGUID: "other-space",
						}
						next(w, r)
					})
					buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

					Expect(nextCalled).To(BeFalse())
					Expect(recorder.Code).To(Equal(http.StatusForbidden))
				})

				It("sets MtlsRule to domain:scope=space on denial", func() {
					var capturedReqInfo *handlers.RequestInfo
					extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						ri, _ := handlers.ContextRequestInfo(r)
						ri.RoutePool = pool
						ri.CallerIdentity = &handlers.CallerIdentity{
							AppGUID:   "caller-app",
							SpaceGUID: "other-space",
						}
						capturedReqInfo = ri
						next(w, r)
					})
					buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

					Expect(capturedReqInfo.MtlsAuth).To(Equal("denied"))
					Expect(capturedReqInfo.MtlsRule).To(Equal("domain:scope=space"))
				})
			})

			// ── Access rules ─────────────────────────────────────────────────────

			Context("access rules", func() {
				Context("cf:app:<guid>", func() {
					It("allows a matching app", func() {
						endpoint := route.NewEndpoint(&route.EndpointOpts{
							AppId:       "backend-app-id",
							Host:        "192.168.1.1",
							Port:        8080,
							AccessScope: route.AccessScopeAny,
							AccessRules: []string{"cf:app:allowed-app-1", "cf:app:allowed-app-2"},
						})
						pool := createPoolWithEndpoint(endpoint)

						extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
							ri, _ := handlers.ContextRequestInfo(r)
							ri.RoutePool = pool
							ri.CallerIdentity = &handlers.CallerIdentity{AppGUID: "allowed-app-2"}
							next(w, r)
						})
						buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

						Expect(nextCalled).To(BeTrue())
						Expect(recorder.Code).To(Equal(http.StatusOK))
					})

					It("denies a non-matching app", func() {
						endpoint := route.NewEndpoint(&route.EndpointOpts{
							AppId:       "backend-app-id",
							Host:        "192.168.1.1",
							Port:        8080,
							AccessScope: route.AccessScopeAny,
							AccessRules: []string{"cf:app:allowed-app-1"},
						})
						pool := createPoolWithEndpoint(endpoint)

						extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
							ri, _ := handlers.ContextRequestInfo(r)
							ri.RoutePool = pool
							ri.CallerIdentity = &handlers.CallerIdentity{AppGUID: "other-app"}
							next(w, r)
						})
						buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

						Expect(nextCalled).To(BeFalse())
						Expect(recorder.Code).To(Equal(http.StatusForbidden))
					})
				})

				Context("cf:space:<guid>", func() {
					It("allows a caller from the matching space", func() {
						endpoint := route.NewEndpoint(&route.EndpointOpts{
							AppId:       "backend-app-id",
							Host:        "192.168.1.1",
							Port:        8080,
							AccessScope: route.AccessScopeAny,
							AccessRules: []string{"cf:space:allowed-space"},
						})
						pool := createPoolWithEndpoint(endpoint)

						extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
							ri, _ := handlers.ContextRequestInfo(r)
							ri.RoutePool = pool
							ri.CallerIdentity = &handlers.CallerIdentity{
								AppGUID:   "some-app",
								SpaceGUID: "allowed-space",
							}
							next(w, r)
						})
						buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

						Expect(nextCalled).To(BeTrue())
					})

					It("denies a caller from a different space", func() {
						endpoint := route.NewEndpoint(&route.EndpointOpts{
							AppId:       "backend-app-id",
							Host:        "192.168.1.1",
							Port:        8080,
							AccessScope: route.AccessScopeAny,
							AccessRules: []string{"cf:space:allowed-space"},
						})
						pool := createPoolWithEndpoint(endpoint)

						extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
							ri, _ := handlers.ContextRequestInfo(r)
							ri.RoutePool = pool
							ri.CallerIdentity = &handlers.CallerIdentity{
								AppGUID:   "some-app",
								SpaceGUID: "other-space",
							}
							next(w, r)
						})
						buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

						Expect(nextCalled).To(BeFalse())
						Expect(recorder.Code).To(Equal(http.StatusForbidden))
					})
				})

				Context("cf:org:<guid>", func() {
					It("allows a caller from the matching org", func() {
						endpoint := route.NewEndpoint(&route.EndpointOpts{
							AppId:       "backend-app-id",
							Host:        "192.168.1.1",
							Port:        8080,
							AccessScope: route.AccessScopeAny,
							AccessRules: []string{"cf:org:allowed-org"},
						})
						pool := createPoolWithEndpoint(endpoint)

						extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
							ri, _ := handlers.ContextRequestInfo(r)
							ri.RoutePool = pool
							ri.CallerIdentity = &handlers.CallerIdentity{
								AppGUID: "some-app",
								OrgGUID: "allowed-org",
							}
							next(w, r)
						})
						buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

						Expect(nextCalled).To(BeTrue())
					})

					It("denies a caller from a different org", func() {
						endpoint := route.NewEndpoint(&route.EndpointOpts{
							AppId:       "backend-app-id",
							Host:        "192.168.1.1",
							Port:        8080,
							AccessScope: route.AccessScopeAny,
							AccessRules: []string{"cf:org:allowed-org"},
						})
						pool := createPoolWithEndpoint(endpoint)

						extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
							ri, _ := handlers.ContextRequestInfo(r)
							ri.RoutePool = pool
							ri.CallerIdentity = &handlers.CallerIdentity{
								AppGUID: "some-app",
								OrgGUID: "other-org",
							}
							next(w, r)
						})
						buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

						Expect(nextCalled).To(BeFalse())
						Expect(recorder.Code).To(Equal(http.StatusForbidden))
					})
				})

				Context("cf:any", func() {
					It("allows any authenticated caller", func() {
						endpoint := route.NewEndpoint(&route.EndpointOpts{
							AppId:       "backend-app-id",
							Host:        "192.168.1.1",
							Port:        8080,
							AccessScope: route.AccessScopeAny,
							AccessRules: []string{"cf:any"},
						})
						pool := createPoolWithEndpoint(endpoint)

						extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
							ri, _ := handlers.ContextRequestInfo(r)
							ri.RoutePool = pool
							ri.CallerIdentity = &handlers.CallerIdentity{AppGUID: "any-random-app"}
							next(w, r)
						})
						buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

						Expect(nextCalled).To(BeTrue())
						Expect(recorder.Code).To(Equal(http.StatusOK))
					})
				})

				Context("multiple rules (OR semantics)", func() {
					It("allows when the caller matches any rule in the list", func() {
						endpoint := route.NewEndpoint(&route.EndpointOpts{
							AppId:       "backend-app-id",
							Host:        "192.168.1.1",
							Port:        8080,
							AccessScope: route.AccessScopeAny,
							AccessRules: []string{
								"cf:app:app-1",
								"cf:space:space-1",
								"cf:org:org-1",
							},
						})
						pool := createPoolWithEndpoint(endpoint)

						// Caller not in app list but IS in space list.
						extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
							ri, _ := handlers.ContextRequestInfo(r)
							ri.RoutePool = pool
							ri.CallerIdentity = &handlers.CallerIdentity{
								AppGUID:   "app-99",
								SpaceGUID: "space-1",
								OrgGUID:   "other-org",
							}
							next(w, r)
						})
						buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

						Expect(nextCalled).To(BeTrue())
						Expect(recorder.Code).To(Equal(http.StatusOK))
					})
				})
			})

			// ── RTR log fields ───────────────────────────────────────────────────

			Context("RTR log fields on successful authorization", func() {
				It("sets MtlsAuth=allowed and caller identity fields", func() {
					endpoint := route.NewEndpoint(&route.EndpointOpts{
						AppId:       "backend-app-id",
						Host:        "192.168.1.1",
						Port:        8080,
						AccessScope: route.AccessScopeAny,
						AccessRules: []string{"cf:app:caller-app"},
					})
					pool := createPoolWithEndpoint(endpoint)

					var capturedReqInfo *handlers.RequestInfo
					extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						ri, _ := handlers.ContextRequestInfo(r)
						ri.RoutePool = pool
						ri.CallerIdentity = &handlers.CallerIdentity{
							AppGUID:   "caller-app",
							SpaceGUID: "caller-space",
							OrgGUID:   "caller-org",
						}
						capturedReqInfo = ri
						next(w, r)
					})
					buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

					Expect(nextCalled).To(BeTrue())
					Expect(capturedReqInfo.MtlsAuth).To(Equal("allowed"))
					Expect(capturedReqInfo.MtlsRule).To(Equal("route:cf:app:caller-app"))
					Expect(capturedReqInfo.CallerApp).To(Equal("caller-app"))
					Expect(capturedReqInfo.CallerSpace).To(Equal("caller-space"))
					Expect(capturedReqInfo.CallerOrg).To(Equal("caller-org"))
				})
			})

			Context("RouteEndpoint is set on reqInfo for denied requests (RTR access log)", func() {
				It("sets RouteEndpoint when access rules deny the request", func() {
					endpoint := route.NewEndpoint(&route.EndpointOpts{
						AppId:       "backend-app-id",
						Host:        "192.168.1.1",
						Port:        8080,
						AccessScope: route.AccessScopeAny,
						AccessRules: []string{"cf:app:allowed-app"},
					})
					pool := createPoolWithEndpoint(endpoint)

					var capturedReqInfo *handlers.RequestInfo
					extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						ri, _ := handlers.ContextRequestInfo(r)
						ri.RoutePool = pool
						ri.CallerIdentity = &handlers.CallerIdentity{AppGUID: "denied-app"}
						capturedReqInfo = ri
						next(w, r)
					})
					buildChain(validTLSState("backend.apps.identity"), extra).ServeHTTP(recorder, request)

					Expect(recorder.Code).To(Equal(http.StatusForbidden))
					Expect(capturedReqInfo.RouteEndpoint).NotTo(BeNil())
					Expect(capturedReqInfo.RouteEndpoint.ApplicationId).To(Equal("backend-app-id"))
				})
			})
		})

		// ── Wildcard domain matching ─────────────────────────────────────────────

		Context("with wildcard mTLS domain", func() {
			It("matches subdomains under the wildcard pattern", func() {
				request = test_util.NewRequest("GET", "my-service.apps.identity", "/", nil)

				endpoint := route.NewEndpoint(&route.EndpointOpts{
					AppId:       "backend-app-id",
					Host:        "192.168.1.1",
					Port:        8080,
					AccessScope: route.AccessScopeAny,
					// No AccessRules — default deny.
				})
				pool := createPoolWithEndpoint(endpoint)

				extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
					ri, _ := handlers.ContextRequestInfo(r)
					ri.RoutePool = pool
					ri.CallerIdentity = &handlers.CallerIdentity{AppGUID: "some-app"}
					next(w, r)
				})
				buildChain(validTLSState("my-service.apps.identity"), extra).ServeHTTP(recorder, request)

				// Default deny because AccessRules is empty.
				Expect(nextCalled).To(BeFalse())
				Expect(recorder.Code).To(Equal(http.StatusForbidden))
			})
		})
	})

	Context("when multiple mTLS domains are configured", func() {
		BeforeEach(func() {
			_, caCertPEM1 := test_util.CreateKeyPair("test-ca-1")
			_, caCertPEM2 := test_util.CreateKeyPair("test-ca-2")

			cfg.Domains = []config.MtlsDomainConfig{
				{
					Domain:              "*.apps.identity",
					CACerts:             string(caCertPEM1),
					ForwardedClientCert: config.SANITIZE_SET,
				},
				{
					Domain:              "*.services.identity",
					CACerts:             string(caCertPEM2),
					ForwardedClientCert: config.SANITIZE_SET,
				},
			}
			err := cfg.Process()
			Expect(err).NotTo(HaveOccurred())

			handler = handlers.NewMtlsAuthorization(cfg, logger.Logger)
		})

		It("enforces authorization for first domain", func() {
			request = test_util.NewRequest("GET", "api.apps.identity", "/", nil)
			endpoint := route.NewEndpoint(&route.EndpointOpts{
				AppId:       "backend-app",
				Host:        "192.168.1.1",
				Port:        8080,
				AccessScope: route.AccessScopeAny,
				// No rules — default deny.
			})
			pool := createPoolWithEndpoint(endpoint)

			extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
				ri, _ := handlers.ContextRequestInfo(r)
				ri.RoutePool = pool
				ri.CallerIdentity = &handlers.CallerIdentity{AppGUID: "some-app"}
				next(w, r)
			})
			buildChain(validTLSState("api.apps.identity"), extra).ServeHTTP(recorder, request)

			Expect(nextCalled).To(BeFalse())
			Expect(recorder.Code).To(Equal(http.StatusForbidden))
		})

		It("enforces authorization for second domain", func() {
			request = test_util.NewRequest("GET", "db.services.identity", "/", nil)
			endpoint := route.NewEndpoint(&route.EndpointOpts{
				AppId:       "backend-app",
				Host:        "192.168.1.1",
				Port:        8080,
				AccessScope: route.AccessScopeAny,
				// No rules — default deny.
			})
			pool := createPoolWithEndpoint(endpoint)

			extra := negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
				ri, _ := handlers.ContextRequestInfo(r)
				ri.RoutePool = pool
				ri.CallerIdentity = &handlers.CallerIdentity{AppGUID: "some-app"}
				next(w, r)
			})
			buildChain(validTLSState("db.services.identity"), extra).ServeHTTP(recorder, request)

			Expect(nextCalled).To(BeFalse())
			Expect(recorder.Code).To(Equal(http.StatusForbidden))
		})

		It("does not enforce authorization for a non-mTLS domain", func() {
			request = test_util.NewRequest("GET", "public.example.com", "/", nil)
			buildChain(nil, nil).ServeHTTP(recorder, request)

			Expect(nextCalled).To(BeTrue())
			Expect(recorder.Code).To(Equal(http.StatusOK))
		})
	})

	// Compile-time check: SetTLSConnState is accessible from test package.
	_ = func() {
		ctx := context.Background()
		_ = handlers.SetTLSConnState(ctx, &handlers.TLSConnState{})
	}
})
