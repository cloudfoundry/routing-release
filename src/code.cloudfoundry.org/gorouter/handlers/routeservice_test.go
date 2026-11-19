package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/urfave/negroni/v3"
	"go.uber.org/zap/zapcore"

	"code.cloudfoundry.org/gorouter/common/secure"
	"code.cloudfoundry.org/gorouter/errorwriter"
	fakeRegistry "code.cloudfoundry.org/gorouter/registry/fakes"
	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/routeservice"
	"code.cloudfoundry.org/gorouter/test_util"
)

var _ = Describe("Route Service Handler", func() {
	var (
		handler *negroni.Negroni

		reg      *fakeRegistry.FakeRegistry
		routeMap map[string]*route.EndpointPool

		resp *httptest.ResponseRecorder
		req  *http.Request

		config       *routeservice.RouteServiceConfig
		crypto       *secure.AesGCM
		routePool    *route.EndpointPool
		forwardedUrl string

		ew = errorwriter.NewPlaintextErrorWriter()

		reqChan chan *http.Request

		nextCalled bool
		logger     *test_util.TestLogger
	)

	nextHandler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		_, err := io.ReadAll(req.Body)
		Expect(err).NotTo(HaveOccurred())

		reqChan <- req
		rw.WriteHeader(http.StatusTeapot)
		rw.Write([]byte("I'm a little teapot, short and stout."))

		nextCalled = true
	})

	testSetupHandler := func(rw http.ResponseWriter, req *http.Request, next http.HandlerFunc) {
		reqInfo, err := ContextRequestInfo(req)
		Expect(err).ToNot(HaveOccurred())
		reqInfo.RoutePool = routePool
		next(rw, req)
	}

	BeforeEach(func() {
		body := bytes.NewBufferString("What are you?")
		testReq := test_util.NewRequest("GET", "my_host.com", "/resource+9-9_9?query=dog%0Acat&query$2=345#page1..5", body)
		forwardedUrl = "https://my_host.com/resource+9-9_9?query=dog%0Acat&query$2=345#page1..5"
		reqBuf := new(bytes.Buffer)
		err := testReq.Write(reqBuf)
		Expect(err).ToNot(HaveOccurred())
		req, err = http.ReadRequest(bufio.NewReader(reqBuf))
		Expect(err).ToNot(HaveOccurred())

		logger = test_util.NewTestLogger("test")

		resp = httptest.NewRecorder()

		reqChan = make(chan *http.Request, 1)

		routePool = route.NewPool(&route.PoolOpts{
			Logger:             logger.Logger,
			RetryAfterFailure:  1 * time.Second,
			Host:               "my_host.com",
			ContextPath:        "/resource+9-9_9",
			MaxConnsPerBackend: 0,
		})

		reg = &fakeRegistry.FakeRegistry{}
		routeMap = make(map[string]*route.EndpointPool)
		reg.LookupStub = func(uri route.Uri) *route.EndpointPool {
			return routeMap[uri.String()]
		}
		routeMap["my_host.com/resource+9-9_9"] = routePool

		crypto, err = secure.NewAesGCM([]byte("ABCDEFGHIJKLMNOP"))
		Expect(err).NotTo(HaveOccurred())
		config = routeservice.NewRouteServiceConfig(
			logger.Logger, true, true, nil, 60*time.Second, crypto, nil, true, false, true, true, []netip.Prefix{})

		nextCalled = false
	})

	AfterEach(func() {
		close(reqChan)
	})

	It("logs when a signed return request is not bound to the current route service", func() {
		routePool.Put(route.NewEndpoint(&route.EndpointOpts{RouteServiceUrl: "https://route-service.com"}))

		handler.ServeHTTP(resp, req)
		var passedReq *http.Request
		Eventually(reqChan).Should(Receive(&passedReq))
		Expect(strings.Join(logger.Lines(zapcore.InfoLevel), "\n")).NotTo(ContainSubstring("route-service-signature-not-trusted-for-current-route-service"))

		routeServiceRequest, err := config.CreateRequest("https://other-route-service.com", forwardedUrl)
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set(routeservice.HeaderKeySignature, routeServiceRequest.Signature)
		req.Header.Set(routeservice.HeaderKeyMetadata, routeServiceRequest.Metadata)
		handler.ServeHTTP(resp, req)
		Eventually(reqChan).Should(Receive(&passedReq))

		Expect(passedReq.Header.Get(routeservice.HeaderKeySignature)).NotTo(BeEmpty())
		Expect(strings.Join(logger.Lines(zapcore.InfoLevel), "\n")).To(ContainSubstring("route-service-signature-not-trusted-for-current-route-service"))
	})

	JustBeforeEach(func() {
		handler = negroni.New()
		handler.Use(NewRequestInfo())
		handler.UseFunc(testSetupHandler)
		handler.Use(negroni.HandlerFunc(func(rw http.ResponseWriter, req *http.Request, next http.HandlerFunc) {
			next(rw, req)
		}))
		handler.Use(NewRouteService(config, reg, logger.Logger, ew))
		handler.UseHandlerFunc(nextHandler)
	})

	Context("with route services disabled", func() {
		BeforeEach(func() {
			config = routeservice.NewRouteServiceConfig(logger.Logger, false, false, nil, 0, nil, nil, false, false, true, true, nil)
		})

		Context("for normal routes", func() {
			BeforeEach(func() {
				endpoint := route.NewEndpoint(&route.EndpointOpts{})

				added := routePool.Put(endpoint)
				Expect(added).To(Equal(route.EndpointAdded))
			})
			It("should not add route service metadata to the request for normal routes", func() {
				handler.ServeHTTP(resp, req)

				var passedReq *http.Request
				Eventually(reqChan).Should(Receive(&passedReq))

				Expect(passedReq.Header.Get(routeservice.HeaderKeySignature)).To(BeEmpty())
				Expect(passedReq.Header.Get(routeservice.HeaderKeyMetadata)).To(BeEmpty())
				Expect(passedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(BeEmpty())

				reqInfo, err := ContextRequestInfo(passedReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(reqInfo.RouteServiceURL).To(BeNil())
				Expect(nextCalled).To(BeTrue(), "Expected the next handler to be called.")
			})
		})

		Context("with route service URL configured for the route", func() {
			BeforeEach(func() {
				endpoint := route.NewEndpoint(&route.EndpointOpts{RouteServiceUrl: "route-service.com"})
				added := routePool.Put(endpoint)
				Expect(added).To(Equal(route.EndpointAdded))
			})

			It("returns 502 Bad Gateway", func() {
				handler.ServeHTTP(resp, req)

				Eventually(logger).Should(gbytes.Say(`route-service-unsupported`))
				Expect(resp.Code).To(Equal(http.StatusBadGateway))
				Expect(resp.Header().Get("X-Cf-RouterError")).To(Equal(`route_service_unsupported`))
				Expect(resp.Body.String()).To(ContainSubstring(`Support for route services is disabled.`))
				Expect(nextCalled).To(BeFalse())
			})
		})
	})

	Context("with Route Services enabled", func() {
		Context("for normal routes", func() {
			BeforeEach(func() {
				endpoint := route.NewEndpoint(&route.EndpointOpts{})
				added := routePool.Put(endpoint)
				Expect(added).To(Equal(route.EndpointAdded))
			})
			It("should not add route service metadata to the request for normal routes", func() {
				handler.ServeHTTP(resp, req)

				var passedReq *http.Request
				Eventually(reqChan).Should(Receive(&passedReq))

				Expect(passedReq.Header.Get(routeservice.HeaderKeySignature)).To(BeEmpty())
				Expect(passedReq.Header.Get(routeservice.HeaderKeyMetadata)).To(BeEmpty())
				Expect(passedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(BeEmpty())
				reqInfo, err := ContextRequestInfo(passedReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(reqInfo.RouteServiceURL).To(BeNil())
				Expect(nextCalled).To(BeTrue(), "Expected the next handler to be called.")
			})

			Context("with strictSignatureValidation enabled", func() {
				BeforeEach(func() {
					config = routeservice.NewRouteServiceConfig(
						logger.Logger, true, false, nil, 60*time.Second, crypto, nil, false, true, true, true, nil,
					)
				})

				It("rejects all invalid signature headers", func() {
					req.Header.Add(routeservice.HeaderKeySignature, "invalid")
					handler.ServeHTTP(resp, req)
					Expect(resp.Code).To(Equal(http.StatusBadRequest))
					Expect(resp.Header().Get("X-Cf-RouterError")).To(Equal("invalid_route_service_signature"))
					Expect(resp.Body.String()).To(ContainSubstring("invalid route service signature detected"))
				})
			})

		})

		Context("with route service URL configured for the route", func() {
			BeforeEach(func() {
				endpoint := route.NewEndpoint(&route.EndpointOpts{RouteServiceUrl: "https://route-service.com"})
				added := routePool.Put(endpoint)
				Expect(added).To(Equal(route.EndpointAdded))
			})

			It("sends the request to the route service with X-CF-Forwarded-Url using https scheme", func() {
				handler.ServeHTTP(resp, req)

				Expect(resp.Code).To(Equal(http.StatusTeapot))

				var passedReq *http.Request
				Eventually(reqChan).Should(Receive(&passedReq))

				Expect(passedReq.Header.Get(routeservice.HeaderKeySignature)).ToNot(BeEmpty())
				Expect(passedReq.Header.Get(routeservice.HeaderKeyMetadata)).ToNot(BeEmpty())
				Expect(passedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(ContainSubstring("https://"))

				reqInfo, err := ContextRequestInfo(passedReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(reqInfo.RouteServiceURL).ToNot(BeNil())

				Expect(reqInfo.RouteServiceURL.Host).To(Equal("route-service.com"))
				Expect(reqInfo.RouteServiceURL.Scheme).To(Equal("https"))
				Expect(reqInfo.ShouldRouteToInternalRouteService).To(BeFalse())
				Expect(nextCalled).To(BeTrue(), "Expected the next handler to be called.")
			})

			Context("when the request target is in absolute-form", func() {
				assertForwardedURL := func(passedReq *http.Request, expectedURL string) {
					Expect(passedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(Equal(expectedURL))

					sig := passedReq.Header.Get(routeservice.HeaderKeySignature)
					meta := passedReq.Header.Get(routeservice.HeaderKeyMetadata)
					signatureContents, err := routeservice.SignatureContentsFromHeaders(sig, meta, crypto)
					Expect(err).NotTo(HaveOccurred())
					Expect(signatureContents.ForwardedUrl).To(Equal(expectedURL))
				}

				readRaw := func(target, host string) *http.Request {
					raw := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, host)
					r, err := http.ReadRequest(bufio.NewReader(strings.NewReader(raw)))
					Expect(err).NotTo(HaveOccurred())
					return r
				}

				// roundTrip sends clientTarget to the handler, then replays the
				// signed headers on callbackTarget as the route service would.
				roundTrip := func(clientTarget, callbackTarget, expectedURL string) {
					req = readRaw(clientTarget, "my_host.com")
					handler.ServeHTTP(resp, req)

					var passedReq *http.Request
					Eventually(reqChan).Should(Receive(&passedReq))
					assertForwardedURL(passedReq, expectedURL)

					nextCalled = false
					resp = httptest.NewRecorder()
					callbackReq := readRaw(callbackTarget, "my_host.com")
					callbackReq.Header.Set(routeservice.HeaderKeySignature, passedReq.Header.Get(routeservice.HeaderKeySignature))
					callbackReq.Header.Set(routeservice.HeaderKeyMetadata, passedReq.Header.Get(routeservice.HeaderKeyMetadata))

					handler.ServeHTTP(resp, callbackReq)

					Expect(resp.Code).To(Equal(http.StatusTeapot))
					Expect(nextCalled).To(BeTrue())

					var secondPassedReq *http.Request
					Eventually(reqChan).Should(Receive(&secondPassedReq))
					Expect(secondPassedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(BeEmpty())
				}

				DescribeTable("sets the forwarded URL",
					func(target, host, expectedURL string) {
						req = readRaw(target, host)
						handler.ServeHTTP(resp, req)

						var passedReq *http.Request
						Eventually(reqChan).Should(Receive(&passedReq))

						assertForwardedURL(passedReq, expectedURL)
					},
					Entry("sets the forwarded URL for an absolute-form request target",
						"http://my_host.com/resource+9-9_9?query=dog",
						"my_host.com",
						"https://my_host.com/resource+9-9_9?query=dog",
					),
					Entry("strips userinfo from an absolute-form request target",
						"http://user:pw@my_host.com/resource+9-9_9?query=dog",
						"my_host.com",
						"https://my_host.com/resource+9-9_9?query=dog",
					),
					// http.ReadRequest (like the net/http server) sets req.Host from
					// the absolute-form authority, ignoring the Host header;
					// forwardedURL then takes the host from req.Host.
					Entry("uses req.Host, which net/http takes from the absolute-form authority",
						"http://my_host.com:8080/resource+9-9_9?query=dog",
						"other.example.com",
						"https://my_host.com/resource+9-9_9?query=dog",
					),
					Entry("normalizes an empty path in an absolute-form target to a slash",
						"http://my_host.com",
						"my_host.com",
						"https://my_host.com/",
					),
					Entry("prefixes a slash to a query-only absolute-form target",
						"http://my_host.com?query=dog",
						"my_host.com",
						"https://my_host.com/?query=dog",
					),
					Entry("keeps the path of an absolute-form target with no authority",
						"http:/resource+9-9_9?query=dog",
						"my_host.com",
						"https://my_host.com/resource+9-9_9?query=dog",
					),
				)

				DescribeTable("keeps the request-target path and query bytes unchanged",
					func(target, expectedURI string) {
						req = readRaw(target, "my_host.com")
						handler.ServeHTTP(resp, req)

						var passedReq *http.Request
						Eventually(reqChan).Should(Receive(&passedReq))

						assertForwardedURL(passedReq, "https://my_host.com"+expectedURI)
					},
					Entry("origin-form /a|b", "/a|b", "/a|b"),
					Entry("origin-form /a%2Fb", "/a%2Fb", "/a%2Fb"),
					Entry("origin-form /p?", "/p?", "/p?"),
					Entry("origin-form //x", "//x", "//x"),
					Entry("absolute-form /a|b", "http://my_host.com/a|b", "/a|b"),
					Entry("absolute-form /a%2Fb", "http://my_host.com/a%2Fb", "/a%2Fb"),
					Entry("absolute-form /p?", "http://my_host.com/p?", "/p?"),
					Entry("absolute-form //x", "http://my_host.com//x", "//x"),
				)

				DescribeTable("completes the round trip when the client started with an absolute-form target",
					roundTrip,
					Entry("with an origin-form callback",
						"http://my_host.com/resource+9-9_9?query=dog",
						"/resource+9-9_9?query=dog",
						"https://my_host.com/resource+9-9_9?query=dog",
					),
					Entry("with an absolute-form callback",
						"http://my_host.com/resource+9-9_9?query=dog",
						"https://my_host.com/resource+9-9_9?query=dog",
						"https://my_host.com/resource+9-9_9?query=dog",
					),
				)

				Context("when recommendHttps is set to false", func() {
					BeforeEach(func() {
						config = routeservice.NewRouteServiceConfig(
							logger.Logger, true, true, nil, 60*time.Second, crypto, nil, false, false, true, true, []netip.Prefix{})
					})

					DescribeTable("signs an http forwarded URL and completes the round trip",
						roundTrip,
						Entry("with an origin-form callback",
							"http://my_host.com/resource+9-9_9?query=dog",
							"/resource+9-9_9?query=dog",
							"http://my_host.com/resource+9-9_9?query=dog",
						),
						Entry("with an absolute-form callback",
							"http://my_host.com/resource+9-9_9?query=dog",
							"http://my_host.com/resource+9-9_9?query=dog",
							"http://my_host.com/resource+9-9_9?query=dog",
						),
					)
				})
			})

			It("keeps escaped paths and query bytes in X-CF-Forwarded-Url", func() {
				req.RequestURI = "/a%2Fb?q=value"
				req.URL.Path = "/a/b"
				req.URL.RawPath = "/a%2Fb"
				req.URL.RawQuery = "q=value"
				req.URL.ForceQuery = false

				handler.ServeHTTP(resp, req)

				var passedReq *http.Request
				Eventually(reqChan).Should(Receive(&passedReq))
				Expect(passedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(Equal("https://my_host.com/a%2Fb?q=value"))
			})

			It("keeps a bare query marker in X-CF-Forwarded-Url", func() {
				req.RequestURI = "/a%2Fb?"
				req.URL.Path = "/a/b"
				req.URL.RawPath = "/a%2Fb"
				req.URL.RawQuery = ""
				req.URL.ForceQuery = true
				handler.ServeHTTP(resp, req)

				var passedReq *http.Request
				Eventually(reqChan).Should(Receive(&passedReq))
				Expect(passedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(Equal("https://my_host.com/a%2Fb?"))
			})

			It("keeps origin-form request-target bytes in X-CF-Forwarded-Url", func() {
				req.RequestURI = "/a|b?x=1"
				req.URL.Path = "/a|b"
				req.URL.RawPath = ""
				req.URL.RawQuery = "x=1"

				handler.ServeHTTP(resp, req)

				var passedReq *http.Request
				Eventually(reqChan).Should(Receive(&passedReq))
				Expect(passedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(Equal("https://my_host.com/a|b?x=1"))
			})

			It("uses the routed host for absolute-form requests", func() {
				parsedURL, err := http.NewRequest(http.MethodGet, "http://other.example/p?q", nil)
				Expect(err).NotTo(HaveOccurred())
				req.URL = parsedURL.URL
				req.Host = "app.example:8080"
				req.RequestURI = "http://other.example/p?q"

				handler.ServeHTTP(resp, req)

				var passedReq *http.Request
				Eventually(reqChan).Should(Receive(&passedReq))
				Expect(passedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(Equal("https://app.example/p?q"))
			})

			It("omits user info from absolute-form requests", func() {
				parsedURL, err := http.NewRequest(http.MethodGet, "http://u:p@app.example/p?q", nil)
				Expect(err).NotTo(HaveOccurred())
				req.URL = parsedURL.URL
				req.Host = "app.example"
				req.RequestURI = "http://u:p@app.example/p?q"

				handler.ServeHTTP(resp, req)

				var passedReq *http.Request
				Eventually(reqChan).Should(Receive(&passedReq))
				Expect(passedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(Equal("https://app.example/p?q"))
			})

			Context("when the route service has a route in the route registry", func() {
				BeforeEach(func() {
					rsPool := route.NewPool(&route.PoolOpts{
						Logger:             logger.Logger,
						RetryAfterFailure:  2 * time.Minute,
						Host:               "route-service.com",
						ContextPath:        "/",
						MaxConnsPerBackend: 0,
					})
					routeMap["route-service.com"] = rsPool
				})

				It("adds a flag to the request context", func() {
					handler.ServeHTTP(resp, req)

					Expect(resp.Code).To(Equal(http.StatusTeapot))

					var passedReq *http.Request
					Eventually(reqChan).Should(Receive(&passedReq))

					Expect(passedReq.Header.Get(routeservice.HeaderKeySignature)).ToNot(BeEmpty())
					Expect(passedReq.Header.Get(routeservice.HeaderKeyMetadata)).ToNot(BeEmpty())
					Expect(passedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(ContainSubstring("https://"))

					reqInfo, err := ContextRequestInfo(passedReq)
					Expect(err).ToNot(HaveOccurred())
					Expect(reqInfo.RouteServiceURL).ToNot(BeNil())

					Expect(reqInfo.RouteServiceURL.Host).To(Equal("route-service.com"))
					Expect(reqInfo.RouteServiceURL.Scheme).To(Equal("https"))
					Expect(reqInfo.ShouldRouteToInternalRouteService).To(BeTrue())
					Expect(nextCalled).To(BeTrue(), "Expected the next handler to be called.")
				})

				Context("when the hairpin feature flag is disabled", func() {
					BeforeEach(func() {
						hairpinning := false
						config = routeservice.NewRouteServiceConfig(
							logger.Logger, true, hairpinning, nil, 60*time.Second, crypto, nil, true, false, true, true, nil,
						)
					})

					It("does not add a flag to the request context", func() {
						handler.ServeHTTP(resp, req)

						Expect(resp.Code).To(Equal(http.StatusTeapot))

						var passedReq *http.Request
						Eventually(reqChan).Should(Receive(&passedReq))

						Expect(passedReq.Header.Get(routeservice.HeaderKeySignature)).ToNot(BeEmpty())
						Expect(passedReq.Header.Get(routeservice.HeaderKeyMetadata)).ToNot(BeEmpty())
						Expect(passedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(ContainSubstring("https://"))

						reqInfo, err := ContextRequestInfo(passedReq)
						Expect(err).ToNot(HaveOccurred())
						Expect(reqInfo.RouteServiceURL).ToNot(BeNil())

						Expect(reqInfo.RouteServiceURL.Host).To(Equal("route-service.com"))
						Expect(reqInfo.RouteServiceURL.Scheme).To(Equal("https"))
						Expect(reqInfo.ShouldRouteToInternalRouteService).To(BeFalse())
						Expect(nextCalled).To(BeTrue(), "Expected the next handler to be called.")
					})

				})
				Context("when the hairpin feature flag is enabled", func() {
					BeforeEach(func() {
						hairpinning := true
						config = routeservice.NewRouteServiceConfig(
							logger.Logger, true, hairpinning, nil, 60*time.Second, crypto, nil, true, false, true, true, nil,
						)
					})

					It("adds a flag to the request context", func() {
						handler.ServeHTTP(resp, req)

						Expect(resp.Code).To(Equal(http.StatusTeapot))

						var passedReq *http.Request
						Eventually(reqChan).Should(Receive(&passedReq))

						Expect(passedReq.Header.Get(routeservice.HeaderKeySignature)).ToNot(BeEmpty())
						Expect(passedReq.Header.Get(routeservice.HeaderKeyMetadata)).ToNot(BeEmpty())
						Expect(passedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(ContainSubstring("https://"))

						reqInfo, err := ContextRequestInfo(passedReq)
						Expect(err).ToNot(HaveOccurred())
						Expect(reqInfo.RouteServiceURL).ToNot(BeNil())

						Expect(reqInfo.RouteServiceURL.Host).To(Equal("route-service.com"))
						Expect(reqInfo.RouteServiceURL.Scheme).To(Equal("https"))
						Expect(reqInfo.ShouldRouteToInternalRouteService).To(BeTrue())
						Expect(nextCalled).To(BeTrue(), "Expected the next handler to be called.")
					})

				})
				Context("when the hairpin feature flag is enabled with allowlist", func() {
					BeforeEach(func() {
						hairpinning := true
						config = routeservice.NewRouteServiceConfig(
							logger.Logger, true, hairpinning, []string{"route-service.com"}, 60*time.Second, crypto, nil, true, false, true, true, nil,
						)
					})

					It("adds a flag to the request context", func() {
						handler.ServeHTTP(resp, req)

						Expect(resp.Code).To(Equal(http.StatusTeapot))

						var passedReq *http.Request
						Eventually(reqChan).Should(Receive(&passedReq))

						Expect(passedReq.Header.Get(routeservice.HeaderKeySignature)).ToNot(BeEmpty())
						Expect(passedReq.Header.Get(routeservice.HeaderKeyMetadata)).ToNot(BeEmpty())
						Expect(passedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(ContainSubstring("https://"))

						reqInfo, err := ContextRequestInfo(passedReq)
						Expect(err).ToNot(HaveOccurred())
						Expect(reqInfo.RouteServiceURL).ToNot(BeNil())

						Expect(reqInfo.RouteServiceURL.Host).To(Equal("route-service.com"))
						Expect(reqInfo.RouteServiceURL.Scheme).To(Equal("https"))
						Expect(reqInfo.ShouldRouteToInternalRouteService).To(BeTrue())
						Expect(nextCalled).To(BeTrue(), "Expected the next handler to be called.")
					})

				})

				Context("when the hairpin feature flag is enabled, only with not matching allowlist entries", func() {
					BeforeEach(func() {
						hairpinning := true
						config = routeservice.NewRouteServiceConfig(
							logger.Logger, true, hairpinning, []string{"example.com"}, 60*time.Second, crypto, nil, true, false, true, true, nil,
						)
					})

					It("does not add a flag to the request context", func() {
						handler.ServeHTTP(resp, req)

						Expect(resp.Code).To(Equal(http.StatusTeapot))

						var passedReq *http.Request
						Eventually(reqChan).Should(Receive(&passedReq))

						Expect(passedReq.Header.Get(routeservice.HeaderKeySignature)).ToNot(BeEmpty())
						Expect(passedReq.Header.Get(routeservice.HeaderKeyMetadata)).ToNot(BeEmpty())
						Expect(passedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(ContainSubstring("https://"))

						reqInfo, err := ContextRequestInfo(passedReq)
						Expect(err).ToNot(HaveOccurred())
						Expect(reqInfo.RouteServiceURL).ToNot(BeNil())

						Expect(reqInfo.RouteServiceURL.Host).To(Equal("route-service.com"))
						Expect(reqInfo.RouteServiceURL.Scheme).To(Equal("https"))
						Expect(reqInfo.ShouldRouteToInternalRouteService).To(BeFalse())
						Expect(nextCalled).To(BeTrue(), "Expected the next handler to be called.")
					})

				})

				Context("when the hairpin feature flag is enabled, with a large list of not matching allowlist entries", func() {
					BeforeEach(func() {
						hairpinning := true
						config = routeservice.NewRouteServiceConfig(
							logger.Logger, true, hairpinning, generateHugeAllowlist(1000000), 60*time.Second, crypto, nil, true, false, true, true, nil,
						)
					})

					It("does not add a flag to the request context", func() {

						start := time.Now()
						handler.ServeHTTP(resp, req)
						duration := time.Since(start)

						// This test does no warmup / cache and a single sample. Take with a grain of salt.
						fmt.Printf("Time taken to process request with large allowlist: %s", duration)

						Expect(resp.Code).To(Equal(http.StatusTeapot))

						var passedReq *http.Request
						Eventually(reqChan).Should(Receive(&passedReq))

						Expect(passedReq.Header.Get(routeservice.HeaderKeySignature)).ToNot(BeEmpty())
						Expect(passedReq.Header.Get(routeservice.HeaderKeyMetadata)).ToNot(BeEmpty())
						Expect(passedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(ContainSubstring("https://"))

						reqInfo, err := ContextRequestInfo(passedReq)
						Expect(err).ToNot(HaveOccurred())
						Expect(reqInfo.RouteServiceURL).ToNot(BeNil())

						Expect(reqInfo.RouteServiceURL.Host).To(Equal("route-service.com"))
						Expect(reqInfo.RouteServiceURL.Scheme).To(Equal("https"))
						Expect(reqInfo.ShouldRouteToInternalRouteService).To(BeFalse())
						Expect(nextCalled).To(BeTrue(), "Expected the next handler to be called.")
					})

				})
			})

			Context("when recommendHttps is set to false", func() {
				BeforeEach(func() {
					config = routeservice.NewRouteServiceConfig(
						logger.Logger, true, false, nil, 60*time.Second, crypto, nil, false, false, true, true, nil,
					)
				})
				It("sends the request to the route service with X-CF-Forwarded-Url using http scheme", func() {
					handler.ServeHTTP(resp, req)

					Expect(resp.Code).To(Equal(http.StatusTeapot))

					var passedReq *http.Request
					Eventually(reqChan).Should(Receive(&passedReq))

					Expect(passedReq.Header.Get(routeservice.HeaderKeySignature)).ToNot(BeEmpty())
					Expect(passedReq.Header.Get(routeservice.HeaderKeyMetadata)).ToNot(BeEmpty())
					Expect(passedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(ContainSubstring("http://"))

					reqInfo, err := ContextRequestInfo(passedReq)
					Expect(err).ToNot(HaveOccurred())
					Expect(reqInfo.RouteServiceURL).ToNot(BeNil())

					Expect(reqInfo.RouteServiceURL.Host).To(Equal("route-service.com"))
					Expect(reqInfo.RouteServiceURL.Scheme).To(Equal("https"))
					Expect(reqInfo.ShouldRouteToInternalRouteService).To(BeFalse())
					Expect(nextCalled).To(BeTrue(), "Expected the next handler to be called.")
				})
			})

			Context("when a request has a valid route service signature and metadata header", func() {
				BeforeEach(func() {
					reqArgs, err := config.CreateRequest("", forwardedUrl)
					Expect(err).ToNot(HaveOccurred())
					req.Header.Set(routeservice.HeaderKeySignature, reqArgs.Signature)
					req.Header.Set(routeservice.HeaderKeyMetadata, reqArgs.Metadata)
				})

				It("strips headers and sends the request to the backend instance", func() {
					handler.ServeHTTP(resp, req)

					Expect(resp.Code).To(Equal(http.StatusTeapot))

					var passedReq *http.Request
					Eventually(reqChan).Should(Receive(&passedReq))

					Expect(passedReq.Header.Get(routeservice.HeaderKeySignature)).To(BeEmpty())
					Expect(passedReq.Header.Get(routeservice.HeaderKeyMetadata)).To(BeEmpty())
					Expect(passedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(BeEmpty())
					reqInfo, err := ContextRequestInfo(passedReq)
					Expect(err).ToNot(HaveOccurred())
					Expect(reqInfo.RouteServiceURL).To(BeNil())
					Expect(nextCalled).To(BeTrue(), "Expected the next handler to be called.")
				})
			})

			Context("when a returned request has a signed forwarded URL", func() {
				BeforeEach(func() {
					routePool = route.NewPool(&route.PoolOpts{
						Logger:             logger.Logger,
						RetryAfterFailure:  time.Second,
						Host:               "app.example",
						ContextPath:        "/",
						MaxConnsPerBackend: 0,
					})
					routePool.Put(route.NewEndpoint(&route.EndpointOpts{RouteServiceUrl: "https://route-service.com"}))
					req.Host = "app.example"
					req.RequestURI = "/b"
					routeMap["app.example/a"] = routePool
					routeMap["app.example/b"] = routePool
					routeMap["app.example"] = routePool
				})

				It("rejects a different host that resolves to the same pool", func() {
					routePool = route.NewPool(&route.PoolOpts{
						Logger:             logger.Logger,
						RetryAfterFailure:  time.Second,
						Host:               "*.example.com",
						ContextPath:        "/",
						MaxConnsPerBackend: 0,
					})
					routePool.Put(route.NewEndpoint(&route.EndpointOpts{RouteServiceUrl: "https://route-service.com"}))
					routeMap["a.example.com"] = routePool
					req.Host = "b.example.com"
					req.RequestURI = "/"
					reqArgs, err := config.CreateRequest("https://route-service.com", "https://a.example.com/")
					Expect(err).NotTo(HaveOccurred())
					req.Header.Set(routeservice.HeaderKeySignature, reqArgs.Signature)
					req.Header.Set(routeservice.HeaderKeyMetadata, reqArgs.Metadata)

					handler.ServeHTTP(resp, req)

					Expect(resp.Code).To(Equal(http.StatusBadGateway))
					Expect(resp.Body.String()).To(ContainSubstring("forwarded URL host does not match request host"))
					Expect(nextCalled).To(BeFalse())
				})

				It("rejects signed forwarded URLs that are opaque or use an unsupported scheme", func() {
					for _, forwardedURL := range []string{"http:opaque", "ftp://app.example/"} {
						resp = httptest.NewRecorder()
						signature := &routeservice.SignatureContents{
							RequestedTime:   time.Now(),
							ForwardedUrl:    forwardedURL,
							RouteServiceUrl: "https://route-service.com",
						}
						signatureHeader, metadataHeader, err := routeservice.BuildSignatureAndMetadata(crypto, signature)
						Expect(err).NotTo(HaveOccurred())
						req.Header.Set(routeservice.HeaderKeySignature, signatureHeader)
						req.Header.Set(routeservice.HeaderKeyMetadata, metadataHeader)

						handler.ServeHTTP(resp, req)

						Expect(resp.Code).To(Equal(http.StatusBadGateway))
						Expect(resp.Body.String()).To(ContainSubstring("malformed forwarded URL"))
						Expect(nextCalled).To(BeFalse())
					}
				})

				It("rejects a signed forwarded URL with user information", func() {
					reqArgs, err := config.CreateRequest("https://route-service.com", "https://x:pass@app.example/")
					Expect(err).NotTo(HaveOccurred())
					req.Header.Set(routeservice.HeaderKeySignature, reqArgs.Signature)
					req.Header.Set(routeservice.HeaderKeyMetadata, reqArgs.Metadata)

					handler.ServeHTTP(resp, req)

					Expect(resp.Code).To(Equal(http.StatusBadGateway))
					Expect(resp.Body.String()).To(ContainSubstring("malformed forwarded URL"))
					Expect(nextCalled).To(BeFalse())
				})

				It("accepts a same-host path rewrite within the same pool", func() {
					reqArgs, err := config.CreateRequest("https://route-service.com", "https://app.example/a")
					Expect(err).NotTo(HaveOccurred())
					req.Header.Set(routeservice.HeaderKeySignature, reqArgs.Signature)
					req.Header.Set(routeservice.HeaderKeyMetadata, reqArgs.Metadata)

					handler.ServeHTTP(resp, req)

					Expect(resp.Code).To(Equal(http.StatusTeapot))
					Expect(nextCalled).To(BeTrue())
				})

				It("rejects a rewrite to a different path pool", func() {
					otherPool := route.NewPool(&route.PoolOpts{
						Logger:             logger.Logger,
						RetryAfterFailure:  time.Second,
						Host:               "app.example",
						ContextPath:        "/other",
						MaxConnsPerBackend: 0,
					})
					routeMap["app.example/other"] = otherPool
					reqArgs, err := config.CreateRequest("https://route-service.com", "https://app.example/other")
					Expect(err).NotTo(HaveOccurred())
					req.Header.Set(routeservice.HeaderKeySignature, reqArgs.Signature)
					req.Header.Set(routeservice.HeaderKeyMetadata, reqArgs.Metadata)

					handler.ServeHTTP(resp, req)

					Expect(resp.Code).To(Equal(http.StatusBadGateway))
					Expect(resp.Body.String()).To(ContainSubstring("does not match the original request URL"))
					Expect(nextCalled).To(BeFalse())
				})

				It("accepts host case and port differences", func() {
					req.Host = "app.example:443"
					routeMap["APP.example"] = routePool
					reqArgs, err := config.CreateRequest("https://route-service.com", "https://APP.example/")
					Expect(err).NotTo(HaveOccurred())
					req.Header.Set(routeservice.HeaderKeySignature, reqArgs.Signature)
					req.Header.Set(routeservice.HeaderKeyMetadata, reqArgs.Metadata)

					handler.ServeHTTP(resp, req)

					Expect(resp.Code).To(Equal(http.StatusTeapot))
					Expect(nextCalled).To(BeTrue())
				})
			})

			Context("when a request has a valid route service signature and metadata header and URL contains special chars", func() {
				BeforeEach(func() {
					reqArgs, err := config.CreateRequest("", "https://my_host.com/resource+9-9_9?query=%23%25")
					Expect(err).ToNot(HaveOccurred())
					req.Header.Set(routeservice.HeaderKeySignature, reqArgs.Signature)
					req.Header.Set(routeservice.HeaderKeyMetadata, reqArgs.Metadata)
				})

				It("should get response from backend instance without errors", func() {
					handler.ServeHTTP(resp, req)

					Expect(resp.Code).To(Equal(http.StatusTeapot))
				})
			})

			Context("when a request has a route service signature but no metadata header", func() {
				BeforeEach(func() {
					reqArgs, err := config.CreateRequest("", forwardedUrl)
					Expect(err).ToNot(HaveOccurred())
					req.Header.Set(routeservice.HeaderKeySignature, reqArgs.Signature)
				})

				It("returns a 502 bad gateway response", func() {
					handler.ServeHTTP(resp, req)

					Expect(resp.Code).To(Equal(http.StatusBadGateway))
					Expect(resp.Body.String()).To(ContainSubstring("Failed to validate Route Service Signature"))
					Eventually(logger).Should(gbytes.Say(`signature-validation-failed`))

					Expect(nextCalled).To(BeFalse())
				})
			})

			Context("when a request has an expired route service signature header", func() {
				BeforeEach(func() {
					signature := &routeservice.SignatureContents{
						RequestedTime:   time.Now().Add(-2 * time.Minute),
						ForwardedUrl:    forwardedUrl,
						RouteServiceUrl: "https://route-service.com",
					}

					signatureHeader, metadataHeader, err := routeservice.BuildSignatureAndMetadata(crypto, signature)
					Expect(err).ToNot(HaveOccurred())
					req.Header.Set(routeservice.HeaderKeySignature, signatureHeader)
					req.Header.Set(routeservice.HeaderKeyMetadata, metadataHeader)
				})

				It("returns a 504 gateway timeout response", func() {
					handler.ServeHTTP(resp, req)

					Expect(resp.Code).To(Equal(http.StatusGatewayTimeout))
					Expect(resp.Body.String()).To(ContainSubstring("Failed to validate Route Service Signature"))
					Eventually(logger).Should(gbytes.Say(`signature-validation-failed`))

					Expect(nextCalled).To(BeFalse())
				})
			})

			Context("when the signature's forwarded_url does not match the request", func() {
				BeforeEach(func() {
					reqArgs, err := config.CreateRequest("", "https://my_host.com/original_path")
					Expect(err).ToNot(HaveOccurred())
					req.Header.Set(routeservice.HeaderKeySignature, reqArgs.Signature)
					req.Header.Set(routeservice.HeaderKeyMetadata, reqArgs.Metadata)

					rsPool := route.NewPool(&route.PoolOpts{
						Logger:             logger.Logger,
						RetryAfterFailure:  2 * time.Minute,
						Host:               "my_host.com",
						ContextPath:        "/original_path",
						MaxConnsPerBackend: 0,
					})
					routeMap["my_host.com/original_path"] = rsPool
				})

				It("returns a 502 bad gateway response", func() {
					handler.ServeHTTP(resp, req)

					Expect(resp.Code).To(Equal(http.StatusBadGateway))
					Expect(resp.Body.String()).To(ContainSubstring("Failed to validate Route Service Signature"))
					Eventually(logger).Should(gbytes.Say(`signature-validation-failed`))

					Expect(nextCalled).To(BeFalse())
				})
			})

			Context("when a request header key does not match the crypto key in the config", func() {
				BeforeEach(func() {
					signature := &routeservice.SignatureContents{
						RequestedTime:   time.Now(),
						ForwardedUrl:    forwardedUrl,
						RouteServiceUrl: "https://route-service.com",
					}

					altCrypto, err := secure.NewAesGCM([]byte("QRSTUVWXYZ123456"))
					Expect(err).NotTo(HaveOccurred())

					signatureHeader, metadataHeader, err := routeservice.BuildSignatureAndMetadata(altCrypto, signature)
					Expect(err).ToNot(HaveOccurred())
					req.Header.Set(routeservice.HeaderKeySignature, signatureHeader)
					req.Header.Set(routeservice.HeaderKeyMetadata, metadataHeader)
				})

				It("returns a 502 bad gateway response", func() {
					handler.ServeHTTP(resp, req)

					Expect(resp.Code).To(Equal(http.StatusBadGateway))
					Expect(resp.Body.String()).To(ContainSubstring("Failed to validate Route Service Signature"))
					Eventually(logger).Should(gbytes.Say(`signature-validation-failed`))

					Expect(nextCalled).To(BeFalse())
				})
			})

			Context("with a previous crypto key in the config", func() {
				var cryptoPrev *secure.AesGCM
				BeforeEach(func() {
					var err error
					cryptoPrev, err = secure.NewAesGCM([]byte("QRSTUVWXYZ123456"))
					Expect(err).ToNot(HaveOccurred())
					config = routeservice.NewRouteServiceConfig(
						logger.Logger, true, false, nil, 60*time.Second, crypto, cryptoPrev, true, false, true, true, nil,
					)
				})

				Context("when a request header key matches the previous crypto key in the config", func() {
					BeforeEach(func() {
						signature := &routeservice.SignatureContents{
							RequestedTime:   time.Now(),
							ForwardedUrl:    forwardedUrl,
							RouteServiceUrl: "https://route-service.com",
						}

						signatureHeader, metadataHeader, err := routeservice.BuildSignatureAndMetadata(cryptoPrev, signature)
						Expect(err).ToNot(HaveOccurred())
						req.Header.Set(routeservice.HeaderKeySignature, signatureHeader)
						req.Header.Set(routeservice.HeaderKeyMetadata, metadataHeader)
					})

					It("sends the request to the backend instance", func() {
						handler.ServeHTTP(resp, req)

						var passedReq *http.Request
						Eventually(reqChan).Should(Receive(&passedReq))

						Expect(passedReq.Header.Get(routeservice.HeaderKeySignature)).To(BeEmpty())
						Expect(passedReq.Header.Get(routeservice.HeaderKeyMetadata)).To(BeEmpty())
						Expect(passedReq.Header.Get(routeservice.HeaderKeyForwardedURL)).To(BeEmpty())
						reqInfo, err := ContextRequestInfo(passedReq)
						Expect(err).ToNot(HaveOccurred())
						Expect(reqInfo.RouteServiceURL).To(BeNil())
						Expect(nextCalled).To(BeTrue(), "Expected the next handler to be called.")
					})
				})

				Context("when a request has an expired route service signature header", func() {
					BeforeEach(func() {
						signature := &routeservice.SignatureContents{
							RequestedTime:   time.Now().Add(-2 * time.Minute),
							ForwardedUrl:    forwardedUrl,
							RouteServiceUrl: "https://route-service.com",
						}

						signatureHeader, metadataHeader, err := routeservice.BuildSignatureAndMetadata(cryptoPrev, signature)
						Expect(err).ToNot(HaveOccurred())
						req.Header.Set(routeservice.HeaderKeySignature, signatureHeader)
						req.Header.Set(routeservice.HeaderKeyMetadata, metadataHeader)
					})

					It("returns a 504 gateway timeout response", func() {
						handler.ServeHTTP(resp, req)

						Expect(resp.Code).To(Equal(http.StatusGatewayTimeout))
						Expect(resp.Body.String()).To(ContainSubstring("Failed to validate Route Service Signature"))
						Eventually(logger).Should(gbytes.Say(`signature-validation-failed`))

						Expect(nextCalled).To(BeFalse())
					})
				})

				Context("when a request header key does not match the previous crypto key in the config", func() {
					BeforeEach(func() {
						signature := &routeservice.SignatureContents{
							RequestedTime:   time.Now(),
							ForwardedUrl:    forwardedUrl,
							RouteServiceUrl: "https://route-service.com",
						}

						altCrypto, err := secure.NewAesGCM([]byte("123456QRSTUVWXYZ"))
						Expect(err).NotTo(HaveOccurred())

						signatureHeader, metadataHeader, err := routeservice.BuildSignatureAndMetadata(altCrypto, signature)
						Expect(err).ToNot(HaveOccurred())
						req.Header.Set(routeservice.HeaderKeySignature, signatureHeader)
						req.Header.Set(routeservice.HeaderKeyMetadata, metadataHeader)
					})

					It("returns a 400 bad request response", func() {
						handler.ServeHTTP(resp, req)

						Expect(resp.Code).To(Equal(http.StatusBadGateway))
						Expect(resp.Body.String()).To(ContainSubstring("Failed to validate Route Service Signature"))

						Expect(nextCalled).To(BeFalse())
					})
				})
			})
		})

		Context("when a websocket request an app bound to a route service", func() {
			BeforeEach(func() {
				endpoint := route.NewEndpoint(&route.EndpointOpts{RouteServiceUrl: "https://goodrouteservice.com"})

				added := routePool.Put(endpoint)
				Expect(added).To(Equal(route.EndpointAdded))
				req.Header.Set("connection", "upgrade")
				req.Header.Set("upgrade", "websocket")

			})
			It("request contains correct route service URL", func() {
				handler.ServeHTTP(resp, req)

				var passedReq *http.Request
				Eventually(reqChan).Should(Receive(&passedReq))

				reqInfo, err := ContextRequestInfo(passedReq)
				Expect(err).ToNot(HaveOccurred())
				Expect(reqInfo.RouteServiceURL.Scheme).To(Equal("https"))
				Expect(reqInfo.RouteServiceURL.Host).To(Equal("goodrouteservice.com"))
				Expect(nextCalled).To(BeTrue(), "Expected the next handler to be called.")
			})
		})

		Context("when a bad route service url is used", func() {
			BeforeEach(func() {
				endpoint := route.NewEndpoint(&route.EndpointOpts{RouteServiceUrl: "https://bad%20service.com"})
				added := routePool.Put(endpoint)
				Expect(added).To(Equal(route.EndpointAdded))

			})
			It("returns a 500 internal server error response", func() {
				handler.ServeHTTP(resp, req)

				Expect(resp.Code).To(Equal(http.StatusInternalServerError))
				Expect(resp.Body.String()).To(ContainSubstring("Route service request failed."))

				Expect(nextCalled).To(BeFalse())
			})
		})

		Context("when hairpinning with an allowlist", func() {
			newHairpinChain := func(rsURL, routeKey, poolHost string, allowlist []string) *negroni.Negroni {
				endpoint := route.NewEndpoint(&route.EndpointOpts{RouteServiceUrl: rsURL})
				Expect(routePool.Put(endpoint)).To(Equal(route.EndpointAdded))

				if routeKey != "" {
					routeMap[routeKey] = route.NewPool(&route.PoolOpts{Logger: logger.Logger, Host: poolHost, ContextPath: "/"})
				}

				config = routeservice.NewRouteServiceConfig(logger.Logger, true, true, allowlist, 60*time.Second, crypto, nil, true, false, true, true, nil)

				h := negroni.New()
				h.Use(NewRequestInfo())
				h.UseFunc(testSetupHandler)
				h.Use(negroni.HandlerFunc(func(rw http.ResponseWriter, req *http.Request, next http.HandlerFunc) {
					next(rw, req)
				}))
				h.Use(NewRouteService(config, reg, logger.Logger, ew))
				h.UseHandlerFunc(nextHandler)

				return h
			}

			serveWith := func(rsURL, routeKey, poolHost string, allowlist []string, expected bool) {
				h := newHairpinChain(rsURL, routeKey, poolHost, allowlist)
				h.ServeHTTP(resp, req)

				var passedReq *http.Request
				Eventually(reqChan).Should(Receive(&passedReq))

				reqInfo, err := ContextRequestInfo(passedReq)
				Expect(err).NotTo(HaveOccurred())
				Expect(reqInfo.ShouldRouteToInternalRouteService).To(Equal(expected))
			}

			DescribeTable("hairpin-allowlist: checks the requested route service hostname",
				serveWith,
				Entry("does not match multi-level subdomain with wildcard", "https://a.b.svc.example.com", "a.b.svc.example.com", "*.svc.example.com", []string{"*.svc.example.com"}, false),
				Entry("matches exact hostname with path", "https://a.svc.example.com/rs-path", "a.svc.example.com/rs-path", "*.svc.example.com", []string{"a.svc.example.com"}, true),
				Entry("does not match leading dot in hostname", "https://.example.com", ".example.com", "*.example.com", []string{"*.example.com"}, false),
				Entry("does not match double leading dot in hostname", "https://..example.com", "..example.com", "*.example.com", []string{"*.example.com"}, false),
				Entry("does not match wildcard in hostname", "https://*.example.com", "*.example.com", "*.example.com", []string{"*.example.com"}, false),
				Entry("does not match double dot in hostname", "https://a..example.com", "a..example.com", "*.example.com", []string{"*.example.com"}, false),
				Entry("does not match trailing dot in hostname", "https://a.example.com.", "a.example.com.", "*.example.com", []string{"*.example.com"}, false),
				Entry("does not match IPv6 literal host", "https://[::1]:443", "[", "*.example.com", []string{"*.example.com"}, false),
			)

			It("hairpin-allowlist: logs a denied host at info once and at debug every time", func() {
				h := newHairpinChain("https://a.b.svc.example.com", "a.b.svc.example.com", "*.svc.example.com", []string{"*.svc.example.com"})

				h.ServeHTTP(resp, req)
				Eventually(reqChan).Should(Receive())

				resp2 := httptest.NewRecorder()
				req2 := httptest.NewRequest("GET", "http://my_host.com/resource+9-9_9", nil)
				h.ServeHTTP(resp2, req2)
				Eventually(reqChan).Should(Receive())

				denied := func(l zapcore.Level) []string {
					var matching []string
					for _, line := range logger.Lines(l) {
						if strings.Contains(line, "route-service-hairpin-denied") {
							matching = append(matching, line)
						}
					}
					return matching
				}

				deniedMatcher := And(
					ContainSubstring(`"route_service_host":"a.b.svc.example.com"`),
					ContainSubstring(`"reason":"hostname-not-in-allowlist"`),
				)

				infoLines := denied(zapcore.InfoLevel)
				Expect(infoLines).To(HaveLen(1))
				Expect(infoLines).To(HaveEach(deniedMatcher))

				debugLines := denied(zapcore.DebugLevel)
				Expect(debugLines).To(HaveLen(2))
				Expect(debugLines).To(HaveEach(deniedMatcher))
			})

			DescribeTable("hairpin-allowlist guard: unchanged hairpin decisions",
				serveWith,
				Entry("does not match different subdomain", "https://a.x.example.com", "a.x.example.com", "*.example.com", []string{"a.x.example.com"}, false),
				Entry("matches case-insensitively with userinfo and port", "https://user@A.Svc.Example.com:8443/rs-path", "A.Svc.Example.com/rs-path", "a.svc.example.com", []string{"a.svc.example.com"}, true),
				Entry("does not hairpin when route is not registered", "https://a.svc.example.com", "", "", []string{"a.svc.example.com"}, false),
				Entry("hairpins when allowlist is nil", "https://a.b.svc.example.com", "a.b.svc.example.com", "*.svc.example.com", nil, true),
				Entry("matches exact hostname", "https://a.svc.example.com", "a.svc.example.com", "a.svc.example.com", []string{"a.svc.example.com"}, true),
				Entry("matches when allowlist includes wildcard and exact host", "https://a.x.example.com", "a.x.example.com", "*.example.com", []string{"*.example.com", "a.x.example.com"}, true),
			)
		})
	})

	Context("when request info is not set on the request context", func() {
		var badHandler *negroni.Negroni
		BeforeEach(func() {
			badHandler = negroni.New()
			badHandler.Use(NewRouteService(config, reg, logger.Logger, ew))
			badHandler.UseHandlerFunc(nextHandler)
		})
		It("calls Panic on the logger", func() {
			defer func() {
				recover()
				Eventually(logger).Should(gbytes.Say(`request-info-err`))
				Expect(nextCalled).To(BeFalse())
			}()
			badHandler.ServeHTTP(resp, req)
		})
	})

	Context("when request info is set on the request context without route pool", func() {
		var badHandler *negroni.Negroni
		BeforeEach(func() {
			badHandler = negroni.New()
			badHandler.Use(NewRequestInfo())
			badHandler.Use(NewRouteService(config, reg, logger.Logger, ew))
			badHandler.UseHandlerFunc(nextHandler)
		})
		It("calls Panic on the logger", func() {
			defer func() {
				recover()
				Eventually(logger).Should(gbytes.Say(`failed-to-access-RoutePool`))
				Expect(nextCalled).To(BeFalse())
			}()
			badHandler.ServeHTTP(resp, req)
		})
	})
	Context("allowlist wildcards resolve correctly", func() {

		type testcase struct {
			name      string
			allowlist []string
			host      string
			matched   bool
			err       bool
		}
		tests := []testcase{
			{
				name:      "Test invalid wildcard leading with 2 subdomains",
				allowlist: []string{"*.*.wildcard-a.com"},
				host:      "first.authentication.wildcard-a.com",
				matched:   false,
				err:       true,
			},
			{
				name:      "Test wildcard in the wrong position",
				allowlist: []string{"first.*.wildcard-a.com"},
				host:      "first.authentication.wildcard-a.com",
				matched:   false,
				err:       true,
			},
			{
				name:      "Test wildcard domain without path",
				allowlist: []string{"*.wildcard-a.com"},
				host:      "authentication.wildcard-a.com",
				matched:   true,
				err:       false,
			},
			{
				name:      "Test wildcard domain is not a part of other domain",
				allowlist: []string{"*.wildcard-a.com"},
				host:      "cola-wildcard-a.com",
				matched:   false,
				err:       false,
			},
			{
				name:      "Test wildcard for subdomain",
				allowlist: []string{"*.authentication.wildcard-a.com"},
				host:      "first.authentication.wildcard-a.com",
				matched:   true,
				err:       false,
			},
			{
				name:      "Test wildcard for subdomain with CamelCase",
				allowlist: []string{"*.authentication.wildcard-a.com"},
				host:      "First.Authentication.Wildcard-A.com",
				matched:   true,
				err:       false,
			},
			{
				name:      "Test wildcard for subdomain with CamelCase in allowlist",
				allowlist: []string{"*.Authentication.Wildcard-A.com"},
				host:      "first.authentication.wildcard-a.com",
				matched:   true,
				err:       false,
			},
			{
				name:      "Test wildcard for wrong domain on subdomain",
				allowlist: []string{"*.authentication.wildcard-a.com"},
				host:      "first.authentication-wildcard-a.com",
				matched:   false,
				err:       false,
			},
			{
				name:      "Test fixed host name",
				allowlist: []string{"authentication.wildcard-a.com"},
				host:      "authentication.wildcard-a.com",
				matched:   true,
				err:       false,
			},
			{
				name:      "Test fixed host name with CamelCase",
				allowlist: []string{"authentication.wildcard-a.com"},
				host:      "Authentication.Wildcard-A.com",
				matched:   true,
				err:       false,
			},
			{
				name:      "Test fixed host name with CamelCase in allowlist",
				allowlist: []string{"Authentication.Wildcard-A.com"},
				host:      "authentication.wildcard-a.com",
				matched:   true,
				err:       false,
			},
			{
				name:      "Test wrong fixed host name",
				allowlist: []string{"authentication.wildcard-a.com"},
				host:      "first.authentication.wildcard-a.com",
				matched:   false,
				err:       false,
			},
			{
				name:      "Test injecting a regex",
				allowlist: []string{"(.|\\w)+"},
				host:      "first.authentication.wildcard-a.com",
				matched:   false,
				err:       true,
			},
			{
				name:      "Test bad allowlist entry",
				allowlist: []string{"*.ba d.cöm"},
				host:      "host.bad.com",
				matched:   false,
				err:       true,
			},
			{
				name:      "Subdomain wildcard should not match domain w/o segment covered by the wildcard",
				allowlist: []string{"*.authentication.wildcard-a.com"},
				host:      "authentication.wildcard-a.com",
				matched:   false,
				err:       false,
			},
		}

		It("for the pattern", func() {
			for _, testCase := range tests {
				By(testCase.name)

				config = routeservice.NewRouteServiceConfig(
					logger.Logger, true, true, testCase.allowlist, 60*time.Second, crypto, nil, true, false, true, true, nil,
				)

				if testCase.err {
					defer func() {
						recover()
						Eventually(logger).Should(gbytes.Say(`allowlist-entry-invalid`))
					}()
					NewRouteService(config, reg, logger.Logger, ew)
					continue
				}

				r := NewRouteService(config, reg, logger.Logger, ew).(*RouteService)

				matched := r.MatchAllowlistHostname(testCase.host)
				Expect(matched).To(Equal(testCase.matched))
			}
		})
	})
})

var _ = Describe("validateForwardedURL", func() {
	It("rejects opaque URLs, user info, unsupported schemes, and mismatched hosts", func() {
		for _, raw := range []string{
			"http:opaque",
			"https://user@app.example/path",
			"ftp://app.example/path",
			"https://other.example/path",
		} {
			Expect(validateForwardedURL(raw, "app.example:8080")).To(HaveOccurred(), raw)
		}
	})

	It("accepts host case and port differences", func() {
		Expect(validateForwardedURL("https://APP.EXAMPLE:443/path", "app.example:8080")).To(Succeed())
	})
})

var _ = Describe("route service signature classification", func() {
	var (
		crypto       *secure.AesGCM
		logger       *test_util.TestLogger
		pool         *route.EndpointPool
		config       *routeservice.RouteServiceConfig
		routeService *RouteService
	)

	BeforeEach(func() {
		logger = test_util.NewTestLogger("test")
		var err error
		crypto, err = secure.NewAesGCM([]byte("ABCDEFGHIJKLMNOP"))
		Expect(err).NotTo(HaveOccurred())
		pool = route.NewPool(&route.PoolOpts{Logger: logger.Logger, Host: "rs.example", ContextPath: "/", MaxConnsPerBackend: 0})
		config = routeservice.NewRouteServiceConfig(logger.Logger, true, false, nil, time.Minute, crypto, nil, true, false, true, true, nil)
		routeService = NewRouteService(config, &fakeRegistry.FakeRegistry{}, logger.Logger, errorwriter.NewPlaintextErrorWriter()).(*RouteService)
	})

	requestWithPool := func(host, target string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "http://example"+target, nil)
		req.Host = host
		req = req.WithContext(context.WithValue(req.Context(), RequestInfoCtxKey, &RequestInfo{RoutePool: pool}))
		return req
	}

	setSignature := func(req *http.Request, signedURL string, requested time.Time) {
		contents := &routeservice.SignatureContents{RequestedTime: requested, RouteServiceUrl: signedURL}
		signature, metadata, err := routeservice.BuildSignatureAndMetadata(crypto, contents)
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set(routeservice.HeaderKeySignature, signature)
		req.Header.Set(routeservice.HeaderKeyMetadata, metadata)
	}

	setExplicitLegacySignatureWithCrypto := func(req *http.Request, signingCrypto *secure.AesGCM) {
		contents := []byte(`{"forwarded_url":"https://rs.example/service","requested_time":"` + time.Now().Format(time.RFC3339Nano) + `","route_service_url":""}`)
		ciphertext, nonce, err := signingCrypto.Encrypt(contents)
		Expect(err).NotTo(HaveOccurred())
		metadata, err := json.Marshal(routeservice.Metadata{Nonce: nonce})
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set(routeservice.HeaderKeySignature, base64.URLEncoding.EncodeToString(ciphertext))
		req.Header.Set(routeservice.HeaderKeyMetadata, base64.URLEncoding.EncodeToString(metadata))
	}

	setExplicitLegacySignature := func(req *http.Request) {
		setExplicitLegacySignatureWithCrypto(req, crypto)
	}

	DescribeTable("matches signed destinations exactly",
		func(signedURL, host, target string, expected bool) {
			req := requestWithPool(host, target)
			setSignature(req, signedURL, time.Now())
			Expect(routeService.IsRouteServiceTraffic(req)).To(Equal(expected))
		},
		Entry("default HTTPS port", "https://RS.example/service", "rs.example:443", "/service", true),
		Entry("missing request port uses signed scheme default", "https://RS.example:8443/service", "rs.example", "/service", false),
		Entry("different port", "https://RS.example/service", "rs.example:80", "/service", false),
		Entry("trailing host dot", "https://rs.example/service", "rs.example.", "/service", false),
		Entry("path case", "https://rs.example/service", "rs.example", "/Service", false),
		Entry("trailing path slash", "https://rs.example/service", "rs.example", "/service/", false),
		Entry("escaped path case", "https://rs.example/a%2Fb", "rs.example", "/a%2fb", false),
		Entry("IPv6 authority", "https://[2001:db8::1]:8443/x", "[2001:db8::1]:8443", "/x", true),
		Entry("query ignored", "https://rs.example/service", "rs.example", "/service?q=1", true),
	)

	It("accepts legacy signatures only when unbound signatures are enabled", func() {
		for _, allow := range []bool{true, false} {
			config = routeservice.NewRouteServiceConfig(logger.Logger, true, false, nil, time.Minute, crypto, nil, true, false, allow, true, nil)
			routeService = NewRouteService(config, &fakeRegistry.FakeRegistry{}, logger.Logger, errorwriter.NewPlaintextErrorWriter()).(*RouteService)
			req := requestWithPool("rs.example", "/service")
			setSignature(req, "", time.Now())
			Expect(routeService.IsRouteServiceTraffic(req)).To(Equal(allow))
		}
	})

	It("does not require the unsigned forwarded URL when a bound signature matches", func() {
		req := requestWithPool("rs.example", "/service")
		setSignature(req, "https://rs.example/service", time.Now())
		Expect(req.Header.Get(routeservice.HeaderKeyForwardedURL)).To(BeEmpty())
		Expect(routeService.IsRouteServiceTraffic(req)).To(BeTrue())
	})

	It("checks bound destinations when validating signatures with the previous key", func() {
		previousCrypto, err := secure.NewAesGCM([]byte("QRSTUVWXYZ123456"))
		Expect(err).NotTo(HaveOccurred())
		config = routeservice.NewRouteServiceConfig(logger.Logger, true, false, nil, time.Minute, crypto, previousCrypto, true, false, true, true, nil)
		routeService = NewRouteService(config, &fakeRegistry.FakeRegistry{}, logger.Logger, errorwriter.NewPlaintextErrorWriter()).(*RouteService)

		for _, testCase := range []struct {
			signedURL string
			want      bool
		}{
			{signedURL: "https://rs.example/service", want: true},
			{signedURL: "https://other.example/service", want: false},
		} {
			req := requestWithPool("rs.example", "/service")
			signature, metadata, err := routeservice.BuildSignatureAndMetadata(previousCrypto, &routeservice.SignatureContents{
				RequestedTime: time.Now(), RouteServiceUrl: testCase.signedURL,
			})
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set(routeservice.HeaderKeySignature, signature)
			req.Header.Set(routeservice.HeaderKeyMetadata, metadata)
			Expect(routeService.IsRouteServiceTraffic(req)).To(Equal(testCase.want), testCase.signedURL)
		}
	})

	It("classifies signatures minted by independent configurations with shared and rotating keys", func() {
		classifyMinted := func(issuerCrypto, receiverCrypto, previousCrypto *secure.AesGCM, allow bool, signedURL string) bool {
			issuer := routeservice.NewRouteServiceConfig(logger.Logger, true, false, nil, time.Minute, issuerCrypto, nil, true, false, allow, true, nil)
			receiver := routeservice.NewRouteServiceConfig(logger.Logger, true, false, nil, time.Minute, receiverCrypto, previousCrypto, true, false, allow, true, nil)
			args, err := issuer.CreateRequest(signedURL, "https://rs.example/service")
			Expect(err).NotTo(HaveOccurred())
			req := requestWithPool("rs.example", "/service")
			req.Header.Set(routeservice.HeaderKeySignature, args.Signature)
			req.Header.Set(routeservice.HeaderKeyMetadata, args.Metadata)
			validator := NewRouteService(receiver, &fakeRegistry.FakeRegistry{}, logger.Logger, errorwriter.NewPlaintextErrorWriter()).(*RouteService)
			return validator.IsRouteServiceTraffic(req)
		}

		sharedIssuerKey, err := secure.NewAesGCM([]byte("ABCDEFGHIJKLMNOP"))
		Expect(err).NotTo(HaveOccurred())
		sharedReceiverKey, err := secure.NewAesGCM([]byte("ABCDEFGHIJKLMNOP"))
		Expect(err).NotTo(HaveOccurred())
		Expect(classifyMinted(sharedIssuerKey, sharedReceiverKey, nil, false, "https://rs.example/service")).To(BeTrue())
		Expect(classifyMinted(sharedIssuerKey, sharedReceiverKey, nil, false, "https://other.example/service")).To(BeFalse())

		oldIssuerKey, err := secure.NewAesGCM([]byte("QRSTUVWXYZ123456"))
		Expect(err).NotTo(HaveOccurred())
		newReceiverKey, err := secure.NewAesGCM([]byte("ABCDEFGHIJKLMNOP"))
		Expect(err).NotTo(HaveOccurred())
		for _, allow := range []bool{true, false} {
			Expect(classifyMinted(oldIssuerKey, newReceiverKey, oldIssuerKey, allow, "https://rs.example/service")).To(BeTrue())
			Expect(classifyMinted(oldIssuerKey, newReceiverKey, oldIssuerKey, allow, "https://other.example/service")).To(BeFalse())

			receiver := routeservice.NewRouteServiceConfig(logger.Logger, true, false, nil, time.Minute, newReceiverKey, oldIssuerKey, true, false, allow, true, nil)
			req := requestWithPool("rs.example", "/service")
			setExplicitLegacySignatureWithCrypto(req, oldIssuerKey)
			validator := NewRouteService(receiver, &fakeRegistry.FakeRegistry{}, logger.Logger, errorwriter.NewPlaintextErrorWriter()).(*RouteService)
			Expect(validator.IsRouteServiceTraffic(req)).To(Equal(allow))
		}
	})

	It("checks the return leg destination with the previous key", func() {
		pool.Put(route.NewEndpoint(&route.EndpointOpts{RouteServiceUrl: "https://route-service.example"}))
		previousCrypto, err := secure.NewAesGCM([]byte("QRSTUVWXYZ123456"))
		Expect(err).NotTo(HaveOccurred())
		registry := &fakeRegistry.FakeRegistry{}
		registry.LookupStub = func(route.Uri) *route.EndpointPool { return pool }
		config = routeservice.NewRouteServiceConfig(logger.Logger, true, false, nil, time.Minute, crypto, previousCrypto, true, false, false, true, nil)
		routeService = NewRouteService(config, registry, logger.Logger, errorwriter.NewPlaintextErrorWriter()).(*RouteService)

		for _, testCase := range []struct {
			signedURL string
			want      bool
		}{
			{signedURL: "https://route-service.example", want: true},
			{signedURL: "https://other-route-service.example", want: false},
		} {
			req := requestWithPool("rs.example", "/service")
			req.RequestURI = "/service"
			signature, metadata, err := routeservice.BuildSignatureAndMetadata(previousCrypto, &routeservice.SignatureContents{
				ForwardedUrl: "https://rs.example/service", RequestedTime: time.Now(), RouteServiceUrl: testCase.signedURL,
			})
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set(routeservice.HeaderKeySignature, signature)
			req.Header.Set(routeservice.HeaderKeyMetadata, metadata)
			arrived, err := routeService.ArrivedViaRouteService(req, logger.Logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(arrived).To(Equal(testCase.want), testCase.signedURL)
		}
	})

	It("rejects malformed destinations and requests without signatures", func() {
		for _, signedURL := range []string{"not a url", "ftp://x/y", "https://user@x/y"} {
			req := requestWithPool("rs.example", "/service")
			setSignature(req, signedURL, time.Now())
			Expect(routeService.IsRouteServiceTraffic(req)).To(BeFalse(), signedURL)
		}
		req := requestWithPool("rs.example", "/service")
		Expect(routeService.IsRouteServiceTraffic(req)).To(BeFalse())
	})

	It("applies classifier outcomes in strict mode", func() {
		cases := []struct {
			name       string
			signedURL  string
			allow      bool
			requested  time.Time
			statusCode int
			next       bool
			errorCode  string
			reason     string
		}{
			{name: "legacy field absent allowed", allow: true, requested: time.Now(), statusCode: http.StatusTeapot, next: true},
			{name: "legacy empty field allowed", signedURL: "", allow: true, requested: time.Now(), statusCode: http.StatusTeapot, next: true},
			{name: "legacy field absent disabled", allow: false, requested: time.Now(), statusCode: http.StatusBadRequest, errorCode: "invalid_route_service_signature", reason: "unbound"},
			{name: "legacy empty field disabled", signedURL: "", allow: false, requested: time.Now(), statusCode: http.StatusBadRequest, errorCode: "invalid_route_service_signature", reason: "unbound"},
			{name: "bound match", signedURL: "https://rs.example/service", requested: time.Now(), statusCode: http.StatusTeapot, next: true},
			{name: "bound mismatch", signedURL: "https://other.example/service", requested: time.Now(), statusCode: http.StatusBadRequest, errorCode: "invalid_route_service_signature", reason: "bound_mismatch"},
			{name: "corrupt signature", requested: time.Now(), statusCode: http.StatusBadRequest, errorCode: "invalid_route_service_signature", reason: "decrypt_failed"},
			{name: "expired", signedURL: "https://rs.example/service", requested: time.Now().Add(-2 * time.Minute), statusCode: http.StatusGatewayTimeout, errorCode: "expired_route_service_signature"},
		}
		for _, testCase := range cases {
			previousLines := len(logger.TestSink.Lines())
			config = routeservice.NewRouteServiceConfig(logger.Logger, true, false, nil, time.Minute, crypto, nil, true, true, testCase.allow, true, nil)
			routeService = NewRouteService(config, &fakeRegistry.FakeRegistry{}, logger.Logger, errorwriter.NewPlaintextErrorWriter()).(*RouteService)
			req := requestWithPool("rs.example", "/service")
			if testCase.name == "legacy empty field allowed" || testCase.name == "legacy empty field disabled" {
				setExplicitLegacySignature(req)
			} else {
				setSignature(req, testCase.signedURL, testCase.requested)
			}
			if testCase.name == "corrupt signature" {
				req.Header.Set(routeservice.HeaderKeySignature, "garbage")
			}
			response := httptest.NewRecorder()
			nextWasCalled := false
			routeService.ServeHTTP(response, req, func(rw http.ResponseWriter, _ *http.Request) {
				nextWasCalled = true
				rw.WriteHeader(http.StatusTeapot)
			})
			Expect(response.Code).To(Equal(testCase.statusCode), testCase.name)
			Expect(nextWasCalled).To(Equal(testCase.next), testCase.name)
			if testCase.errorCode != "" {
				Expect(response.Header().Get("X-Cf-RouterError")).To(Equal(testCase.errorCode), testCase.name)
			}
			if testCase.statusCode == http.StatusBadRequest {
				Expect(response.Body.String()).To(ContainSubstring("invalid route service signature detected"), testCase.name)
				Expect(logger.TestSink.Lines()[previousLines:]).To(ContainElement(ContainSubstring(`"reason":"`+testCase.reason+`"`)), testCase.name)
			}
			if testCase.name == "expired" {
				Expect(logger.TestSink.Lines()[previousLines:]).NotTo(ContainElement(ContainSubstring(`"reason"`)))
			}
		}
	})

	It("binds return leg trust to the current route service destination", func() {
		pool.Put(route.NewEndpoint(&route.EndpointOpts{RouteServiceUrl: "https://route-service.example"}))
		registry := &fakeRegistry.FakeRegistry{}
		registry.LookupStub = func(route.Uri) *route.EndpointPool { return pool }
		for _, testCase := range []struct {
			name      string
			signedURL string
			allow     bool
			want      bool
		}{
			{name: "matching bound destination is trusted while legacy is disabled", signedURL: "https://route-service.example", want: true},
			{name: "legacy destination is trusted while enabled", allow: true, want: true},
			{name: "legacy destination is not trusted while disabled", want: false},
			{name: "different destination is not trusted", signedURL: "https://other-route-service.example", allow: true, want: false},
			{name: "malformed destination is not trusted", signedURL: "not a url", allow: true, want: false},
		} {
			config = routeservice.NewRouteServiceConfig(logger.Logger, true, false, nil, time.Minute, crypto, nil, true, false, testCase.allow, true, nil)
			routeService = NewRouteService(config, registry, logger.Logger, errorwriter.NewPlaintextErrorWriter()).(*RouteService)
			req := requestWithPool("rs.example", "/service")
			req.Host = "rs.example"
			req.RequestURI = "/service"
			if testCase.signedURL == "" {
				setExplicitLegacySignature(req)
			} else {
				contents := &routeservice.SignatureContents{ForwardedUrl: "https://rs.example/service", RequestedTime: time.Now(), RouteServiceUrl: testCase.signedURL}
				signature, metadata, err := routeservice.BuildSignatureAndMetadata(crypto, contents)
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set(routeservice.HeaderKeySignature, signature)
				req.Header.Set(routeservice.HeaderKeyMetadata, metadata)
			}
			arrived, err := routeService.ArrivedViaRouteService(req, logger.Logger)
			Expect(err).NotTo(HaveOccurred(), testCase.name)
			Expect(arrived).To(Equal(testCase.want), testCase.name)
			Expect(routeService.IsRouteServiceTraffic(req)).To(Equal(testCase.want), testCase.name)
		}
	})

	It("does not trust a return leg whose forwarded URL resolves to another pool", func() {
		pool.Put(route.NewEndpoint(&route.EndpointOpts{RouteServiceUrl: "https://route-service.example"}))
		otherPool := route.NewPool(&route.PoolOpts{Logger: logger.Logger, Host: "rs.example", ContextPath: "/b", MaxConnsPerBackend: 0})
		registry := &fakeRegistry.FakeRegistry{}
		registry.LookupStub = func(uri route.Uri) *route.EndpointPool {
			if uri == "rs.example/b/service" {
				return otherPool
			}
			return pool
		}
		routeService = NewRouteService(config, registry, logger.Logger, errorwriter.NewPlaintextErrorWriter()).(*RouteService)
		req := requestWithPool("rs.example", "/service")
		contents := &routeservice.SignatureContents{ForwardedUrl: "https://rs.example/b/service", RequestedTime: time.Now(), RouteServiceUrl: "https://route-service.example"}
		signature, metadata, err := routeservice.BuildSignatureAndMetadata(crypto, contents)
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set(routeservice.HeaderKeySignature, signature)
		req.Header.Set(routeservice.HeaderKeyMetadata, metadata)

		arrived, err := routeService.ArrivedViaRouteService(req, logger.Logger)
		Expect(err).To(HaveOccurred())
		Expect(arrived).To(BeFalse())
		Expect(routeService.IsRouteServiceTraffic(req)).To(BeFalse())
	})

	It("does not trust a return leg when the current route service URL cannot be parsed", func() {
		pool.Put(route.NewEndpoint(&route.EndpointOpts{RouteServiceUrl: "https://route-service.example/%zz"}))
		registry := &fakeRegistry.FakeRegistry{}
		registry.LookupStub = func(route.Uri) *route.EndpointPool { return pool }
		routeService = NewRouteService(config, registry, logger.Logger, errorwriter.NewPlaintextErrorWriter()).(*RouteService)
		req := requestWithPool("rs.example", "/service")
		req.RequestURI = "/service"
		signature, metadata, err := routeservice.BuildSignatureAndMetadata(crypto, &routeservice.SignatureContents{
			ForwardedUrl: "https://rs.example/service", RequestedTime: time.Now(), RouteServiceUrl: "https://route-service.example/",
		})
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set(routeservice.HeaderKeySignature, signature)
		req.Header.Set(routeservice.HeaderKeyMetadata, metadata)

		arrived, err := routeService.ArrivedViaRouteService(req, logger.Logger)
		Expect(err).NotTo(HaveOccurred())
		Expect(arrived).To(BeFalse())
	})
})

func generateHugeAllowlist(size int) []string {
	buffer := make([]string, 0, size)

	for i := 0; i < size; i++ {
		buffer = append(buffer, fmt.Sprintf("*.subdomain-%d.example.com", i))
	}

	return buffer
}
