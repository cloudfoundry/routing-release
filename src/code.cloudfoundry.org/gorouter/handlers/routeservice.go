package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/urfave/negroni/v3"

	"code.cloudfoundry.org/gorouter/errorwriter"
	log "code.cloudfoundry.org/gorouter/logger"
	"code.cloudfoundry.org/gorouter/registry"
	"code.cloudfoundry.org/gorouter/route"
	"code.cloudfoundry.org/gorouter/routeservice"
)

type RouteService struct {
	config                      *routeservice.RouteServiceConfig
	registry                    registry.Registry
	logger                      *slog.Logger
	errorWriter                 errorwriter.ErrorWriter
	hairpinningAllowlistDomains map[string]struct{}
	hairpinDeniedHosts          sync.Map
}

var errUnboundRouteServiceSignature = errors.New("route service signature is not bound to this destination")

// NewRouteService creates a handler responsible for handling route services
func NewRouteService(
	config *routeservice.RouteServiceConfig,
	routeRegistry registry.Registry,
	logger *slog.Logger,
	errorWriter errorwriter.ErrorWriter,
) negroni.Handler {
	allowlistDomains, err := CreateDomainAllowlist(config.RouteServiceHairpinningAllowlist())

	if err != nil {
		log.Panic(logger, "allowlist-entry-invalid", log.ErrAttr(err))
	}
	return &RouteService{
		config:                      config,
		registry:                    routeRegistry,
		logger:                      logger,
		errorWriter:                 errorWriter,
		hairpinningAllowlistDomains: allowlistDomains,
	}
}

func (r *RouteService) ServeHTTP(rw http.ResponseWriter, req *http.Request, next http.HandlerFunc) {
	logger := LoggerWithTraceInfo(r.logger, req)
	reqInfo, err := ContextRequestInfo(req)
	if err != nil {
		log.Panic(logger, "request-info-err", log.ErrAttr(err))
		return
	}
	if reqInfo.RoutePool == nil {
		log.Panic(logger, "request-info-err", log.ErrAttr(errors.New("failed-to-access-RoutePool")))
		return
	}

	routeServiceURL := reqInfo.RoutePool.RouteServiceUrl()
	if routeServiceURL == "" {
		// No route service is associated with this request
		if r.config.StrictSignatureValidation() && len(req.Header.Values(routeservice.HeaderKeySignature)) > 0 {
			err := r.classifySignature(req)
			if err != nil {
				if errors.Is(err, routeservice.ErrExpired) {
					AddRouterErrorHeader(rw, "expired_route_service_signature")
					r.errorWriter.WriteError(
						rw,
						http.StatusGatewayTimeout,
						fmt.Sprintf("Failed to validate Route Service Signature: %s", err.Error()),
						logger,
					)
				} else {
					AddRouterErrorHeader(rw, "invalid_route_service_signature")
					r.errorWriter.WriteError(
						rw,
						http.StatusBadRequest,
						"invalid route service signature detected",
						logger,
					)
				}
				return
			}
		}

		next(rw, req)
		return
	}

	if !r.config.RouteServiceEnabled() {
		logger.Info("route-service-unsupported")
		AddRouterErrorHeader(rw, "route_service_unsupported")
		r.errorWriter.WriteError(
			rw,
			http.StatusBadGateway,
			"Support for route services is disabled.",
			logger,
		)
		return
	}
	if IsWebSocketUpgrade(req) && !r.config.EnableWebsockets() {
		logger.Info("route-service-unsupported")
		AddRouterErrorHeader(rw, "route_service_unsupported")
		r.errorWriter.WriteError(
			rw,
			http.StatusServiceUnavailable,
			"Websocket requests are not supported for routes bound to Route Services.",
			logger,
		)
		return
	}

	hasBeenToRouteService, err := r.ArrivedViaRouteService(req, logger)
	if err != nil {
		logger.Error("signature-validation-failed", log.ErrAttr(err))
		if errors.Is(err, routeservice.ErrExpired) {
			r.errorWriter.WriteError(
				rw,
				http.StatusGatewayTimeout,
				fmt.Sprintf("Failed to validate Route Service Signature: %s", err.Error()),
				logger,
			)
		} else {
			r.errorWriter.WriteError(
				rw,
				http.StatusBadGateway,
				fmt.Sprintf("Failed to validate Route Service Signature: %s", err.Error()),
				logger,
			)
		}
		return
	}

	if !hasBeenToRouteService && req.Header.Get(routeservice.HeaderKeySignature) != "" {
		logger.Info("route-service-signature-not-trusted-for-current-route-service")
	}

	if hasBeenToRouteService {
		// Remove the headers since the backend should not see it
		req.Header.Del(routeservice.HeaderKeySignature)
		req.Header.Del(routeservice.HeaderKeyMetadata)
		req.Header.Del(routeservice.HeaderKeyForwardedURL)
		next(rw, req)
		return
	}

	// Update request with metadata for route service destination
	var recommendedScheme string
	if r.config.RouteServiceRecommendHttps() {
		recommendedScheme = "https"
	} else {
		recommendedScheme = "http"
	}
	forwardedURLRaw := forwardedURL(recommendedScheme, req)

	var routeServiceArgs routeservice.RequestToSendToRouteService
	err = validateForwardedURL(forwardedURLRaw, req.Host)
	if err == nil {
		routeServiceArgs, err = r.config.CreateRequest(routeServiceURL, forwardedURLRaw)
	}
	if err != nil {
		logger.Error("route-service-failed", log.ErrAttr(err))

		r.errorWriter.WriteError(
			rw,
			http.StatusInternalServerError,
			"Route service request failed.",
			logger,
		)
		return
	}

	hostWithoutPort := hostWithoutPort(routeServiceArgs.ParsedUrl.Host)
	escapedPath := routeServiceArgs.ParsedUrl.EscapedPath()
	if r.config.RouteServiceHairpinning() && r.AllowRouteServiceHairpinningRequest(hostWithoutPort, route.Uri(hostWithoutPort+escapedPath)) {
		reqInfo.ShouldRouteToInternalRouteService = true
	}

	req.Header.Set(routeservice.HeaderKeySignature, routeServiceArgs.Signature)
	req.Header.Set(routeservice.HeaderKeyMetadata, routeServiceArgs.Metadata)
	req.Header.Set(routeservice.HeaderKeyForwardedURL, routeServiceArgs.ForwardedURL)
	reqInfo.RouteServiceURL = routeServiceArgs.ParsedUrl
	next(rw, req)
}

