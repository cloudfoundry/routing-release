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
// Scope and access rules checking have been moved to post-selection handlers.
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
// Examples:
//   - domainMatches("mtls-backend.apps.identity", "*.apps.identity") => true
//   - domainMatches("mtls-backend.apps.identity", "mtls-backend.apps.identity") => true
//   - domainMatches("foo.bar.com", "*.apps.identity") => false
func domainMatches(hostname, domainPattern string) bool {
	// Exact match
	if hostname == domainPattern {
		return true
	}
	// Wildcard match
	if strings.HasPrefix(domainPattern, "*.") {
		suffix := domainPattern[1:] // Remove the '*'
		return strings.HasSuffix(hostname, suffix)
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

	// ── Layer 0: Non-mTLS domain — no checks required ─────────────────────────
	if !h.config.IsMtlsDomain(hostDomain) {
		next(w, r)
		return
	}

	// ── Layer 0b: SNI / Host mismatch check (421) ──────────────────────────────
	// For mTLS domains we verify that the TLS handshake actually enforced client
	// certificate validation for *this* domain. Without this check an attacker
	// could connect with SNI for a non-mTLS domain and then send a Host header
	// pointing at an mTLS domain — bypassing certificate validation entirely.
	connState := GetTLSConnectionState(r)
	reqInfo.TlsSNI = connState.SNI

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
	var _ *route.EndpointPool = pool // Explicit type reference to satisfy compiler
	applicationId := pool.ApplicationId()

	// ── Layer 2: Access scope — is enforcement active? ─────────────────────────
	// Cloud Controller sets access_scope in route options when the domain was
	// created with --enforce-access-rules. An empty scope means "no enforcement":
	// the route is on an mTLS domain but authorization is handled by the backend.
	accessScope := pool.AccessScope()
	if accessScope == "" {
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
		reqInfo.MtlsAuth = "denied"
		reqInfo.MtlsRule = "identity_extraction"
		reqInfo.MtlsDeniedReason = "certificate does not contain CF identity OU fields"
		w.WriteHeader(http.StatusForbidden)
		return
	}

	identity := reqInfo.CallerIdentity
	// Populate caller fields for RTR log.
	reqInfo.CallerApp = identity.AppGUID
	reqInfo.CallerSpace = identity.SpaceGUID
	reqInfo.CallerOrg = identity.OrgGUID

	// Pre-auth checks passed — continue to proxy (scope and access rules will be
	// checked post-selection in the round tripper).
	next(w, r)
}
