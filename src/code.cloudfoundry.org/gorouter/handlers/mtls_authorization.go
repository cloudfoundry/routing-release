package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/urfave/negroni/v3"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/logger"
	"code.cloudfoundry.org/gorouter/route"
)

// mtlsAuthorization enforces the RFC two-layer mTLS authorization model:
//
//  1. SNI/Host mismatch check — returns 421 if the TLS handshake did not
//     enforce mTLS for the requested mTLS domain.
//
//  2. Route-level authorization — only active when the pool's AccessScope is
//     non-empty (set by Cloud Controller via route options):
//     a. Scope boundary check (any / org / space)
//     b. Access rules check (cf:app:<guid>, cf:space:<guid>, cf:org:<guid>, cf:any)
//     c. Default-deny when AccessScope is set but no AccessRules are present
//
// If the pool has no AccessScope the request is forwarded without checks
// (mTLS domain without enforce_access_rules, used for external client cert validation).
type mtlsAuthorization struct {
	config *config.Config
	logger *slog.Logger
}

// NewMtlsAuthorization creates a new mTLS authorization handler.
func NewMtlsAuthorization(cfg *config.Config, logger *slog.Logger) negroni.Handler {
	return &mtlsAuthorization{
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

func (h *mtlsAuthorization) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	reqInfo, err := ContextRequestInfo(r)
	if err != nil {
		h.logger.Error("mtls-authorization-failed", logger.ErrAttr(err), slog.String("reason", "request-info-missing"))
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

	if !connState.ClientCertRequired || connState.MtlsDomain != hostDomain {
		h.logger.Warn("mtls-enforcement-mismatch",
			slog.String("host", r.Host),
			slog.String("tls_sni", connState.SNI),
			slog.String("tls_mtls_domain", connState.MtlsDomain))
		w.WriteHeader(http.StatusMisdirectedRequest) // 421
		return
	}

	// ── Layer 1: Route lookup ──────────────────────────────────────────────────
	if reqInfo.RoutePool == nil || reqInfo.RoutePool.IsEmpty() {
		h.logger.Info("mtls-authorization-denied",
			slog.String("host", r.Host),
			slog.String("reason", "no-route-pool"))
		w.WriteHeader(http.StatusNotFound)
		return
	}

	pool := reqInfo.RoutePool
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
		h.logger.Info("mtls-authorization-denied",
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

	// ── Layer 2a: Scope boundary check ────────────────────────────────────────
	switch accessScope {
	case route.AccessScopeOrg:
		orgIDs := pool.EndpointOrgIDs()
		if !slices.Contains(orgIDs, identity.OrgGUID) {
			h.logger.Info("mtls-authorization-denied",
				slog.String("host", r.Host),
				slog.String("endpoint-app", applicationId),
				slog.String("caller-org", identity.OrgGUID),
				slog.String("reason", "scope-org-mismatch"))
			setRouteEndpointForAccessLog(reqInfo, pool, h.logger)
			reqInfo.MtlsAuth = "denied"
			reqInfo.MtlsRule = "domain:scope=org"
			reqInfo.MtlsDeniedReason = fmt.Sprintf("caller org %s not in endpoint pool", identity.OrgGUID)
			w.WriteHeader(http.StatusForbidden)
			return
		}

	case route.AccessScopeSpace:
		spaceIDs := pool.EndpointSpaceIDs()
		if !slices.Contains(spaceIDs, identity.SpaceGUID) {
			h.logger.Info("mtls-authorization-denied",
				slog.String("host", r.Host),
				slog.String("endpoint-app", applicationId),
				slog.String("caller-space", identity.SpaceGUID),
				slog.String("reason", "scope-space-mismatch"))
			setRouteEndpointForAccessLog(reqInfo, pool, h.logger)
			reqInfo.MtlsAuth = "denied"
			reqInfo.MtlsRule = "domain:scope=space"
			reqInfo.MtlsDeniedReason = fmt.Sprintf("caller space %s not in endpoint pool", identity.SpaceGUID)
			w.WriteHeader(http.StatusForbidden)
			return
		}

	case route.AccessScopeAny:
		// Any authenticated caller passes scope — nothing more to check here.

	default:
		// Unknown scope — treat as deny to be safe.
		h.logger.Warn("mtls-authorization-denied",
			slog.String("host", r.Host),
			slog.String("endpoint-app", applicationId),
			slog.String("unknown-scope", accessScope))
		setRouteEndpointForAccessLog(reqInfo, pool, h.logger)
		reqInfo.MtlsAuth = "denied"
		reqInfo.MtlsRule = "domain:scope=unknown"
		reqInfo.MtlsDeniedReason = fmt.Sprintf("unknown access scope %q", accessScope)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// ── Layer 2b: Access rules ─────────────────────────────────────────────────
	accessRules := pool.AccessRules()
	if len(accessRules) == 0 {
		// Default deny: enforcement active but no rules configured.
		h.logger.Info("mtls-authorization-denied",
			slog.String("host", r.Host),
			slog.String("endpoint-app", applicationId),
			slog.String("reason", "no-access-rules"))
		setRouteEndpointForAccessLog(reqInfo, pool, h.logger)
		reqInfo.MtlsAuth = "denied"
		reqInfo.MtlsRule = "route:no_access_rules"
		reqInfo.MtlsDeniedReason = "route has no access rules configured"
		w.WriteHeader(http.StatusForbidden)
		return
	}

	matchedRule, allowed := evaluateAccessRules(accessRules, identity)
	if !allowed {
		h.logger.Info("mtls-authorization-denied",
			slog.String("host", r.Host),
			slog.String("endpoint-app", applicationId),
			slog.String("caller-app", identity.AppGUID),
			slog.String("reason", "access-rules-deny"))
		setRouteEndpointForAccessLog(reqInfo, pool, h.logger)
		reqInfo.MtlsAuth = "denied"
		reqInfo.MtlsRule = "route:access_rules"
		reqInfo.MtlsDeniedReason = fmt.Sprintf("caller app %s not in access_rules", identity.AppGUID)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// ── Authorized ─────────────────────────────────────────────────────────────
	h.logger.Debug("mtls-authorization-granted",
		slog.String("host", r.Host),
		slog.String("endpoint-app", applicationId),
		slog.String("caller-app", identity.AppGUID),
		slog.String("matched-rule", matchedRule))
	reqInfo.MtlsAuth = "allowed"
	reqInfo.MtlsRule = "route:" + matchedRule

	next(w, r)
}

// evaluateAccessRules checks whether the caller identity satisfies any of the
// access rules. Rules use the selector syntax from the RFC:
//
//	cf:any             — allow any authenticated caller
//	cf:app:<guid>      — allow a specific app
//	cf:space:<guid>    — allow all apps in a space
//	cf:org:<guid>      — allow all apps in an org
//
// Returns the matched selector string and true on success; empty string and false
// if no rule matches.
func evaluateAccessRules(rules []string, identity *CallerIdentity) (string, bool) {
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		switch {
		case rule == "cf:any":
			return rule, true
		case strings.HasPrefix(rule, "cf:app:"):
			guid := strings.TrimPrefix(rule, "cf:app:")
			if guid == identity.AppGUID {
				return rule, true
			}
		case strings.HasPrefix(rule, "cf:space:"):
			guid := strings.TrimPrefix(rule, "cf:space:")
			if guid != "" && guid == identity.SpaceGUID {
				return rule, true
			}
		case strings.HasPrefix(rule, "cf:org:"):
			guid := strings.TrimPrefix(rule, "cf:org:")
			if guid != "" && guid == identity.OrgGUID {
				return rule, true
			}
		}
	}
	return "", false
}
