package handlers

import (
	"fmt"
	"log/slog"
	"strings"

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

// Check performs post-selection access rules authorization.
// Returns nil if authorized, or an AuthError if no access rule matches
// the caller's identity.
func (h *MtlsAccessRulesAuth) Check(endpoint *route.Endpoint, reqInfo *RequestInfo) error {
	// Only enforce access rules if we have caller identity (mTLS domain)
	if reqInfo.CallerIdentity == nil {
		return nil // Not an mTLS domain or no identity extracted
	}

	// Enforce access rules on mTLS domains
	if reqInfo.RoutePool == nil {
		return nil
	}

	poolHost := reqInfo.RoutePool.Host()

	// Get access rules from the selected endpoint (per-endpoint rules)
	accessRules := endpoint.AccessRules
	if len(accessRules) == 0 {
		// Default deny: mTLS domain but no rules configured
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
	if reqInfo.AuthResult == nil {
		reqInfo.AuthResult = &AuthResult{}
	}
	reqInfo.AuthResult.Rule = "route:" + matchedRule

	h.logger.Debug("mtls-access-rules-granted",
		slog.String("route", poolHost),
		slog.String("caller-app", identity.AppGUID),
		slog.String("matched-rule", matchedRule),
		slog.String("endpoint", endpoint.CanonicalAddr()))

	return nil
}
