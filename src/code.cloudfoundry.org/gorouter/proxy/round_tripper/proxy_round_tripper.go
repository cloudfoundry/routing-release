package round_tripper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	router_http "code.cloudfoundry.org/gorouter/common/http"
	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/handlers"
	log "code.cloudfoundry.org/gorouter/logger"
	"code.cloudfoundry.org/gorouter/metrics"
	"code.cloudfoundry.org/gorouter/proxy/fails"
	"code.cloudfoundry.org/gorouter/proxy/utils"
	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/routeservice"
)

const (
	VcapCookieId                             = "__VCAP_ID__"
	VcapMetaCookieId                         = "__VCAP_ID_META__"
	CookieHeader                             = "Set-Cookie"
	BadGatewayMessage                        = "502 Bad Gateway: Registered endpoint failed to handle the request."
	HostnameErrorMessage                     = "503 Service Unavailable"
	InvalidCertificateMessage                = "526 Invalid SSL Certificate"
	SSLHandshakeMessage                      = "525 SSL Handshake Failed"
	SSLCertRequiredMessage                   = "496 SSL Certificate Required"
	ContextCancelledMessage                  = "499 Request Cancelled"
	HTTP2Protocol                            = "http2"
	AuthNegotiateHeaderCookieMaxAgeInSeconds = 60
)

var (
	NoEndpointsAvailable   = errors.New("No endpoints available")
	TooManyResponseHeaders = errors.New("Too many response headers")
)

//go:generate counterfeiter -o fakes/fake_proxy_round_tripper.go . ProxyRoundTripper
type ProxyRoundTripper interface {
	http.RoundTripper
	CancelRequest(*http.Request)
}

type RoundTripperFactory interface {
	New(expectedServerName string, isRouteService, isHttp2 bool) ProxyRoundTripper
}

func GetRoundTripper(endpoint *route.Endpoint, roundTripperFactory RoundTripperFactory, isRouteService, http2Enabled bool) ProxyRoundTripper {
	endpoint.RoundTripperInit.Do(func() {
		endpoint.SetRoundTripperIfNil(func() route.ProxyRoundTripper {
			isHttp2 := (endpoint.Protocol == HTTP2Protocol) && http2Enabled
			return roundTripperFactory.New(endpoint.ServerCertDomainSAN, isRouteService, isHttp2)
		})
	})

	return endpoint.RoundTripper()
}

//go:generate counterfeiter -o fakes/fake_error_handler.go --fake-name ErrorHandler . errorHandler
type errorHandler interface {
	HandleError(utils.ProxyResponseWriter, error)
}

func NewProxyRoundTripper(
	roundTripperFactory RoundTripperFactory,
	retriableClassifiers fails.Classifier,
	logger *slog.Logger,
	combinedReporter metrics.MetricReporter,
	errHandler errorHandler,
	routeServicesTransport http.RoundTripper,
	cfg *config.Config,
) ProxyRoundTripper {

	return &roundTripper{
		logger:                 logger,
		combinedReporter:       combinedReporter,
		roundTripperFactory:    roundTripperFactory,
		retriableClassifier:    retriableClassifiers,
		errorHandler:           errHandler,
		routeServicesTransport: routeServicesTransport,
		config:                 cfg,
	}
}

type roundTripper struct {
	logger                 *slog.Logger
	combinedReporter       metrics.MetricReporter
	roundTripperFactory    RoundTripperFactory
	retriableClassifier    fails.Classifier
	errorHandler           errorHandler
	routeServicesTransport http.RoundTripper
	config                 *config.Config
}