func validateForwardedURL(raw, routedHost string) error {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return err
	}
	if u.Opaque != "" {
		return errors.New("forwarded URL must not be opaque")
	}
	if u.User != nil {
		return errors.New("forwarded URL must not include user info")
	}
	if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("unsupported forwarded URL scheme %q", u.Scheme)
	}
	if !strings.EqualFold(hostWithoutPort(u.Host), hostWithoutPort(routedHost)) {
		return fmt.Errorf("forwarded URL host %q does not match routed host %q", u.Host, routedHost)
	}
	return nil
}

// CreateDomainAllowlist collects the static parts of wildcard allowlist expressions and wildcards stripped of their first segment.
//
// Each entry is checked to follow DNS wildcard notation, e.g.
// *.domain-123.com, subdomain.example.com
//
// but not subdomain.*.example.com, *.*.example.com or invalid DNS names, e.g. ?!.example.com
//
// This function is exported so it can be tested as part of the route service test
func CreateDomainAllowlist(allowlist []string) (map[string]struct{}, error) {

	// This check is a preliminary configuration check and case insensitive. Route URL host names are matched verbatim.
	var validAllowlistEntryPattern = regexp.MustCompile(`(?i)^(\*\.)?[a-z\d-]+(\.[a-z\d-]+)+$`)

	// allowlistHostNames is a hash set containing DNS names or DNS name suffixes based on wildcards.
	// map[string]struct{} is used for minimal memory overhead for the value, using the map as set.
	var allowlistHostNames = make(map[string]struct{}, len(allowlist))

	for _, entry := range allowlist {

		// canonicalize host name to lowercase
		entry = strings.ToLower(entry)

		if !validAllowlistEntryPattern.MatchString(entry) {
			return nil, fmt.Errorf("invalid route service hairpinning allowlist entry: %s. Must be wildcard (*.domain.com) or FQDN (hostname.domain.com)", entry)
		}

		// the allowlist entry is a wildcard. Strip the leading segment ('*').
		if entry[0] == '*' {
			entry = stripFqdnHostname(entry)
		}

		allowlistHostNames[entry] = struct{}{}
	}

	return allowlistHostNames, nil
}

// stripHostFromFQDN strips the first segment of an FQDN, for wildcards or host names.
func stripFqdnHostname(entry string) string {
	_, after, found := strings.Cut(entry, ".")
	if found {
		return "." + after
	}
	return entry
}

