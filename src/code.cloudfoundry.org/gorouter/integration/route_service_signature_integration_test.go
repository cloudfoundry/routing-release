package integration

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/net/websocket"

	"code.cloudfoundry.org/gorouter/common/secure"
	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/mbus"
	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/routeservice"
)

var _ = Describe("outbound route service signature classification", func() {
	var testState *testState
	var app *httptest.Server
	const appHostname = "signature-app.some.domain"

	BeforeEach(func() {
		testState = NewTestState()
		testState.cfg.ForwardedClientCert = config.FORWARD
		testState.cfg.SanitizeForwardedProto = true
		app = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Received-XFCC", r.Header.Get("X-Forwarded-Client-Cert"))
			w.Header().Set("X-Received-XFP", r.Header.Get("X-Forwarded-Proto"))
			w.WriteHeader(http.StatusOK)
		}))
		testState.StartGorouterOrFail()
		testState.register(app, appHostname)
	})

	AfterEach(func() {
		if testState != nil {
			testState.StopAndCleanup()
		}
		if app != nil {
			app.Close()
		}
	})

	signedHeadersAt := func(forwardedURL, routeServiceURL string, requested time.Time) http.Header {
		key := secure.NewPbkdf2([]byte(testState.cfg.RouteServiceSecret), 16)
		crypto, err := secure.NewAesGCM(key)
		Expect(err).NotTo(HaveOccurred())
		signature, metadata, err := routeservice.BuildSignatureAndMetadata(crypto, &routeservice.SignatureContents{
			ForwardedUrl: forwardedURL, RequestedTime: requested, RouteServiceUrl: routeServiceURL,
		})
		Expect(err).NotTo(HaveOccurred())
		headers := make(http.Header)
		headers.Set(routeservice.HeaderKeyForwardedURL, forwardedURL)
		headers.Set(routeservice.HeaderKeySignature, signature)
		headers.Set(routeservice.HeaderKeyMetadata, metadata)
		return headers
	}
	signedHeaders := func(forwardedURL, routeServiceURL string) http.Header {
		return signedHeadersAt(forwardedURL, routeServiceURL, time.Now())
	}
	requestWithSignatureFor := func(forwardedURL, routeServiceURL string) *http.Response {
		req := testState.newGetRequest(forwardedURL)
		req.Header = signedHeaders(forwardedURL, routeServiceURL)
		req.Header.Set("X-Forwarded-Client-Cert", "caller-xfcc")
		req.Header.Set("X-Forwarded-Proto", "caller-xfp")
		res, err := testState.client.Do(req)
		Expect(err).NotTo(HaveOccurred())
		return res
	}
	requestWithSignature := func(routeServiceURL string) *http.Response {
		return requestWithSignatureFor(fmt.Sprintf("https://%s/", appHostname), routeServiceURL)
	}

	assertHairpinHeaders := func(hairpin bool) {
		testState.StopAndCleanup()
		testState.cfg.RouteServicesHairpinning = hairpin
		testState.cfg.ForwardedClientCert = config.SANITIZE_SET
		testState.cfg.EnableSSL = true
		testState.cfg.ClientCertificateValidationString = "request"
		testState.cfg.RouteServiceRecommendHttps = true
		testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = testState.trustedClientTLSConfig.Certificates
		routeServiceClientTLSConfig := testState.routeServiceClient.Transport.(*http.Transport).TLSClientConfig.Clone()
		routeServiceClientTLSConfig.Certificates = testState.trustedRouteServiceClientTLSConfig.Certificates
		testState.routeServiceClient.Transport.(*http.Transport).TLSClientConfig = routeServiceClientTLSConfig
		testState.StartGorouterOrFail()
		routeServiceHeaders := make(chan http.Header, 1)
		routeService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			routeServiceHeaders <- r.Header.Clone()
			forwardedURL := r.Header.Get(routeservice.HeaderKeyForwardedURL)
			request := testState.newGetRequest(forwardedURL)
			for _, header := range []string{
				routeservice.HeaderKeyForwardedURL,
				routeservice.HeaderKeySignature,
				routeservice.HeaderKeyMetadata,
				"X-Forwarded-Client-Cert",
				"X-Forwarded-Proto",
			} {
				request.Header[header] = r.Header.Values(header)
			}
			request.Header.Set("X-Forwarded-Proto", "route-service-xfp")
			response, err := testState.routeServiceClient.Do(request)
			Expect(err).NotTo(HaveOccurred())
			defer response.Body.Close()
			for _, header := range []string{"X-Received-XFCC", "X-Received-XFP"} {
				w.Header().Set(header, response.Header.Get(header))
			}
			w.WriteHeader(response.StatusCode)
		}))
		defer routeService.Close()
		testState.registerWithInternalRouteService(app, routeService, appHostname, testState.cfg.SSLPort)

		request := testState.newGetRequest("https://" + appHostname)
		request.Header.Set("X-Forwarded-Proto", "caller-xfp")
		response, err := testState.client.Do(request)
		Expect(err).NotTo(HaveOccurred())
		defer response.Body.Close()
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		var headers http.Header
		Eventually(routeServiceHeaders).Should(Receive(&headers))
		clientXFCC := sanitize(testState.trustedClientTLSConfig.Certificates[0])
		Expect(headers.Get("X-Forwarded-Client-Cert")).To(Equal(clientXFCC))
		Expect(headers.Get("X-Forwarded-Proto")).To(Equal("https"))
		Expect(response.Header.Get("X-Received-XFCC")).To(Equal(clientXFCC))
		Expect(response.Header.Get("X-Received-XFP")).To(Equal("route-service-xfp"))
	}

	It("sanitizes headers from a signature bound to another route service, or rejects in strict mode", func() {
		res := requestWithSignature("https://route-service-a.some.domain/path")
		Expect(res.StatusCode).To(Equal(http.StatusOK))
		Expect(res.Header.Get("X-Received-XFCC")).To(BeEmpty())
		Expect(res.Header.Get("X-Received-XFP")).To(Equal("https"))
		res.Body.Close()

		testState.StopAndCleanup()
		testState.cfg.RouteServiceConfig.StrictSignatureValidation = true
		testState.StartGorouterOrFail()
		testState.register(app, appHostname)
		res = requestWithSignature("https://route-service-a.some.domain/path")
		defer res.Body.Close()
		Expect(res.StatusCode).To(Equal(http.StatusBadRequest))
	})

	It("continues to the app with sanitized headers for an expired signature on an unbound route", func() {
		forwardedURL := "https://" + appHostname + "/"
		request := testState.newGetRequest(forwardedURL)
		request.Header = signedHeadersAt(forwardedURL, forwardedURL, time.Now().Add(-2*time.Minute))
		request.Header.Set("X-Forwarded-Client-Cert", "caller-xfcc")
		request.Header.Set("X-Forwarded-Proto", "caller-xfp")

		response, err := testState.client.Do(request)
		Expect(err).NotTo(HaveOccurred())
		defer response.Body.Close()
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		Expect(response.Header.Get("X-Received-XFCC")).To(BeEmpty())
		Expect(response.Header.Get("X-Received-XFP")).To(Equal("https"))
	})

	It("sends a return leg signed for route service A through the rebound route service B", func() {
		testState.StopAndCleanup()
		testState.cfg.SanitizeForwardedProto = true
		testState.cfg.ForwardedClientCert = config.SANITIZE_SET
		testState.StartGorouterOrFail()
		appHeaders := make(chan http.Header, 1)
		app.Close()
		app = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			appHeaders <- r.Header.Clone()
			w.Header().Set("X-Received-XFCC", r.Header.Get("X-Forwarded-Client-Cert"))
			w.Header().Set("X-Received-XFP", r.Header.Get("X-Forwarded-Proto"))
			w.WriteHeader(http.StatusOK)
		}))
		var routeURLFor = func(server *httptest.Server) string {
			_, port := hostnameAndPort(server.Listener.Addr().String())
			return fmt.Sprintf("https://internal-route-service-%d.localhost.routing.cf-app.com:%d", port, testState.cfg.SSLPort)
		}
		registerRouteService := func(server *httptest.Server, id string) mbus.RegistryMessage {
			_, port := hostnameAndPort(server.Listener.Addr().String())
			internalHost := fmt.Sprintf("internal-route-service-%d.localhost.routing.cf-app.com", port)
			testState.register(server, internalHost)
			_, appPort := hostnameAndPort(app.Listener.Addr().String())
			message := mbus.RegistryMessage{
				Host: "127.0.0.1", Port: appPort, Uris: []route.Uri{route.Uri(appHostname)},
				RouteServiceURL: routeURLFor(server), PrivateInstanceID: id, StaleThresholdInSeconds: 10,
			}
			testState.registerAndWait(message)
			return message
		}

		signedHeadersFromA := make(chan http.Header, 1)
		routeServiceA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			signedHeadersFromA <- r.Header.Clone()
			w.WriteHeader(http.StatusOK)
		}))
		defer routeServiceA.Close()
		routeServiceBHeaders := make(chan http.Header, 1)
		routeServiceB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			routeServiceBHeaders <- r.Header.Clone()
			forwardedURL := r.Header.Get(routeservice.HeaderKeyForwardedURL)
			request := testState.newGetRequest(forwardedURL)
			for _, header := range []string{routeservice.HeaderKeyForwardedURL, routeservice.HeaderKeySignature, routeservice.HeaderKeyMetadata, "X-Forwarded-Client-Cert", "X-Forwarded-Proto"} {
				request.Header[header] = r.Header.Values(header)
			}
			response, err := testState.routeServiceClient.Do(request)
			Expect(err).NotTo(HaveOccurred())
			defer response.Body.Close()
			for _, header := range []string{"X-Received-XFCC", "X-Received-XFP"} {
				w.Header().Set(header, response.Header.Get(header))
			}
			w.WriteHeader(response.StatusCode)
		}))
		defer routeServiceB.Close()
		routeARegistration := registerRouteService(routeServiceA, "signature-route-service-a")

		request := testState.newGetRequest("https://" + appHostname)
		request.Header.Set("X-Forwarded-Client-Cert", "caller-xfcc")
		request.Header.Set("X-Forwarded-Proto", "caller-xfp")
		response, err := testState.client.Do(request)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		response.Body.Close()
		var signedHeaders http.Header
		Eventually(signedHeadersFromA).Should(Receive(&signedHeaders))
		Expect(signedHeaders.Get(routeservice.HeaderKeySignature)).NotTo(BeEmpty())

		unregisterBytes, err := json.Marshal(routeARegistration)
		Expect(err).NotTo(HaveOccurred())
		Expect(testState.mbusClient.Publish("router.unregister", unregisterBytes)).To(Succeed())
		routeBRegistration := registerRouteService(routeServiceB, "signature-route-service-b")
		routesURL := fmt.Sprintf("http://%s:%s@127.0.0.1:%d/routes", testState.cfg.Status.User, testState.cfg.Status.Pass, testState.cfg.Status.Routes.Port)
		Eventually(func() ([]string, error) {
			response, err := http.Get(routesURL)
			if err != nil {
				return nil, err
			}
			defer response.Body.Close()
			var routes map[string][]struct {
				RouteServiceURL string `json:"route_service_url"`
			}
			if err := json.NewDecoder(response.Body).Decode(&routes); err != nil {
				return nil, err
			}
			var urls []string
			for _, endpoint := range routes[appHostname] {
				urls = append(urls, endpoint.RouteServiceURL)
			}
			return urls, nil
		}).Should(ConsistOf(routeBRegistration.RouteServiceURL))

		forwardedURL := "https://" + appHostname + "/"
		returnRequest := testState.newGetRequest(forwardedURL)
		returnRequest.Header = signedHeaders.Clone()
		returnRequest.Header.Set("X-Forwarded-Client-Cert", "caller-xfcc")
		returnRequest.Header.Set("X-Forwarded-Proto", "caller-xfp")
		response, err = testState.routeServiceClient.Do(returnRequest)
		Expect(err).NotTo(HaveOccurred())
		defer response.Body.Close()
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		Expect(response.Header.Get("X-Received-XFCC")).NotTo(Equal("caller-xfcc"))
		Expect(response.Header.Get("X-Received-XFP")).To(Equal("https"))
		var routeServiceBRequestHeaders http.Header
		Eventually(routeServiceBHeaders).Should(Receive(&routeServiceBRequestHeaders))
		Expect(routeServiceBRequestHeaders.Get(routeservice.HeaderKeySignature)).NotTo(Equal(signedHeaders.Get(routeservice.HeaderKeySignature)))
		Expect(routeServiceBRequestHeaders.Get("X-Forwarded-Proto")).To(Equal("https"))
		var headers http.Header
		Eventually(appHeaders).Should(Receive(&headers))
		Expect(headers.Get("X-Forwarded-Client-Cert")).NotTo(Equal("caller-xfcc"))
		Expect(headers.Get("X-Forwarded-Proto")).NotTo(Equal("caller-xfp"))
	})

	It("preserves headers through an on-platform route service with hairpinning disabled and enabled", func() {
		assertHairpinHeaders(false)
		assertHairpinHeaders(true)
	})

	It("scopes a route service signature to its context path", func() {
		testState.StopAndCleanup()
		testState.StartGorouterOrFail()
		routeServiceHeaders := make(chan http.Header, 1)
		routeService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			routeServiceHeaders <- r.Header.Clone()
			forwardedURL := r.Header.Get(routeservice.HeaderKeyForwardedURL)
			request := testState.newGetRequest(forwardedURL)
			for _, header := range []string{
				routeservice.HeaderKeyForwardedURL,
				routeservice.HeaderKeySignature,
				routeservice.HeaderKeyMetadata,
				"X-Forwarded-Client-Cert",
				"X-Forwarded-Proto",
			} {
				request.Header[header] = r.Header.Values(header)
			}
			request.Header.Set("X-Forwarded-Proto", "route-service-xfp")
			response, err := testState.routeServiceClient.Do(request)
			Expect(err).NotTo(HaveOccurred())
			defer response.Body.Close()
			w.Header().Set("X-Received-XFCC", response.Header.Get("X-Received-XFCC"))
			w.Header().Set("X-Received-XFP", response.Header.Get("X-Received-XFP"))
			w.WriteHeader(response.StatusCode)
		}))
		defer routeService.Close()
		contextRoute := appHostname + "/a"
		testState.registerWithInternalRouteService(app, routeService, contextRoute, testState.cfg.SSLPort)
		testState.register(app, appHostname+"/b")
		routeServiceURL := "https://" + appHostname + "/a"

		response := requestWithSignatureFor("https://"+appHostname+"/b", routeServiceURL)
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		Expect(response.Header.Get("X-Received-XFCC")).To(BeEmpty())
		Expect(response.Header.Get("X-Received-XFP")).To(Equal("https"))
		response.Body.Close()

		testState.StopAndCleanup()
		testState.cfg.RouteServiceConfig.StrictSignatureValidation = true
		testState.StartGorouterOrFail()
		testState.registerWithInternalRouteService(app, routeService, contextRoute, testState.cfg.SSLPort)
		testState.register(app, appHostname+"/b")
		response = requestWithSignatureFor("https://"+appHostname+"/b", routeServiceURL)
		Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
		response.Body.Close()

		testState.StopAndCleanup()
		testState.cfg.RouteServiceConfig.StrictSignatureValidation = false
		testState.cfg.ForwardedClientCert = config.SANITIZE_SET
		testState.cfg.EnableSSL = true
		testState.cfg.ClientCertificateValidationString = "request"
		testState.cfg.RouteServiceRecommendHttps = true
		testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = testState.trustedClientTLSConfig.Certificates
		routeServiceClientTLSConfig := testState.routeServiceClient.Transport.(*http.Transport).TLSClientConfig.Clone()
		routeServiceClientTLSConfig.Certificates = testState.trustedRouteServiceClientTLSConfig.Certificates
		testState.routeServiceClient.Transport.(*http.Transport).TLSClientConfig = routeServiceClientTLSConfig
		testState.StartGorouterOrFail()
		testState.registerWithInternalRouteService(app, routeService, contextRoute, testState.cfg.SSLPort)
		testState.register(app, appHostname+"/b")
		request := testState.newGetRequest("https://" + contextRoute)
		request.Header.Set("X-Forwarded-Proto", "caller-xfp")
		response, err := testState.client.Do(request)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		Expect(response.Header.Get("X-Received-XFCC")).To(Equal(sanitize(testState.trustedClientTLSConfig.Certificates[0])))
		Expect(response.Header.Get("X-Received-XFP")).To(Equal("route-service-xfp"))
		var routeServiceRequestHeaders http.Header
		Eventually(routeServiceHeaders).Should(Receive(&routeServiceRequestHeaders))
		Expect(routeServiceRequestHeaders.Get("X-Forwarded-Client-Cert")).To(Equal(sanitize(testState.trustedClientTLSConfig.Certificates[0])))
		Expect(routeServiceRequestHeaders.Get("X-Forwarded-Proto")).To(Equal("https"))
		response.Body.Close()
	})

	It("matches a route service on its non-default port and path", func() {
		testState.StopAndCleanup()
		testState.StartGorouterOrFail()
		routeService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Received-XFCC", r.Header.Get("X-Forwarded-Client-Cert"))
			w.Header().Set("X-Received-XFP", r.Header.Get("X-Forwarded-Proto"))
		}))
		defer routeService.Close()
		testState.register(routeService, appHostname+"/path")

		forwardedURL := fmt.Sprintf("https://%s:%d/path", appHostname, testState.cfg.SSLPort)
		send := func(targetURL string) *http.Response {
			request := testState.newGetRequest(targetURL)
			request.Header = signedHeaders(forwardedURL, forwardedURL)
			request.Header.Set("X-Forwarded-Client-Cert", "caller-xfcc")
			request.Header.Set("X-Forwarded-Proto", "caller-xfp")
			response, err := testState.client.Do(request)
			Expect(err).NotTo(HaveOccurred())
			return response
		}

		response := send(forwardedURL)
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		Expect(response.Header.Get("X-Received-XFCC")).To(Equal("caller-xfcc"))
		Expect(response.Header.Get("X-Received-XFP")).To(Equal("caller-xfp"))
		response.Body.Close()

		response = send(fmt.Sprintf("https://%s:443/path", appHostname))
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		Expect(response.Header.Get("X-Received-XFCC")).To(BeEmpty())
		Expect(response.Header.Get("X-Received-XFP")).To(Equal("https"))
		response.Body.Close()
	})

	It("classifies HTTP/2 requests at an on-platform route service", func() {
		testState.StopAndCleanup()
		testState.cfg.EnableHTTP2 = true
		testState.StartGorouterOrFail()
		routeService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Received-XFCC", r.Header.Get("X-Forwarded-Client-Cert"))
			w.Header().Set("X-Received-XFP", r.Header.Get("X-Forwarded-Proto"))
		}))
		defer routeService.Close()
		testState.register(routeService, appHostname+"/path")

		protocols := new(http.Protocols)
		protocols.SetHTTP2(true)
		transport := &http.Transport{
			TLSClientConfig: testState.client.Transport.(*http.Transport).TLSClientConfig.Clone(),
			Protocols:       protocols,
		}
		client := &http.Client{Transport: transport}
		defer transport.CloseIdleConnections()
		forwardedURL := fmt.Sprintf("https://%s:%d/path", appHostname, testState.cfg.SSLPort)
		send := func(routeServiceURL string) *http.Response {
			request := testState.newGetRequest(forwardedURL)
			request.Header = signedHeaders(forwardedURL, routeServiceURL)
			request.Header.Set("X-Forwarded-Client-Cert", "caller-xfcc")
			request.Header.Set("X-Forwarded-Proto", "caller-xfp")
			response, err := client.Do(request)
			Expect(err).NotTo(HaveOccurred())
			return response
		}

		response := send(forwardedURL)
		Expect(response.Proto).To(Equal("HTTP/2.0"))
		Expect(response.Header.Get("X-Received-XFCC")).To(Equal("caller-xfcc"))
		Expect(response.Header.Get("X-Received-XFP")).To(Equal("caller-xfp"))
		response.Body.Close()

		response = send("https://another-route-service.some.domain/path")
		Expect(response.Proto).To(Equal("HTTP/2.0"))
		Expect(response.Header.Get("X-Received-XFCC")).To(BeEmpty())
		Expect(response.Header.Get("X-Received-XFP")).To(Equal("https"))
		response.Body.Close()
	})

	It("upgrades WebSocket requests through an on-platform route service", func() {
		testState.StopAndCleanup()
		testState.cfg.RouteServiceConfig.EnableWebsockets = true
		testState.cfg.ForwardedClientCert = config.SANITIZE_SET
		testState.cfg.EnableSSL = true
		testState.cfg.ClientCertificateValidationString = "request"
		testState.cfg.RouteServiceRecommendHttps = true
		testState.client.Transport.(*http.Transport).TLSClientConfig.Certificates = testState.trustedClientTLSConfig.Certificates
		testState.StartGorouterOrFail()
		routeServiceHeaders := make(chan http.Header, 3)
		appHeaders := make(chan http.Header, 1)
		mismatchNext := make(chan struct{}, 1)
		finishAtApp := make(chan struct{}, 1)
		websocketApp := httptest.NewServer(websocket.Server{
			Handler: func(conn *websocket.Conn) {
				appHeaders <- conn.Request().Header.Clone()
				conn.Close()
			},
		})
		defer websocketApp.Close()
		routeServiceTLSConfig := testState.client.Transport.(*http.Transport).TLSClientConfig.Clone()
		routeServiceTLSConfig.Certificates = testState.trustedRouteServiceClientTLSConfig.Certificates
		routeServiceTLSConfig.InsecureSkipVerify = true

		routeService := httptest.NewServer(&httputil.ReverseProxy{
			Rewrite: func(request *httputil.ProxyRequest) {
				routeServiceHeaders <- request.In.Header.Clone()
				forwardedURL, err := url.Parse(request.Out.Header.Get(routeservice.HeaderKeyForwardedURL))
				Expect(err).NotTo(HaveOccurred())
				select {
				case <-mismatchNext:
					finishAtApp <- struct{}{}
					for key, values := range signedHeaders(forwardedURL.String(), "https://"+appHostname+"/different") {
						request.Out.Header[key] = values
					}
					request.Out.Header.Set("X-Forwarded-Client-Cert", "caller-xfcc")
					request.Out.Header.Set("X-Forwarded-Proto", "caller-xfp")
				case <-finishAtApp:
					request.Out.URL, err = url.Parse(websocketApp.URL)
					Expect(err).NotTo(HaveOccurred())
					request.Out.Host = forwardedURL.Host
					request.Out.Header.Set("X-Forwarded-Client-Cert", request.In.Header.Get("X-Forwarded-Client-Cert"))
					request.Out.Header.Set("X-Forwarded-Proto", "route-service-xfp")
					return
				default:
				}
				request.Out.Header.Set("X-Forwarded-Proto", "route-service-xfp")
				request.Out.URL = &url.URL{Scheme: "https", Host: fmt.Sprintf("127.0.0.1:%d", testState.cfg.SSLPort)}
				request.Out.Host = forwardedURL.Host
			},
			Transport: &http.Transport{TLSClientConfig: routeServiceTLSConfig},
		})
		defer routeService.Close()
		testState.registerWithInternalRouteService(websocketApp, routeService, appHostname, testState.cfg.SSLPort)

		location, err := url.Parse("wss://" + appHostname)
		Expect(err).NotTo(HaveOccurred())
		origin, err := url.Parse("https://" + appHostname)
		Expect(err).NotTo(HaveOccurred())
		requestHeaders := make(http.Header)
		requestHeaders.Set("X-Forwarded-Proto", "caller-xfp")
		websocketConfig := &websocket.Config{
			Location: location,
			Origin:   origin,
			Version:  websocket.ProtocolVersionHybi13,
			Header:   requestHeaders,
		}
		tlsConfig := testState.client.Transport.(*http.Transport).TLSClientConfig.Clone()
		tlsConfig.InsecureSkipVerify = true
		rawConnection, err := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", testState.cfg.SSLPort), tlsConfig)
		Expect(err).NotTo(HaveOccurred())
		websocketConnection, err := websocket.NewClient(websocketConfig, rawConnection)
		Expect(err).NotTo(HaveOccurred())
		defer websocketConnection.Close()

		var headers http.Header
		Eventually(routeServiceHeaders).Should(Receive(&headers))
		clientXFCC := sanitize(testState.trustedClientTLSConfig.Certificates[0])
		Expect(headers.Get("X-Forwarded-Client-Cert")).To(Equal(clientXFCC))
		Expect(headers.Get("X-Forwarded-Proto")).To(Equal("https"))
		var appRequestHeaders http.Header
		Eventually(appHeaders).Should(Receive(&appRequestHeaders))
		Expect(appRequestHeaders.Get("X-Forwarded-Client-Cert")).To(Equal(clientXFCC))
		Expect(appRequestHeaders.Get("X-Forwarded-Proto")).To(Equal("route-service-xfp"))

		mismatchNext <- struct{}{}
		secondRawConnection, err := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", testState.cfg.SSLPort), tlsConfig)
		Expect(err).NotTo(HaveOccurred())
		secondWebsocketConnection, err := websocket.NewClient(websocketConfig, secondRawConnection)
		Expect(err).NotTo(HaveOccurred())
		defer secondWebsocketConnection.Close()
		Eventually(routeServiceHeaders).Should(Receive(&headers))
		Eventually(routeServiceHeaders).Should(Receive(&headers))
		Expect(headers.Get("X-Forwarded-Client-Cert")).To(Equal(sanitize(testState.trustedRouteServiceClientTLSConfig.Certificates[0])))
		Expect(headers.Get("X-Forwarded-Proto")).To(Equal("https"))
		Eventually(appHeaders).Should(Receive(&appRequestHeaders))
		Expect(appRequestHeaders.Get("X-Forwarded-Client-Cert")).NotTo(Equal("caller-xfcc"))
		Expect(appRequestHeaders.Get("X-Forwarded-Proto")).To(Equal("route-service-xfp"))
	})

	It("preserves legacy signatures when allowed, sanitizes when disallowed, and rejects in strict mode", func() {
		res := requestWithSignature("")
		Expect(res.StatusCode).To(Equal(http.StatusOK))
		Expect(res.Header.Get("X-Received-XFCC")).To(Equal("caller-xfcc"))
		Expect(res.Header.Get("X-Received-XFP")).To(Equal("caller-xfp"))
		res.Body.Close()

		testState.StopAndCleanup()
		testState.cfg.RouteServiceConfig.AllowUnboundSignatures = false
		testState.StartGorouterOrFail()
		testState.register(app, appHostname)
		res = requestWithSignature("")
		Expect(res.StatusCode).To(Equal(http.StatusOK))
		Expect(res.Header.Get("X-Received-XFCC")).To(BeEmpty())
		Expect(res.Header.Get("X-Received-XFP")).To(Equal("https"))
		res.Body.Close()

		testState.StopAndCleanup()
		testState.cfg.RouteServiceConfig.AllowUnboundSignatures = false
		testState.cfg.RouteServiceConfig.StrictSignatureValidation = true
		testState.StartGorouterOrFail()
		testState.register(app, appHostname)
		res = requestWithSignature("")
		defer res.Body.Close()
		Expect(res.StatusCode).To(Equal(http.StatusBadRequest))
	})
})