func (rt *roundTripper) RoundTrip(originalRequest *http.Request) (*http.Response, error) {
	var err error
	var res *http.Response
	var endpoint *route.Endpoint

	originalCtx := originalRequest.Context()
	request := originalRequest.Clone(originalCtx)
	if request.Body != nil {
		// Temporarily disable closing of the body while in the RoundTrip function, since
		// the underlying Transport will close the client request body.
		// https://github.com/golang/go/blob/ab5d9f5831cd267e0d8e8954cfe9987b737aec9c/src/net/http/request.go#L179-L182

		request.Body = io.NopCloser(request.Body)
	}

	reqInfo, err := handlers.ContextRequestInfo(request)
	if err != nil {
		return nil, err
	}
	if reqInfo.RoutePool == nil {
		return nil, errors.New("RoutePool not set on context")
	}

	if reqInfo.ProxyResponseWriter == nil {
		return nil, errors.New("ProxyResponseWriter not set on context")
	}

	stickyEndpointID, mustBeSticky := handlers.GetStickySession(request, rt.config.StickySessionCookieNames, rt.config.StickySessionsForAuthNegotiate)
	numberOfEndpoints := reqInfo.RoutePool.NumEndpoints()
	locallyOptimistic := rt.config.LoadBalanceAZPreference == config.AZ_PREF_LOCAL
	routingProperties := route.RoutingProperties{
		RequestHeaders:         &request.Header,
		LocallyOptimistic:      locallyOptimistic,
		GlobalRoutingAlgorithm: rt.config.LoadBalance,
		AZ:                     rt.config.Zone,
	}

	iter := reqInfo.RoutePool.Endpoints(rt.logger, stickyEndpointID, mustBeSticky, routingProperties)

	// The selectEndpointErr needs to be tracked separately. If we get an error
	// while selecting an endpoint we might just have run out of routes. In
	// such cases the last error that was returned by the round trip should be
	// used to produce a 502 instead of the error returned from selecting the
	// endpoint which would result in a 404 Not Found.
	var selectEndpointErr error
	var maxAttempts int
	if reqInfo.RouteServiceURL == nil {
		maxAttempts = max(rt.config.Backends.MaxAttempts, 1)
	} else {
		maxAttempts = rt.config.RouteServiceConfig.MaxAttempts
	}
	triedEndpoints := map[string]bool{}

	// It is safe to use trace inside the loop unconditionally as we set it as
	// the first thing, but code outside the for loop should check if it's nil
	// to be on the safe side.
	var trace *requestTracer
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		logger := rt.logger

		// We have to set the tracing for each round-trip as we otherwise risk
		// having some asynchronous callback modifying the previous trace even
		// after we have reset it as the handling of connections and requests
		// in net/http is performing most of its task concurrently. If we re-use
		// the previous context and set the trace again this causes us to
		// accumulate callbacks as new traces are called in addition to the
		// existing ones which could cause issues if a request is retried a lot.
		request := request.Clone(originalCtx)
		request, trace = traceRequest(request)

		if reqInfo.RouteServiceURL == nil {
			// Because this for-loop is 1-indexed, we substract one from the attempt value passed to selectEndpoint,
			// which expects a 0-indexed value
			endpoint, selectEndpointErr = rt.selectEndpoint(iter, attempt-1)

			if attempt > 1 {
				if attempt > reqInfo.RoutePool.NumEndpoints() {
					// check if new endpoints were registered
					if selectEndpointErr == nil {
						if _, found := triedEndpoints[endpoint.CanonicalAddr()]; found {
							break
						}
					}
				}
			}

			if selectEndpointErr != nil {
				logger.Error("select-endpoint-failed", slog.String("host", reqInfo.RoutePool.Host()), log.ErrAttr(selectEndpointErr))
				break
			}
			logger = logger.With(slog.Group("route-endpoint", endpoint.ToLogData()...))
			triedEndpoints[endpoint.CanonicalAddr()] = true
			reqInfo.RouteEndpoint = endpoint

			logger.Debug("backend", slog.Int("attempt", attempt))
			if endpoint.IsTLS() {
				request.URL.Scheme = "https"
			} else {
				request.URL.Scheme = "http"
			}
			res, err = rt.backendRoundTrip(request, endpoint, iter, logger)

			logger = logger.With(
				slog.Int("attempt", attempt),
				slog.String("vcap_request_id", request.Header.Get(handlers.VcapRequestIdHeader)),
				slog.Int("num-endpoints", numberOfEndpoints),
				slog.Bool("got-connection", trace.GotConn()),
				slog.Bool("wrote-headers", trace.WroteHeaders()),
				slog.Bool("conn-reused", trace.ConnReused()),
				slog.Float64("dns-lookup-time", trace.DnsTime()),
				slog.Float64("dial-time", trace.DialTime()),
				slog.Float64("tls-handshake-time", trace.TlsTime()),
				slog.String("local-address", trace.LocalAddr()),
			)

			if err != nil {
				reqInfo.FailedAttempts++
				reqInfo.LastFailedAttemptFinishedAt = time.Now()
				retriable, err := rt.isRetriable(request, err, trace)

				logger.Error("backend-endpoint-failed",
					log.ErrAttr(err),
					slog.Bool("retriable", retriable),
				)

				iter.EndpointFailed(err)

				if retriable {
					continue
				}
			}

			if res != nil && err == nil {
				err = checkResponseHeaders(rt.config.MaxResponseHeaders, res.Header)
				if err != nil {
					logger.Error("backend-too-many-response-headers",
						log.ErrAttr(err),
						slog.Bool("retriable", false),
					)
					break
				}
			}

			break
		} else {
			logger.Debug(
				"route-service",
				slog.Any("route-service-url", log.StructValue(reqInfo.RouteServiceURL)),
				slog.Int("attempt", attempt),
			)

			endpoint = &route.Endpoint{
				Tags: map[string]string{},
			}
			reqInfo.RouteEndpoint = endpoint
			request.Host = reqInfo.RouteServiceURL.Host
			request.URL = new(url.URL)
			*request.URL = *reqInfo.RouteServiceURL

			var roundTripper http.RoundTripper
			roundTripper = GetRoundTripper(endpoint, rt.roundTripperFactory, true, rt.config.EnableHTTP2)
			if reqInfo.ShouldRouteToInternalRouteService {
				roundTripper = rt.routeServicesTransport
			}

			res, err = rt.timedRoundTrip(roundTripper, request, logger)

			logger = logger.With(
				slog.Int("attempt", attempt),
				slog.String("vcap_request_id", request.Header.Get(handlers.VcapRequestIdHeader)),
				slog.Int("num-endpoints", numberOfEndpoints),
				slog.Bool("got-connection", trace.GotConn()),
				slog.Bool("wrote-headers", trace.WroteHeaders()),
				slog.Bool("conn-reused", trace.ConnReused()),
				slog.Float64("dns-lookup-time", trace.DnsTime()),
				slog.Float64("dial-time", trace.DialTime()),
				slog.Float64("tls-handshake-time", trace.TlsTime()),
				slog.String("local-address", trace.LocalAddr()),
			)

			if err != nil {
				reqInfo.FailedAttempts++
				reqInfo.LastFailedAttemptFinishedAt = time.Now()
				retriable, err := rt.isRetriable(request, err, trace)

				logger.Error(
					"route-service-connection-failed",
					slog.String("route-service-endpoint", request.URL.String()),
					log.ErrAttr(err),
					slog.Bool("retriable", retriable),
				)

				if retriable {
					continue
				}
			}

			if res != nil && err == nil {
				err = checkResponseHeaders(rt.config.MaxResponseHeaders, res.Header)
				if err != nil {
					logger.Error("route-service-too-many-response-headers",
						log.ErrAttr(err),
						slog.Bool("retriable", false),
					)
					break
				}

			}

			if res != nil && (res.StatusCode < 200 || res.StatusCode >= 300) {
				logger.Info(
					"route-service-response",
					slog.String("route-service-endpoint", request.URL.String()),
					slog.Int("status-code", res.StatusCode),
				)
			}

			break
		}
	}

	// if the client disconnects before response is sent then return context.Canceled (499) instead of the gateway error
	if err != nil && errors.Is(originalRequest.Context().Err(), context.Canceled) && !errors.Is(err, context.Canceled) {
		rt.logger.Error("gateway-error-and-original-request-context-cancelled", log.ErrAttr(err))
		err = originalRequest.Context().Err()
		if originalRequest.Body != nil {
			_ = originalRequest.Body.Close()
		}
	}

	// If we have an error from the round trip, we prefer it over errors
	// returned from selecting the endpoint, see declaration of
	// selectEndpointErr for details.
	if err == nil {
		err = selectEndpointErr
	}

	if err != nil {
		rt.errorHandler.HandleError(reqInfo.ProxyResponseWriter, err)
		if handlers.IsWebSocketUpgrade(request) {
			rt.combinedReporter.CaptureWebSocketFailure()
		}
		return nil, err
	}

	// Round trip was successful at this point
	reqInfo.RoundTripSuccessful = true

	// Set status code for access log
	if res != nil {
		reqInfo.ProxyResponseWriter.SetStatus(res.StatusCode)
	}

	// Write metric for ws upgrades
	if handlers.IsWebSocketUpgrade(request) {
		rt.combinedReporter.CaptureWebSocketUpdate()
	}

	if trace != nil {
		// Record the times from the last attempt, but only on success and if we
		// have a trace.
		reqInfo.DnsStartedAt = trace.DnsStart()
		reqInfo.DnsFinishedAt = trace.DnsDone()
		reqInfo.DialStartedAt = trace.DialStart()
		reqInfo.DialFinishedAt = trace.DialDone()
		reqInfo.TlsHandshakeStartedAt = trace.TlsStart()
		reqInfo.TlsHandshakeFinishedAt = trace.TlsDone()
		reqInfo.LocalAddress = trace.LocalAddr()
	}

	if res != nil && endpoint.PrivateInstanceId != "" && !requestSentToRouteService(request) {
		setupStickySession(
			res, request.Cookies(), endpoint, stickyEndpointID, rt.config.SecureCookies,
			reqInfo.RoutePool.ContextPath(), rt.config.StickySessionCookieNames,
			rt.config.StickySessionsForAuthNegotiate, rt.logger,
		)
	}

	return res, nil
}

