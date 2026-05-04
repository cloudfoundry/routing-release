package handlers

import (
	"log/slog"
	"net/http"
	"strings"

	"code.cloudfoundry.org/gorouter/config"
	logger "code.cloudfoundry.org/gorouter/logger"
	"code.cloudfoundry.org/gorouter/route"
)

// mtlsPreAuth performs pre-selection mTLS authorization checks that can be
// validated before endpoint selection (load balancing). This includes:
//   - SNI/Host validation (421 Misdirected Request)
//   - Route pool lookup (404 Not Found)
//   - Identity extraction requirement check (403 Forbidden)
//
// Scope and route policies checking have been moved to post-selection handlers.
type mtlsPreAuth struct {
	config *config.Config
	logger *slog.Logger
}

// NewMtlsPreAuth creates a new pre-selection mTLS authorization handler.
func NewMtlsPreAuth(cfg *config.Config, logger *slog.Logger) *mtlsPreAuth {
	return &mtlsPreAuth{
		config: cfg,
		logger: logger,
	}
}

// domainMatches checks if a hostname matches a domain pattern (supports wildcard domains).
// Wildcard patterns (*.domain) only match a single DNS label, not multiple levels.
// Examples:
//   - domainMatches("mtls-backend.apps.identity", "*.apps.identity") => true
//   - domainMatches("deep.sub.apps.identity", "*.apps.identity") => false (multi-level)
//   - domainMatches("mtls-backend.apps.identity", "mtls-backend.apps.identity") => true
//   - domainMatches("foo.bar.com", "*.apps.identity") => false
func domainMatches(hostname, domainPattern string) bool {
	// Exact match
	if hostname == domainPattern {
		return true
	}
	// Wildcard match - must match single label only
	if strings.HasPrefix(domainPattern, "*.") {
		suffix := domainPattern[1:] // Remove the '*', suffix = ".apps.identity"
		if !strings.HasSuffix(hostname, suffix) {
			return false
		}
		// Extract the prefix before the suffix
		prefix := strings.TrimSuffix(hostname, suffix)
		// Ensure the prefix contains exactly one label (no dots)
		return !strings.Contains(prefix, ".")
	}
	return false
}

// setRouteEndpointForAccessLog sets the RouteEndpoint on reqInfo so that access
// logs are emitted to the target app even when the request is denied before the
// proxy has a chance to select an endpoint.
func setRouteEndpointForAccessLog(reqInfo *RequestInfo, pool *route.EndpointPool, logger *slog.Logger) {
	if pool == nil || reqInfo.RouteEndpoint != nil {
		return
	}
	iter := pool.Endpoints(logger, "", false, route.RoutingProperties{})
	if endpoint := iter.Next(0); endpoint != nil {
		reqInfo.RouteEndpoint = endpoint
	}
}

func (h *mtlsPreAuth) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	reqInfo, err := ContextRequestInfo(r)
	if err != nil {
		h.logger.Error("mtls-pre-auth-failed", logger.ErrAttr(err), slog.String("reason", "request-info-missing"))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	hostDomain := hostWithoutPort(r.Host)
	connState := GetTLSConnectionState(r)
	reqInfo.TlsSNI = connState.SNI

	isMtlsDomain := h.config.IsMtlsDomain(hostDomain)

	// ── Layer 0: Non-mTLS domain handling ──────────────────────────────────────
	if !isMtlsDomain {
		// If the Host is NOT an mTLS domain but the client presented a certificate,
		// verify that the Host matches the mTLS domain from the TLS handshake.
		// This prevents an attack where:
		// 1. Client connects with SNI for an mTLS domain (gets client cert validated)
		// 2. Client sends Host header for a non-mTLS domain (bypasses checks)
		if connState.ClientCertRequired && !domainMatches(hostDomain, connState.MtlsDomain) {
			h.logger.Warn("mtls-enforcement-mismatch",
				slog.String("host", r.Host),
				slog.String("tls_sni", connState.SNI),
				slog.String("tls_mtls_domain", connState.MtlsDomain))
			w.WriteHeader(http.StatusMisdirectedRequest) // 421
			return
		}
		// Not an mTLS domain and no security issue, pass through
		next(w, r)
		return
	}

	// ── Layer 0b: mTLS domain - verify certificate was required ────────────────
	// For mTLS domains we verify that the TLS handshake actually enforced client
	// certificate validation for *this* domain. Without this check an attacker
	// could connect with SNI for a non-mTLS domain and then send a Host header
	// pointing at an mTLS domain — bypassing certificate validation entirely.
	if !connState.ClientCertRequired || !domainMatches(hostDomain, connState.MtlsDomain) {
		h.logger.Warn("mtls-enforcement-mismatch",
			slog.String("host", r.Host),
			slog.String("tls_sni", connState.SNI),
			slog.String("tls_mtls_domain", connState.MtlsDomain))
		w.WriteHeader(http.StatusMisdirectedRequest) // 421
		return
	}

	// ── Layer 1: Route lookup ──────────────────────────────────────────────────
	if reqInfo.RoutePool == nil || reqInfo.RoutePool.IsEmpty() {
		h.logger.Info("mtls-pre-auth-denied",
			slog.String("host", r.Host),
			slog.String("reason", "no-route-pool"))
		w.WriteHeader(http.StatusNotFound)
		return
	}

	pool := reqInfo.RoutePool
	applicationId := pool.ApplicationId()

	// ── Layer 2: Route policy scope — is enforcement active? ───────────────────
	// Cloud Controller sets route_policy_scope in route options when the domain
	// was created with --enforce-route-policies. An empty scope means "no
	// enforcement": the route is on an mTLS domain but authorization is handled
	// by the backend.
	routePolicyScope := pool.RoutePolicyScope()
	if routePolicyScope == "" {
		// No enforcement — forward without authorization checks.
		next(w, r)
		return
	}

	// Enforcement is active — we need caller identity for all checks below.
	if reqInfo.CallerIdentity == nil {
		h.logger.Info("mtls-pre-auth-denied",
			slog.String("host", r.Host),
			slog.String("endpoint-app", applicationId),
			slog.String("reason", "identity-extraction-failed"))
		setRouteEndpointForAccessLog(reqInfo, pool, h.logger)
		reqInfo.AuthResult = &AuthResult{
			Outcome:      "denied",
			Rule:         "identity_extraction",
			DeniedReason: "certificate does not contain CF identity OU fields",
		}
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// Pre-auth checks passed — continue to proxy (scope and route policies will be
	// checked post-selection in the round tripper).
	next(w, r)
}
