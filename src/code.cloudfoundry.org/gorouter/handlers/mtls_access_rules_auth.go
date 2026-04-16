package handlers

import (
	"fmt"
	"log/slog"

	"code.cloudfoundry.org/gorouter/route"
)

// MtlsAccessRulesAuth performs post-selection route-level access rules authorization.
// It evaluates access rules (cf:app:, cf:space:, cf:org:, cf:any) against the
// caller's identity after endpoint selection.
//
// Access rules provide fine-grained per-route authorization beyond domain-level
// scope enforcement. This handler runs in the post-selection pipeline.
type MtlsAccessRulesAuth struct {
	logger *slog.Logger
}

// NewMtlsAccessRulesAuth creates a new post-selection access rules authorization handler.
func NewMtlsAccessRulesAuth(logger *slog.Logger) *MtlsAccessRulesAuth {
	return &MtlsAccessRulesAuth{
		logger: logger,
	}
}

// Check performs post-selection access rules authorization.
// Returns nil if authorized, or an AuthError if no access rule matches
// the caller's identity.
func (h *MtlsAccessRulesAuth) Check(endpoint *route.Endpoint, reqInfo *RequestInfo) error {
	// Only enforce access rules if enforcement is active
	if reqInfo.RoutePool == nil {
		return nil
	}

	accessScope := reqInfo.RoutePool.AccessScope()
	if accessScope == "" {
		return nil // No enforcement active
	}

	// Access rules require caller identity
	if reqInfo.CallerIdentity == nil {
		return nil // Identity check should have failed earlier in pre-auth
	}

	poolHost := reqInfo.RoutePool.Host()

	// Get access rules from the pool
	accessRules := reqInfo.RoutePool.AccessRules()
	if len(accessRules) == 0 {
		// Default deny: enforcement is active but no rules configured
		h.logger.Info("mtls-access-rules-denied",
			slog.String("route", poolHost),
			slog.String("reason", "no-access-rules"),
			slog.String("endpoint", endpoint.CanonicalAddr()))

		return NewAuthError(
			"route:no_access_rules",
			"route has no access rules configured",
		)
	}

	// Evaluate access rules
	identity := reqInfo.CallerIdentity
	matchedRule, allowed := evaluateAccessRules(accessRules, identity)

	if !allowed {
		h.logger.Info("mtls-access-rules-denied",
			slog.String("route", poolHost),
			slog.String("caller-app", identity.AppGUID),
			slog.String("reason", "access-rules-deny"),
			slog.String("endpoint", endpoint.CanonicalAddr()))

		return NewAuthError(
			"route:access_rules",
			fmt.Sprintf("caller app %s not in access_rules", identity.AppGUID),
		)
	}

	// Access rule matched - populate reqInfo for RTR logs
	reqInfo.MtlsRule = "route:" + matchedRule

	h.logger.Debug("mtls-access-rules-granted",
		slog.String("route", poolHost),
		slog.String("caller-app", identity.AppGUID),
		slog.String("matched-rule", matchedRule),
		slog.String("endpoint", endpoint.CanonicalAddr()))

	return nil
}