func (rt *roundTripper) CancelRequest(request *http.Request) {
	endpoint, err := handlers.GetEndpoint(request.Context())
	if err != nil {
		return
	}

	tr := GetRoundTripper(endpoint, rt.roundTripperFactory, false, rt.config.EnableHTTP2)
	tr.CancelRequest(request)
}

func (rt *roundTripper) backendRoundTrip(request *http.Request, endpoint *route.Endpoint, iter route.EndpointIterator, logger *slog.Logger) (*http.Response, error) {
	request.URL.Host = endpoint.CanonicalAddr()
	request.Header.Set("X-CF-ApplicationID", endpoint.ApplicationId)
	request.Header.Set("X-CF-InstanceIndex", endpoint.PrivateInstanceIndex)
	setRequestXCfInstanceId(request, endpoint)

	// increment connection stats
	iter.PreRequest(endpoint)

	rt.combinedReporter.CaptureRoutingRequest(endpoint)
	tr := GetRoundTripper(endpoint, rt.roundTripperFactory, false, rt.config.EnableHTTP2)
	res, err := rt.timedRoundTrip(tr, request, logger)

	// decrement connection stats
	iter.PostRequest(endpoint)
	return res, err
}

func (rt *roundTripper) timedRoundTrip(tr http.RoundTripper, request *http.Request, logger *slog.Logger) (*http.Response, error) {
	if rt.getEndpointTimeout(request) <= 0 || handlers.IsWebSocketUpgrade(request) {
		return tr.RoundTrip(request)
	}

	reqCtx, cancel := context.WithTimeout(request.Context(), rt.config.EndpointTimeout)
	request = request.WithContext(reqCtx)

	// unfortunately if the cancel function above is not called that
	// results in a vet error
	vrid := request.Header.Get(handlers.VcapRequestIdHeader)
	go func() {
		<-reqCtx.Done()
		if errors.Is(reqCtx.Err(), context.DeadlineExceeded) {
			logger.Error("backend-request-timeout", log.ErrAttr(reqCtx.Err()), slog.String("vcap_request_id", vrid))
		}
		cancel()
	}()

	resp, err := tr.RoundTrip(request)
	if err != nil {
		cancel()
		return nil, err
	}

	return resp, err
}
func (rt *roundTripper) getEndpointTimeout(request *http.Request) time.Duration {
	httpProtoMajor := request.ProtoMajor
	endpointTimeout := rt.config.EndpointTimeout
	switch httpProtoMajor {
	case 2:
		if rt.config.Http2EndpointTimeout != 0 {
			endpointTimeout = rt.config.Http2EndpointTimeout
		}
	case 1:
		if rt.config.Http1EndpointTimeout != 0 {
			endpointTimeout = rt.config.Http1EndpointTimeout
		}
	}
	return endpointTimeout
}
func (rt *roundTripper) selectEndpoint(iter route.EndpointIterator, attempt int) (*route.Endpoint, error) {
	endpoint := iter.Next(attempt)
	if endpoint == nil {
		return nil, NoEndpointsAvailable
	}

	return endpoint, nil
}