// MatchAllowlistHostname checks, if the provided host name matches an entry as is, or matches a wildcard when stripping the first segment.
func (r *RouteService) MatchAllowlistHostname(host string) bool {
	// canonicalize case of the host
	host = strings.ToLower(host)

	// FQDN matches an allowlist entry
	if _, ok := r.hairpinningAllowlistDomains[host]; ok {
		return true
	}

	// Wildcard FQDN suffix matches an allowlist entry
	strippedHostname := stripFqdnHostname(host)
	if _, ok := r.hairpinningAllowlistDomains[strippedHostname]; ok {
		return true
	}

	// doesn't match any allowlist entry, don't allow.
	return false
}

// AllowRouteServiceHairpinningRequest decides whether a route service request can be resolved
// internally via the route registry or should be handled as external request.
// If the provided route is not known to this gorouter's route registry, the request will always be external.
// If the route is known, the hairpinning allowlist is consulted (if defined).
//
// The requested route service hostname must match an allowlist entry, and the matched route must itself match an allowlist entry or be the wildcard route directly above that host.
// Should the allowlist be empty, it is considered disabled and will allow internal resolution of any request
// that can be resolved via the gorouter's route registry.
//
// returns true to use internal resolution via this gorouter, false for external resolution via route service URL call.
func (r *RouteService) AllowRouteServiceHairpinningRequest(host string, uri route.Uri) bool {
	pool := r.registry.Lookup(uri)
	if pool == nil {
		// route is not known to the route registry, resolve externally
		return false
	}

	if len(r.hairpinningAllowlistDomains) == 0 {
		// route is known and there is no allowlist, allow by default
		return true
	}

	h := strings.ToLower(host)
	if strings.HasPrefix(h, ".") || strings.Contains(h, "*") || !r.MatchAllowlistHostname(h) {
		r.logHairpinDenied(h, uri, pool.Host(), "hostname-not-in-allowlist")
		return false
	}

	matched := strings.ToLower(pool.Host())
	_, parent, _ := strings.Cut(h, ".")
	if r.MatchAllowlistHostname(matched) || matched == "*."+parent {
		return true
	}

	r.logHairpinDenied(h, uri, pool.Host(), "matched-route-not-allowed")
	return false
}

func (r *RouteService) logHairpinDenied(host string, uri route.Uri, matched, reason string) {
	attrs := []any{
		slog.String("route_service_host", host),
		slog.String("route_service_uri", uri.String()),
		slog.String("matched_route", matched),
		slog.String("reason", reason),
	}
	if _, seen := r.hairpinDeniedHosts.LoadOrStore(host, struct{}{}); !seen {
		r.logger.Info("route-service-hairpin-denied", attrs...)
	}
	r.logger.Debug("route-service-hairpin-denied", attrs...)
}

func (r *RouteService) IsRouteServiceTraffic(req *http.Request) bool {
	return r.classifySignature(req) == nil
}

func (r *RouteService) classifySignature(req *http.Request) error {
	if req.Header.Get(routeservice.HeaderKeySignature) == "" {
		return errors.New("route service signature is missing")
	}

	signature, err := r.config.ValidateRequest(newRequestReceivedFromRouteService("", req.Header))
	if err != nil {
		return err
	}

	reqInfo, err := ContextRequestInfo(req)
	if err != nil {
		return err
	}
	if reqInfo.RoutePool == nil {
		return errors.New("failed-to-access-RoutePool")
	}

	if reqInfo.RoutePool.RouteServiceUrl() != "" {
		arrivedViaRouteService, err := r.ArrivedViaRouteService(req, r.logger)
		if err != nil {
			return err
		}
		if !arrivedViaRouteService {
			return errUnboundRouteServiceSignature
		}
		return nil
	}

	if signature.RouteServiceUrl == "" {
		if r.config.AllowUnboundSignatures() {
			return nil
		}
		return errUnboundRouteServiceSignature
	}

	signedURL, err := parseSignedRouteServiceURL(signature.RouteServiceUrl)
	if err != nil || !matchesSignedRouteService(signedURL, req) {
		return errUnboundRouteServiceSignature
	}
	return nil
}

func parseSignedRouteServiceURL(raw string) (*url.URL, error) {
	signed, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(signed.Scheme) {
	case "http", "https":
	default:
		return nil, errors.New("unsupported route service URL scheme")
	}
	if signed.Opaque != "" || signed.Host == "" || signed.User != nil || signed.Fragment != "" {
		return nil, errors.New("malformed route service URL")
	}
	return signed, nil
}

