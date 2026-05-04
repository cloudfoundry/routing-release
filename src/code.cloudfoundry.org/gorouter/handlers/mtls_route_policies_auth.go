package handlers

import (
	"fmt"
	"log/slog"
	"strings"

	"code.cloudfoundry.org/gorouter/route"
)

// MtlsRoutePoliciesAuth performs post-selection route-level route policies authorization.
// It evaluates route policies (cf:app:, cf:space:, cf:org:, cf:any) against the
// caller's identity after endpoint selection.
//
// Route policies provide fine-grained per-route authorization beyond domain-level
// scope enforcement. This handler runs in the post-selection pipeline.
type MtlsRoutePoliciesAuth struct {
	logger *slog.Logger
}

// NewMtlsRoutePoliciesAuth creates a new post-selection route policies authorization handler.
func NewMtlsRoutePoliciesAuth(logger *slog.Logger) *MtlsRoutePoliciesAuth {
	return &MtlsRoutePoliciesAuth{
		logger: logger,
	}
}

// evaluateRoutePolicies checks whether the caller identity satisfies any of the
// route policies. Policies use the source syntax from the RFC:
//
//	cf:any             — allow any authenticated caller
//	cf:app:<guid>      — allow a specific app
//	cf:space:<guid>    — allow all apps in a space
//	cf:org:<guid>      — allow all apps in an org
//
// Returns the matched source string and true on success; empty string and false
// if no policy matches.
func evaluateRoutePolicies(policies []string, identity *CallerIdentity) (string, bool) {
	for _, policy := range policies {
		policy = strings.TrimSpace(policy)
		switch {
		case policy == "cf:any":
			return policy, true
		case strings.HasPrefix(policy, "cf:app:"):
			guid := strings.TrimPrefix(policy, "cf:app:")
			if guid != "" && guid == identity.AppGUID {
				return policy, true
			}
		case strings.HasPrefix(policy, "cf:space:"):
			guid := strings.TrimPrefix(policy, "cf:space:")
			if guid != "" && guid == identity.SpaceGUID {
				return policy, true
			}
		case strings.HasPrefix(policy, "cf:org:"):
			guid := strings.TrimPrefix(policy, "cf:org:")
			if guid != "" && guid == identity.OrgGUID {
				return policy, true
			}
		}
	}
	return "", false
}

// Check performs post-selection route policies authorization.
// Returns nil if authorized, or an AuthError if no route policy matches
// the caller's identity.
func (h *MtlsRoutePoliciesAuth) Check(endpoint *route.Endpoint, reqInfo *RequestInfo) error {
	// Get route policy scope from pool
	if reqInfo.RoutePool == nil {
		return nil // Should not happen, but be defensive
	}

	routePolicyScope := reqInfo.RoutePool.RoutePolicyScope()
	if routePolicyScope == "" {
		return nil // No route policy enforcement configured
	}

	// Route policy enforcement requires caller identity
	if reqInfo.CallerIdentity == nil {
		// Defense in depth: identity should have been checked in pre-auth,
		// but explicitly deny here to avoid silent authorization bypass
		h.logger.Warn("mtls-route-policies-denied",
			slog.String("route", reqInfo.RoutePool.Host()),
			slog.String("reason", "no-caller-identity"),
			slog.String("endpoint", endpoint.CanonicalAddr()))

		return NewAuthError(
			"route:no_caller_identity",
			"no caller identity present",
		)
	}

	poolHost := reqInfo.RoutePool.Host()

	// Get route policies from the selected endpoint (per-endpoint policies)
	routePolicies := endpoint.RoutePolicies
	if len(routePolicies) == 0 {
		// Default deny: mTLS domain with enforcement enabled but no policies configured
		h.logger.Info("mtls-route-policies-denied",
			slog.String("route", poolHost),
			slog.String("reason", "no-route-policies"),
			slog.String("endpoint", endpoint.CanonicalAddr()))

		return NewAuthError(
			"route:no_route_policies",
			"route has no route policies configured",
		)
	}

	// Evaluate route policies
	identity := reqInfo.CallerIdentity
	matchedPolicy, allowed := evaluateRoutePolicies(routePolicies, identity)

	if !allowed {
		h.logger.Info("mtls-route-policies-denied",
			slog.String("route", poolHost),
			slog.String("caller-app", identity.AppGUID),
			slog.String("reason", "route-policies-deny"),
			slog.String("endpoint", endpoint.CanonicalAddr()))

		return NewAuthError(
			"route:route_policies",
			fmt.Sprintf("caller app %s not in route_policies", identity.AppGUID),
		)
	}

	// Route policy matched - populate reqInfo for RTR logs
	if reqInfo.AuthResult == nil {
		reqInfo.AuthResult = &AuthResult{}
	}
	reqInfo.AuthResult.Outcome = "allowed"
	reqInfo.AuthResult.Rule = "route:" + matchedPolicy

	h.logger.Debug("mtls-route-policies-granted",
		slog.String("route", poolHost),
		slog.String("caller-app", identity.AppGUID),
		slog.String("matched-policy", matchedPolicy),
		slog.String("endpoint", endpoint.CanonicalAddr()))

	return nil
}