func checkResponseHeaders(maxCount int, headers http.Header) error {
	if maxCount > 0 {
		// Go doesn't split header values on commas, instead it only splits the value when it's
		// provided via repeated header keys. We can therefore get the number of header lines by
		// checking how many values are in the map.
		hdrCount := 0
		for _, vv := range headers {
			hdrCount += len(vv)
		}

		if hdrCount > maxCount {
			return TooManyResponseHeaders
		}
	}

	return nil
}

func setRequestXCfInstanceId(request *http.Request, endpoint *route.Endpoint) {
	value := endpoint.PrivateInstanceId
	if value == "" {
		value = endpoint.CanonicalAddr()
	}

	request.Header.Set(router_http.CfInstanceIdHeader, value)
}

// vcapAttributes holds the decoded attributes from the __VCAP_ID_META__ cookie value, used to restore cookie attributes when refreshing a stale instance.
type vcapAttributes struct {
	secure       bool
	sameSite     http.SameSite
	partitioned  bool
	maxAgeEpoch  int64 // Unix epoch = time.Now()+MaxAge; remaining seconds restored on refresh. -1 = MaxAge < 0 to invalidate cookie.
	expiresEpoch int64 // Unix epoch from original Expires; restored verbatim on refresh.
}

// getMaxAgeFromMetaCookie returns the remaining MaxAge in seconds, or -1 if the original MaxAge has elapsed or was negative.
func (m vcapAttributes) getMaxAgeFromMetaCookie() int {
	if m.maxAgeEpoch < 0 {
		return -1
	} else if m.maxAgeEpoch > 0 {
		if remaining := m.maxAgeEpoch - time.Now().Unix(); remaining > 0 {
			return int(remaining)
		} else {
			return -1
		}
	}
	return 0
}