func matchesSignedRouteService(signed *url.URL, req *http.Request) bool {
	requestHost := req.Host
	requestPort := ""
	if host, port, err := net.SplitHostPort(req.Host); err == nil {
		requestHost = host
		requestPort = port
	} else if strings.HasPrefix(requestHost, "[") && strings.HasSuffix(requestHost, "]") {
		requestHost = strings.TrimSuffix(strings.TrimPrefix(requestHost, "["), "]")
	}
	if !strings.EqualFold(signed.Hostname(), requestHost) {
		return false
	}

	defaultPort := ""
	switch strings.ToLower(signed.Scheme) {
	case "http":
		defaultPort = "80"
	case "https":
		defaultPort = "443"
	default:
		return false
	}
	signedPort := signed.Port()
	if signedPort == "" {
		signedPort = defaultPort
	}
	if requestPort == "" {
		requestPort = defaultPort
	}
	signedPortNumber, err := strconv.Atoi(signedPort)
	if err != nil {
		return false
	}
	requestPortNumber, err := strconv.Atoi(requestPort)
	if err != nil || signedPortNumber != requestPortNumber {
		return false
	}

	signedPath := signed.EscapedPath()
	if signedPath == "" {
		signedPath = "/"
	}
	requestPath := req.URL.EscapedPath()
	if requestPath == "" {
		requestPath = "/"
	}
	return signedPath == requestPath
}

func (r *RouteService) ArrivedViaRouteService(req *http.Request, logger *slog.Logger) (bool, error) {
	reqInfo, err := ContextRequestInfo(req)
	if err != nil {
		log.Panic(logger, "request-info-err", log.ErrAttr(err))
		return false, err
	}
	if reqInfo.RoutePool == nil {
		err = errors.New("failed-to-access-RoutePool")
		log.Panic(logger, "request-info-err", log.ErrAttr(err))
		return false, err
	}

	var recommendedScheme string

	if r.config.RouteServiceRecommendHttps() {
		recommendedScheme = "https"
	} else {
		recommendedScheme = "http"
	}

	forwardedURLRaw := forwardedURL(recommendedScheme, req)
	routeServiceURL := reqInfo.RoutePool.RouteServiceUrl()
	rsSignature := req.Header.Get(routeservice.HeaderKeySignature)

	if hasBeenToRouteService(routeServiceURL, rsSignature) {
		// A request from a route service destined for a backend instances
		request := newRequestReceivedFromRouteService(forwardedURLRaw, req.Header)
		validatedSig, err := r.config.ValidateRequest(request)
		if err != nil {
			return false, err
		}
		err = r.validateRouteServicePool(validatedSig, reqInfo.RoutePool, req.Host)
		if err != nil {
			return false, err
		}
		if validatedSig.RouteServiceUrl != "" {
			parsedRouteServiceURL, err := url.Parse(reqInfo.RoutePool.RouteServiceUrl())
			if err != nil || parsedRouteServiceURL.String() != validatedSig.RouteServiceUrl {
				return false, nil
			}
		} else if !r.config.AllowUnboundSignatures() {
			return false, nil
		}
		return true, nil
	}
	return false, nil
}

func newRequestReceivedFromRouteService(appUrl string, requestHeaders http.Header) routeservice.RequestReceivedFromRouteService {
	return routeservice.RequestReceivedFromRouteService{
		AppUrl:    appUrl,
		Signature: requestHeaders.Get(routeservice.HeaderKeySignature),
		Metadata:  requestHeaders.Get(routeservice.HeaderKeyMetadata),
	}
}

func (r *RouteService) validateRouteServicePool(
	validatedSig *routeservice.SignatureContents,
	requestPool *route.EndpointPool,
	reqHost string,
) error {

	forwardedURL, err := url.ParseRequestURI(validatedSig.ForwardedUrl)
	if err != nil {
		return err
	}
	if forwardedURL.Opaque != "" || forwardedURL.User != nil ||
		(!strings.EqualFold(forwardedURL.Scheme, "http") && !strings.EqualFold(forwardedURL.Scheme, "https")) {
		return errors.New("malformed forwarded URL")
	}
	if !strings.EqualFold(hostWithoutPort(forwardedURL.Host), hostWithoutPort(reqHost)) {
		return errors.New("forwarded URL host does not match request host")
	}

	uri := route.Uri(hostWithoutPort(forwardedURL.Host) + forwardedURL.EscapedPath())
	forwardedPool := r.registry.Lookup(uri)
	if forwardedPool == nil {
		return fmt.Errorf(
			"original request URL %s does not exist in the routing table",
			uri.String(),
		)
	}

	if !route.PoolsMatch(requestPool, forwardedPool) {
		return fmt.Errorf(
			"route service forwarded URL %s%s does not match the original request URL %s",
			requestPool.Host(),
			requestPool.ContextPath(),
			uri.String(),
		)
	}
	return nil
}

func hasBeenToRouteService(rsUrl, sigHeader string) bool {
	return sigHeader != "" && rsUrl != ""
}
