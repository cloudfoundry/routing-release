package handlers_test

import (
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

	// Helper to create a pool with an endpoint
	createPoolWithEndpoint := func(endpoint *route.Endpoint) *route.EndpointPool {
		pool := route.NewPool(&route.PoolOpts{
			Host:                   "backend.apps.mtls.internal",
			Logger:                 slog.Default(),
			LoadBalancingAlgorithm: config.LOAD_BALANCE_RR,
		})
		pool.Put(endpoint)
		return pool
	}

	BeforeEach(func() {
		logger = test_util.NewTestLogger("mtls-authorization")
		cfg, _ = config.DefaultConfig()

		// Generate a valid CA certificate for mTLS domain config
		_, caCertPEM := test_util.CreateKeyPair("test-ca")

		// Configure an mTLS domain
		cfg.MtlsDomains = []config.MtlsDomainConfig{
			{
				Domain:              "*.apps.mtls.internal",
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

	var runHandler = func() {
		// Set up handler chain with RequestInfo
		reqInfoHandler := handlers.NewRequestInfo()
		n := negroni.New()
		n.Use(reqInfoHandler)
		n.Use(handler)
		n.UseHandlerFunc(nextHandler)

		n.ServeHTTP(recorder, request)
	}

	Context("when RequestInfo is not in context", func() {
		BeforeEach(func() {
			request = test_util.NewRequest("GET", "backend.apps.mtls.internal", "/", nil)
		})

		It("returns 500 Internal Server Error", func() {
			handler.ServeHTTP(recorder, request, nextHandler)

			Expect(nextCalled).To(BeFalse())
			Expect(recorder.Code).To(Equal(http.StatusInternalServerError))
		})
	})

	Context("when request is not on an mTLS domain", func() {
		BeforeEach(func() {
			request = test_util.NewRequest("GET", "regular.example.com", "/", nil)
		})

		It("calls next handler without authorization", func() {
			runHandler()

			Expect(nextCalled).To(BeTrue())
			Expect(recorder.Code).To(Equal(http.StatusOK))
		})
	})

	Context("when request is on an mTLS domain", func() {
		BeforeEach(func() {
			request = test_util.NewRequest("GET", "backend.apps.mtls.internal", "/", nil)
		})

		Context("when no route pool is set", func() {
			BeforeEach(func() {
				// Don't set RoutePool in RequestInfo
			})

			It("returns 404 Not Found", func() {
				runHandler()

				Expect(nextCalled).To(BeFalse())
				Expect(recorder.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when route pool has no allowed sources", func() {
			BeforeEach(func() {
				// Create endpoint without allowed sources
				endpoint := route.NewEndpoint(&route.EndpointOpts{
					AppId:             "backend-app-id",
					Host:              "192.168.1.1",
					Port:              8080,
					PrivateInstanceId: "backend-instance-id",
				})

				pool := createPoolWithEndpoint(endpoint)

				// Set up request with pool but no allowed sources
				reqInfoHandler := handlers.NewRequestInfo()
				n := negroni.New()
				n.Use(reqInfoHandler)
				n.UseFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
					reqInfo, err := handlers.ContextRequestInfo(r)
					Expect(err).NotTo(HaveOccurred())
					reqInfo.RoutePool = pool
					request = r
					next(w, r)
				})
				n.Use(handler)
				n.UseHandlerFunc(nextHandler)

				n.ServeHTTP(recorder, request)
			})

			It("returns 403 Forbidden", func() {
				Expect(nextCalled).To(BeFalse())
				Expect(recorder.Code).To(Equal(http.StatusForbidden))
			})
		})

		Context("when route pool has empty allowed sources", func() {
			BeforeEach(func() {
				// Create endpoint with empty allowed sources (default deny)
				endpoint := route.NewEndpoint(&route.EndpointOpts{
					AppId:             "backend-app-id",
					Host:              "192.168.1.1",
					Port:              8080,
					PrivateInstanceId: "backend-instance-id",
					AllowedSources:    &route.AllowedSources{},
				})

				pool := createPoolWithEndpoint(endpoint)

				reqInfoHandler := handlers.NewRequestInfo()
				n := negroni.New()
				n.Use(reqInfoHandler)
				n.UseFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
					reqInfo, err := handlers.ContextRequestInfo(r)
					Expect(err).NotTo(HaveOccurred())
					reqInfo.RoutePool = pool
					request = r
					next(w, r)
				})
				n.Use(handler)
				n.UseHandlerFunc(nextHandler)

				n.ServeHTTP(recorder, request)
			})

			It("returns 403 Forbidden", func() {
				Expect(nextCalled).To(BeFalse())
				Expect(recorder.Code).To(Equal(http.StatusForbidden))
			})
		})

		Context("when route pool has allowed sources", func() {
			var endpoint *route.Endpoint
			var pool *route.EndpointPool

			BeforeEach(func() {
				// Create endpoint with allowed sources
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:             "backend-app-id",
					Host:              "192.168.1.1",
					Port:              8080,
					PrivateInstanceId: "backend-instance-id",
					AllowedSources: &route.AllowedSources{
						Apps: []string{"allowed-app-1", "allowed-app-2"},
					},
				})
				pool = createPoolWithEndpoint(endpoint)
			})

			Context("when caller identity is not set", func() {
				BeforeEach(func() {
					// Set up request with pool but no caller identity
					reqInfoHandler := handlers.NewRequestInfo()
					n := negroni.New()
					n.Use(reqInfoHandler)
					n.UseFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						reqInfo, err := handlers.ContextRequestInfo(r)
						Expect(err).NotTo(HaveOccurred())
						reqInfo.RoutePool = pool
						// Don't set CallerIdentity
						request = r
						next(w, r)
					})
					n.Use(handler)
					n.UseHandlerFunc(nextHandler)

					n.ServeHTTP(recorder, request)
				})

				It("returns 401 Unauthorized", func() {
					Expect(nextCalled).To(BeFalse())
					Expect(recorder.Code).To(Equal(http.StatusUnauthorized))
				})
			})

			Context("when caller is not in allowed sources list", func() {
				BeforeEach(func() {
					// Set up request with pool and caller identity that's not allowed
					reqInfoHandler := handlers.NewRequestInfo()
					n := negroni.New()
					n.Use(reqInfoHandler)
					n.UseFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						reqInfo, err := handlers.ContextRequestInfo(r)
						Expect(err).NotTo(HaveOccurred())
						reqInfo.RoutePool = pool
						reqInfo.CallerIdentity = &handlers.CallerIdentity{
							AppGUID: "unauthorized-app",
						}
						request = r
						next(w, r)
					})
					n.Use(handler)
					n.UseHandlerFunc(nextHandler)

					n.ServeHTTP(recorder, request)
				})

				It("returns 403 Forbidden", func() {
					Expect(nextCalled).To(BeFalse())
					Expect(recorder.Code).To(Equal(http.StatusForbidden))
				})
			})

			Context("when caller is in allowed sources list", func() {
				BeforeEach(func() {
					// Set up request with pool and authorized caller identity
					reqInfoHandler := handlers.NewRequestInfo()
					n := negroni.New()
					n.Use(reqInfoHandler)
					n.UseFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						reqInfo, err := handlers.ContextRequestInfo(r)
						Expect(err).NotTo(HaveOccurred())
						reqInfo.RoutePool = pool
						reqInfo.CallerIdentity = &handlers.CallerIdentity{
							AppGUID: "allowed-app-2",
						}
						request = r
						next(w, r)
					})
					n.Use(handler)
					n.UseHandlerFunc(nextHandler)

					n.ServeHTTP(recorder, request)
				})

				It("calls next handler", func() {
					Expect(nextCalled).To(BeTrue())
					Expect(recorder.Code).To(Equal(http.StatusOK))
				})
			})

			Context("when caller matches first app in allowed sources list", func() {
				BeforeEach(func() {
					// Test that authorization works for first app in list
					reqInfoHandler := handlers.NewRequestInfo()
					n := negroni.New()
					n.Use(reqInfoHandler)
					n.UseFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						reqInfo, err := handlers.ContextRequestInfo(r)
						Expect(err).NotTo(HaveOccurred())
						reqInfo.RoutePool = pool
						reqInfo.CallerIdentity = &handlers.CallerIdentity{
							AppGUID: "allowed-app-1",
						}
						request = r
						next(w, r)
					})
					n.Use(handler)
					n.UseHandlerFunc(nextHandler)

					n.ServeHTTP(recorder, request)
				})

				It("calls next handler", func() {
					Expect(nextCalled).To(BeTrue())
					Expect(recorder.Code).To(Equal(http.StatusOK))
				})
			})
		})

		Context("with wildcard mTLS domain matching", func() {
			BeforeEach(func() {
				// Test with specific subdomain under wildcard
				request = test_util.NewRequest("GET", "my-service.apps.mtls.internal", "/", nil)
			})

			Context("when pool has no allowed sources", func() {
				BeforeEach(func() {
					endpoint := route.NewEndpoint(&route.EndpointOpts{
						AppId:             "backend-app-id",
						Host:              "192.168.1.1",
						Port:              8080,
						PrivateInstanceId: "backend-instance-id",
					})

					pool := createPoolWithEndpoint(endpoint)

					reqInfoHandler := handlers.NewRequestInfo()
					n := negroni.New()
					n.Use(reqInfoHandler)
					n.UseFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						reqInfo, err := handlers.ContextRequestInfo(r)
						Expect(err).NotTo(HaveOccurred())
						reqInfo.RoutePool = pool
						request = r
						next(w, r)
					})
					n.Use(handler)
					n.UseHandlerFunc(nextHandler)

					n.ServeHTTP(recorder, request)
				})

				It("returns 403 Forbidden", func() {
					Expect(nextCalled).To(BeFalse())
					Expect(recorder.Code).To(Equal(http.StatusForbidden))
				})
			})
		})
	})

	Context("when multiple mTLS domains are configured", func() {
		BeforeEach(func() {
			// Generate valid CA certificates for mTLS domain configs
			_, caCertPEM1 := test_util.CreateKeyPair("test-ca-1")
			_, caCertPEM2 := test_util.CreateKeyPair("test-ca-2")

			// Configure multiple mTLS domains
			cfg.MtlsDomains = []config.MtlsDomainConfig{
				{
					Domain:              "*.apps.mtls.internal",
					CACerts:             string(caCertPEM1),
					ForwardedClientCert: config.SANITIZE_SET,
				},
				{
					Domain:              "*.services.mtls.internal",
					CACerts:             string(caCertPEM2),
					ForwardedClientCert: config.SANITIZE_SET,
				},
			}
			err := cfg.Process()
			Expect(err).NotTo(HaveOccurred())

			handler = handlers.NewMtlsAuthorization(cfg, logger.Logger)
		})

		Context("when request is on first mTLS domain", func() {
			BeforeEach(func() {
				request = test_util.NewRequest("GET", "api.apps.mtls.internal", "/", nil)
			})

			It("enforces authorization for first domain", func() {
				endpoint := route.NewEndpoint(&route.EndpointOpts{
					AppId:             "backend-app",
					Host:              "192.168.1.1",
					Port:              8080,
					PrivateInstanceId: "instance-id",
				})
				pool := createPoolWithEndpoint(endpoint)

				reqInfoHandler := handlers.NewRequestInfo()
				n := negroni.New()
				n.Use(reqInfoHandler)
				n.UseFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
					reqInfo, err := handlers.ContextRequestInfo(r)
					Expect(err).NotTo(HaveOccurred())
					reqInfo.RoutePool = pool
					request = r
					next(w, r)
				})
				n.Use(handler)
				n.UseHandlerFunc(nextHandler)

				n.ServeHTTP(recorder, request)

				Expect(nextCalled).To(BeFalse())
				Expect(recorder.Code).To(Equal(http.StatusForbidden))
			})
		})

		Context("when request is on second mTLS domain", func() {
			BeforeEach(func() {
				request = test_util.NewRequest("GET", "db.services.mtls.internal", "/", nil)
			})

			It("enforces authorization for second domain", func() {
				endpoint := route.NewEndpoint(&route.EndpointOpts{
					AppId:             "backend-app",
					Host:              "192.168.1.1",
					Port:              8080,
					PrivateInstanceId: "instance-id",
				})
				pool := createPoolWithEndpoint(endpoint)

				reqInfoHandler := handlers.NewRequestInfo()
				n := negroni.New()
				n.Use(reqInfoHandler)
				n.UseFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
					reqInfo, err := handlers.ContextRequestInfo(r)
					Expect(err).NotTo(HaveOccurred())
					reqInfo.RoutePool = pool
					request = r
					next(w, r)
				})
				n.Use(handler)
				n.UseHandlerFunc(nextHandler)

				n.ServeHTTP(recorder, request)

				Expect(nextCalled).To(BeFalse())
				Expect(recorder.Code).To(Equal(http.StatusForbidden))
			})
		})
	})

	Context("with RFC-compliant AllowedSources authorization", func() {
		BeforeEach(func() {
			request = test_util.NewRequest("GET", "backend.apps.mtls.internal", "/", nil)
		})

		Context("when AllowedSources.Any is true", func() {
			var endpoint *route.Endpoint
			var pool *route.EndpointPool

			BeforeEach(func() {
				endpoint = route.NewEndpoint(&route.EndpointOpts{
					AppId:             "backend-app-id",
					Host:              "192.168.1.1",
					Port:              8080,
					PrivateInstanceId: "backend-instance-id",
					AllowedSources: &route.AllowedSources{
						Any: true,
					},
				})
				pool = createPoolWithEndpoint(endpoint)
			})

			Context("when caller is authenticated", func() {
				BeforeEach(func() {
					reqInfoHandler := handlers.NewRequestInfo()
					n := negroni.New()
					n.Use(reqInfoHandler)
					n.UseFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						reqInfo, err := handlers.ContextRequestInfo(r)
						Expect(err).NotTo(HaveOccurred())
						reqInfo.RoutePool = pool
						reqInfo.CallerIdentity = &handlers.CallerIdentity{
							AppGUID: "random-app-guid",
						}
						request = r
						next(w, r)
					})
					n.Use(handler)
					n.UseHandlerFunc(nextHandler)

					n.ServeHTTP(recorder, request)
				})

				It("allows any authenticated app", func() {
					Expect(nextCalled).To(BeTrue())
					Expect(recorder.Code).To(Equal(http.StatusOK))
				})
			})

			Context("when caller is not authenticated", func() {
				BeforeEach(func() {
					reqInfoHandler := handlers.NewRequestInfo()
					n := negroni.New()
					n.Use(reqInfoHandler)
					n.UseFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
						reqInfo, err := handlers.ContextRequestInfo(r)
						Expect(err).NotTo(HaveOccurred())
						reqInfo.RoutePool = pool
						// Don't set CallerIdentity
						request = r
						next(w, r)
					})
					n.Use(handler)
					n.UseHandlerFunc(nextHandler)

					n.ServeHTTP(recorder, request)
				})

				It("returns 401 Unauthorized", func() {
					Expect(nextCalled).To(BeFalse())
					Expect(recorder.Code).To(Equal(http.StatusUnauthorized))
				})
			})
		})

		Context("when caller's space is in AllowedSources.Spaces", func() {
			BeforeEach(func() {
				endpoint := route.NewEndpoint(&route.EndpointOpts{
					AppId:             "backend-app-id",
					Host:              "192.168.1.1",
					Port:              8080,
					PrivateInstanceId: "backend-instance-id",
					AllowedSources: &route.AllowedSources{
						Spaces: []string{"allowed-space-1", "allowed-space-2"},
					},
				})
				pool := createPoolWithEndpoint(endpoint)

				reqInfoHandler := handlers.NewRequestInfo()
				n := negroni.New()
				n.Use(reqInfoHandler)
				n.UseFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
					reqInfo, err := handlers.ContextRequestInfo(r)
					Expect(err).NotTo(HaveOccurred())
					reqInfo.RoutePool = pool
					reqInfo.CallerIdentity = &handlers.CallerIdentity{
						AppGUID:   "caller-app-guid",
						SpaceGUID: "allowed-space-2",
					}
					request = r
					next(w, r)
				})
				n.Use(handler)
				n.UseHandlerFunc(nextHandler)

				n.ServeHTTP(recorder, request)
			})

			It("allows the request", func() {
				Expect(nextCalled).To(BeTrue())
				Expect(recorder.Code).To(Equal(http.StatusOK))
			})
		})

		Context("when caller's space is not in AllowedSources.Spaces", func() {
			BeforeEach(func() {
				endpoint := route.NewEndpoint(&route.EndpointOpts{
					AppId:             "backend-app-id",
					Host:              "192.168.1.1",
					Port:              8080,
					PrivateInstanceId: "backend-instance-id",
					AllowedSources: &route.AllowedSources{
						Spaces: []string{"allowed-space-1", "allowed-space-2"},
					},
				})
				pool := createPoolWithEndpoint(endpoint)

				reqInfoHandler := handlers.NewRequestInfo()
				n := negroni.New()
				n.Use(reqInfoHandler)
				n.UseFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
					reqInfo, err := handlers.ContextRequestInfo(r)
					Expect(err).NotTo(HaveOccurred())
					reqInfo.RoutePool = pool
					reqInfo.CallerIdentity = &handlers.CallerIdentity{
						AppGUID:   "caller-app-guid",
						SpaceGUID: "different-space",
					}
					request = r
					next(w, r)
				})
				n.Use(handler)
				n.UseHandlerFunc(nextHandler)

				n.ServeHTTP(recorder, request)
			})

			It("returns 403 Forbidden", func() {
				Expect(nextCalled).To(BeFalse())
				Expect(recorder.Code).To(Equal(http.StatusForbidden))
			})
		})

		Context("when caller's org is in AllowedSources.Orgs", func() {
			BeforeEach(func() {
				endpoint := route.NewEndpoint(&route.EndpointOpts{
					AppId:             "backend-app-id",
					Host:              "192.168.1.1",
					Port:              8080,
					PrivateInstanceId: "backend-instance-id",
					AllowedSources: &route.AllowedSources{
						Orgs: []string{"allowed-org-1", "allowed-org-2"},
					},
				})
				pool := createPoolWithEndpoint(endpoint)

				reqInfoHandler := handlers.NewRequestInfo()
				n := negroni.New()
				n.Use(reqInfoHandler)
				n.UseFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
					reqInfo, err := handlers.ContextRequestInfo(r)
					Expect(err).NotTo(HaveOccurred())
					reqInfo.RoutePool = pool
					reqInfo.CallerIdentity = &handlers.CallerIdentity{
						AppGUID: "caller-app-guid",
						OrgGUID: "allowed-org-1",
					}
					request = r
					next(w, r)
				})
				n.Use(handler)
				n.UseHandlerFunc(nextHandler)

				n.ServeHTTP(recorder, request)
			})

			It("allows the request", func() {
				Expect(nextCalled).To(BeTrue())
				Expect(recorder.Code).To(Equal(http.StatusOK))
			})
		})

		Context("when caller's org is not in AllowedSources.Orgs", func() {
			BeforeEach(func() {
				endpoint := route.NewEndpoint(&route.EndpointOpts{
					AppId:             "backend-app-id",
					Host:              "192.168.1.1",
					Port:              8080,
					PrivateInstanceId: "backend-instance-id",
					AllowedSources: &route.AllowedSources{
						Orgs: []string{"allowed-org-1", "allowed-org-2"},
					},
				})
				pool := createPoolWithEndpoint(endpoint)

				reqInfoHandler := handlers.NewRequestInfo()
				n := negroni.New()
				n.Use(reqInfoHandler)
				n.UseFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
					reqInfo, err := handlers.ContextRequestInfo(r)
					Expect(err).NotTo(HaveOccurred())
					reqInfo.RoutePool = pool
					reqInfo.CallerIdentity = &handlers.CallerIdentity{
						AppGUID: "caller-app-guid",
						OrgGUID: "different-org",
					}
					request = r
					next(w, r)
				})
				n.Use(handler)
				n.UseHandlerFunc(nextHandler)

				n.ServeHTTP(recorder, request)
			})

			It("returns 403 Forbidden", func() {
				Expect(nextCalled).To(BeFalse())
				Expect(recorder.Code).To(Equal(http.StatusForbidden))
			})
		})

		Context("with multiple authorization levels", func() {
			BeforeEach(func() {
				// Endpoint allows specific apps, specific spaces, and specific orgs
				endpoint := route.NewEndpoint(&route.EndpointOpts{
					AppId:             "backend-app-id",
					Host:              "192.168.1.1",
					Port:              8080,
					PrivateInstanceId: "backend-instance-id",
					AllowedSources: &route.AllowedSources{
						Apps:   []string{"app-1", "app-2"},
						Spaces: []string{"space-1"},
						Orgs:   []string{"org-1"},
					},
				})
				pool := createPoolWithEndpoint(endpoint)

				reqInfoHandler := handlers.NewRequestInfo()
				n := negroni.New()
				n.Use(reqInfoHandler)
				n.UseFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
					reqInfo, err := handlers.ContextRequestInfo(r)
					Expect(err).NotTo(HaveOccurred())
					reqInfo.RoutePool = pool
					// Caller is not in the app list, but is in the allowed space
					reqInfo.CallerIdentity = &handlers.CallerIdentity{
						AppGUID:   "app-3",
						SpaceGUID: "space-1",
						OrgGUID:   "different-org",
					}
					request = r
					next(w, r)
				})
				n.Use(handler)
				n.UseHandlerFunc(nextHandler)

				n.ServeHTTP(recorder, request)
			})

			It("allows if any level matches", func() {
				Expect(nextCalled).To(BeTrue())
				Expect(recorder.Code).To(Equal(http.StatusOK))
			})
		})
	})
})