func (m vcapAttributes) getExpiresFromMetaCookie() time.Time {
	if m.expiresEpoch != 0 {
		return time.Unix(m.expiresEpoch, 0)
	}
	return time.Time{}
}

func setupStickySession(
	response *http.Response,
	requestCookies []*http.Cookie,
	endpoint *route.Endpoint,
	originalEndpointId string,
	secureCookies bool,
	path string,
	stickySessionCookieNames config.StringSet,
	authNegotiateSticky bool,
	logger *slog.Logger,
) {
	sessionCookies, vcapCookie := getSessionCookies(response, stickySessionCookieNames)

	// When the application sets VCAP_ID, Gorouter does not overwrite it
	if vcapCookie != nil {
		return
	}

	authHeader := response.Header.Get("WWW-Authenticate")
	hasAuthNegotiate := authNegotiateSticky && strings.HasPrefix(strings.ToLower(authHeader), "negotiate")

	// Define the scenarios for sticky session setup
	// Stale Session: Sticky session to non-existing endpoint
	staleSessionScenario := originalEndpointId != "" && originalEndpointId != endpoint.PrivateInstanceId
	// Auth Negotiation: Auth negotiation headers in request, and session has not yet been established or is stale
	authNegotiateScenario := hasAuthNegotiate && (originalEndpointId == "" || staleSessionScenario)
	// New Session: The application sets a session cookie in the response
	newSessionScenario := len(sessionCookies) > 0

	// Early return: Not a sticky session scenario
	if !(staleSessionScenario || authNegotiateScenario || newSessionScenario) {
		return
	}

	newVcapCookie := func() *http.Cookie {
		return &http.Cookie{
			Name:     VcapCookieId,
			Value:    endpoint.PrivateInstanceId,
			Path:     path,
			HttpOnly: true,
			Secure:   secureCookies,
		}
	}

	if authNegotiateScenario {
		vcapCookie := newVcapCookie()
		vcapCookie.MaxAge = AuthNegotiateHeaderCookieMaxAgeInSeconds
		vcapCookie.SameSite = http.SameSiteStrictMode
		addCookieToResponse(response, vcapCookie, logger)
	} else if newSessionScenario {
		for _, sc := range sessionCookies {
			vcapCookie := newVcapCookie()
			vcapCookie.Secure = vcapCookie.Secure || sc.Secure // config.SecureCookies (via newVcapCookie) always overrules
			vcapCookie.SameSite = sc.SameSite
			vcapCookie.Partitioned = sc.Partitioned
			vcapCookie.MaxAge = sc.MaxAge
			vcapCookie.Expires = sc.Expires
			addCookieToResponse(response, vcapCookie, logger)
			addCookieToResponse(response, createVcapMetaCookie(vcapCookie), logger)
		}
	} else if staleSessionScenario {
		vcapCookie := newVcapCookie()
		if m := getAttributesFromMetaCookie(requestCookies, logger); m != nil { // Take cookie attributes from __VCAP_ID_META__ in request
			vcapCookie.Secure = vcapCookie.Secure || m.secure // config.SecureCookies (via newVcapCookie) always overrules
			vcapCookie.SameSite = m.sameSite
			vcapCookie.Partitioned = m.partitioned
			vcapCookie.MaxAge = m.getMaxAgeFromMetaCookie()
			vcapCookie.Expires = m.getExpiresFromMetaCookie()
		}
		addCookieToResponse(response, vcapCookie, logger)
	}
}

