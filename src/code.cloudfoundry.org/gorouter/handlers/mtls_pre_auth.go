package handlers

import (
	"log/slog"
	"net/http"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/route"
	"github.com/urfave/negroni/v3"
)

// mtlsPreAuth performs pre-selection mTLS authorization checks that require
// caller identity to be available. It MUST run AFTER CfIdentity in the handler
// chain (which extracts CallerIdentity from the XFCC header).
//
// Checks performed:
//   - Route pool existence (404 Not Found)
//   - Route policy scope enforcement (403 Forbidden if identity missing)
//
// SNI/Host validation (421) is handled by MtlsSniCheck which runs earlier.
// Scope and route policies checking are performed post-selection.
type mtlsPreAuth struct {
	config *config.Config
	logger *slog.Logger
}

// NewMtlsPreAuth creates a new pre-selection mTLS authorization handler.
// This handler MUST be placed after CfIdentity in the handler chain.
// Returns NoopHandler when no mTLS domains are configured.
func NewMtlsPreAuth(cfg *config.Config, logger *slog.Logger) negroni.Handler {
	if len(cfg.Domains) == 0 {
		return NoopHandler
	}
	return &mtlsPreAuth{
		config: cfg,
		logger: logger,
	}
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
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	hostDomain := hostWithoutPort(r.Host)

	// Only apply to mTLS domains
	if !h.config.IsMtlsDomain(hostDomain) {
		next(w, r)
		return
	}

	// ── Layer 1: Route lookup ──────────────────────────────────────────────────
	if reqInfo.RoutePool == nil || reqInfo.RoutePool.IsEmpty() {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	pool := reqInfo.RoutePool

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
