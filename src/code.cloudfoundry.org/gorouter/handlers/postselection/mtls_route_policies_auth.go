package postselection

import (
	"fmt"
	"log/slog"
	"strings"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/handlers"
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
// Returns NoopPostSelectionHandler when no mTLS domains are configured.
func NewMtlsRoutePoliciesAuth(cfg *config.Config, logger *slog.Logger) PostSelectionHandler {
	if len(cfg.Domains) == 0 {
		return NoopPostSelectionHandler
	}
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
// if no policy matches. Unrecognized (malformed) rules are skipped, but logged
// at warn level so operators can detect misconfigured route policies.
func (h *MtlsRoutePoliciesAuth) evaluateRoutePolicies(policies []string, identity *handlers.CallerIdentity) (string, bool) {
	for _, policy := range policies {
		policy = strings.TrimSpace(policy)
		if policy == "" {
			continue
		}
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
		default:
			// Unrecognized rule syntax. Without this log a malformed policy is
			// silently ignored, making it very hard to diagnose why gorouter is
			// not enforcing a rule the operator believes is configured.
			if h.logger != nil {
				h.logger.Warn("malformed-route-policy", slog.String("policy", policy))
			}
		}
	}
	return "", false
}

// Check performs post-selection route policies authorization.
// Returns nil if authorized, or an AuthError if no route policy matches
// the caller's identity.
func (h *MtlsRoutePoliciesAuth) Check(endpoint *route.Endpoint, reqInfo *handlers.RequestInfo) error {
	// Get route policy scope from pool
	if reqInfo.RoutePool == nil {
		// This should not happen in normal operation, but if it does,
		// we must deny the request to avoid authorization bypass
		return NewAuthError("internal_error", "route pool missing during authorization")
	}

	routePolicyScope := reqInfo.RoutePool.RoutePolicyScope()
	if routePolicyScope == "" {
		return nil // No route policy enforcement configured
	}

	// Route policy enforcement requires caller identity
	if reqInfo.CallerIdentity == nil {
		// Defense in depth: identity should have been checked in pre-auth,
		// but explicitly deny here to avoid silent authorization bypass
		return NewAuthError(
			"route:no_caller_identity",
			"no caller identity present",
		)
	}

	// Get route policies from the pool. The pool holds the most up-to-date
	// policies for the route; per-endpoint copies can be stale when a route is
	// shared across backends, so authorization must use the pool-level view.
	routePolicies := reqInfo.RoutePool.RoutePolicies()
	if len(routePolicies) == 0 {
		// Default deny: mTLS domain with enforcement enabled but no policies configured
		return NewAuthError(
			"route:no_route_policies",
			"route has no route policies configured",
		)
	}

	// Evaluate route policies
	identity := reqInfo.CallerIdentity
	matchedPolicy, allowed := h.evaluateRoutePolicies(routePolicies, identity)

	if !allowed {
		return NewAuthError(
			"route:route_policies",
			fmt.Sprintf("caller app %s not in route_policies", identity.AppGUID),
		)
	}

	// Route policy matched - populate reqInfo for RTR logs
	if reqInfo.AuthResult == nil {
		reqInfo.AuthResult = &handlers.AuthResult{}
	}
	reqInfo.AuthResult.Outcome = "allowed"
	reqInfo.AuthResult.Rule = "route:" + matchedPolicy

	return nil
}