// getSessionCookies returns either the __VCAP_ID__ cookie if present, or all session cookies from the response.
// Multiple session cookies may be present during a CHIPS migration (partitioned + non-partitioned delete).
func getSessionCookies(response *http.Response, stickySessionCookieNames config.StringSet) ([]*http.Cookie, *http.Cookie) {
	var sessionCookies []*http.Cookie
	for _, cookie := range response.Cookies() {
		if cookie.Name == VcapCookieId {
			return nil, cookie
		}
		if IsSessionCookie(cookie.Name, stickySessionCookieNames) {
			sessionCookies = append(sessionCookies, cookie)
		}
	}
	return sessionCookies, nil
}

// IsSessionCookie reports whether cookieName matches a configured sticky session cookie name,
// either directly or after stripping the "__Host-" prefix (RFC 6265bis).
func IsSessionCookie(cookieName string, stickySessionCookieNames config.StringSet) bool {
	name := strings.TrimPrefix(cookieName, "__Host-")
	_, ok := stickySessionCookieNames[name]
	return ok
}

// getAttributesFromMetaCookie returns the __VCAP_ID_META__ cookie from the request cookies, when it exists
func getAttributesFromMetaCookie(cookies []*http.Cookie, logger *slog.Logger) *vcapAttributes {
	for _, c := range cookies {
		if c.Name == VcapMetaCookieId {
			vcapMetaAttributes, err := parseVcapMeta(c.Value)
			if err != nil {
				logger.Error("vcap-id-meta-cookie-parse-error", log.ErrAttr(err))
			}
			return vcapMetaAttributes
		}
	}
	logger.Info("vcap-id-meta-cookie-not-found") // Expected when rolling out VCAP_ID_META
	return nil
}

// parseVcapMeta deserializes the __VCAP_ID_META__ cookie value.
func parseVcapMeta(value string) (*vcapAttributes, error) {
	params, err := url.ParseQuery(value)
	if err != nil {
		return nil, err
	}
	var metaAttributesFromCookie vcapAttributes
	var parseErrors []error
	if params.Has("secure") {
		metaAttributesFromCookie.secure = true
	}
	if params.Has("partitioned") {
		metaAttributesFromCookie.partitioned = true
	}
	switch params.Get("samesite") {
	case "none":
		metaAttributesFromCookie.sameSite = http.SameSiteNoneMode
	case "lax":
		metaAttributesFromCookie.sameSite = http.SameSiteLaxMode
	case "strict":
		metaAttributesFromCookie.sameSite = http.SameSiteStrictMode
	case "":
		// not set — keep zero value
	default:
		parseErrors = append(parseErrors, fmt.Errorf("samesite: unknown value %q", params.Get("samesite")))
	}
	if s := params.Get("maxage"); s != "" {
		if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
			metaAttributesFromCookie.maxAgeEpoch = ts
		} else {
			parseErrors = append(parseErrors, fmt.Errorf("maxage: %w", err))
		}
	}
	if s := params.Get("expires"); s != "" {
		if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
			metaAttributesFromCookie.expiresEpoch = ts
		} else {
			parseErrors = append(parseErrors, fmt.Errorf("expires: %w", err))
		}
	}
	return &metaAttributesFromCookie, errors.Join(parseErrors...)
}

// createVcapMetaCookie copies cookie attributes from __VCAP_ID__ to __VCAP_ID_META__
func createVcapMetaCookie(vcapIdCookie *http.Cookie) *http.Cookie {
	cookieAttributes := encodeCookieAttributes(vcapIdCookie)
	metaCookie := *vcapIdCookie
	metaCookie.Name = VcapMetaCookieId
	metaCookie.Value = cookieAttributes
	return &metaCookie
}

// encodeCookieAttributes encodes the attributes of __VCAP_ID__ as a URL query string, which is stored as the value for __VCAP_ID_META__
func encodeCookieAttributes(vcapIdCookie *http.Cookie) string {
	var parts []string
	if vcapIdCookie.Secure {
		parts = append(parts, "secure")
	}
	if vcapIdCookie.Partitioned {
		parts = append(parts, "partitioned")
	}

	kvParts := url.Values{}
	switch vcapIdCookie.SameSite {
	case http.SameSiteNoneMode:
		kvParts.Set("samesite", "none")
	case http.SameSiteLaxMode:
		kvParts.Set("samesite", "lax")
	case http.SameSiteStrictMode:
		kvParts.Set("samesite", "strict")
	default:
	}

	if vcapIdCookie.MaxAge != 0 {
		var maxAgeEpoch int64
		if vcapIdCookie.MaxAge < 0 {
			maxAgeEpoch = -1
		} else if vcapIdCookie.MaxAge > 0 {
			maxAgeEpoch = time.Now().Unix() + int64(vcapIdCookie.MaxAge)
		}
		kvParts.Set("maxage", strconv.FormatInt(maxAgeEpoch, 10))
	}

	if !vcapIdCookie.Expires.IsZero() {
		expiresEpoch := vcapIdCookie.Expires.Unix()
		kvParts.Set("expires", strconv.FormatInt(expiresEpoch, 10))
	}

	if encoded := kvParts.Encode(); encoded != "" {
		parts = append(parts, encoded)
	}
	return strings.Join(parts, "&")
}

func addCookieToResponse(response *http.Response, cookie *http.Cookie, logger *slog.Logger) {
	if cookieStr := cookie.String(); cookieStr != "" {
		response.Header.Add(CookieHeader, cookieStr)
	} else {
		logger.Error("invalid-cookie-name", slog.String("cookie-name", cookie.Name))
	}
}

func requestSentToRouteService(request *http.Request) bool {
	sigHeader := request.Header.Get(routeservice.HeaderKeySignature)
	rsUrl := request.Header.Get(routeservice.HeaderKeyForwardedURL)
	return sigHeader != "" && rsUrl != ""
}

// Matches behavior of isReplayable() in standard library net/http/request.go
// https://github.com/golang/go/blob/5c489514bc5e61ad9b5b07bd7d8ec65d66a0512a/src/net/http/request.go
func isIdempotent(request *http.Request) bool {
	if request.Body == nil || request.Body == http.NoBody || request.GetBody != nil {
		switch request.Method {
		case "GET", "HEAD", "OPTIONS", "TRACE", "":
			return true
		}
		// The Idempotency-Key, while non-standard, is widely used to
		// mean a POST or other request is idempotent. See
		// https://golang.org/issue/19943#issuecomment-421092421
		if request.Header.Get("Idempotency-Key") != "" || request.Header.Get("X-Idempotency-Key") != "" {
			return true
		}
	}
	return false
}

func (rt *roundTripper) isRetriable(request *http.Request, err error, trace *requestTracer) (bool, error) {
	// if the context has been cancelled we do not perform further retries
	if request.Context().Err() != nil {
		return false, fmt.Errorf("%w (%w)", request.Context().Err(), err)
	}

	// io.EOF errors are considered safe to retry for certain requests
	// Replace the error here to track this state when classifying later.
	if err == io.EOF && isIdempotent(request) {
		err = fails.IdempotentRequestEOFError
	}
	// We can retry for sure if we never obtained a connection
	// since there is no way any data was transmitted. If headers could not
	// be written in full, the request should also be safe to retry.
	if !trace.GotConn() || !trace.WroteHeaders() {
		err = fmt.Errorf("%w (%w)", fails.IncompleteRequestError, err)
	}

	retriable := rt.retriableClassifier.Classify(err)
	return retriable, err
}
