package postselection

import (
	"fmt"
	"log/slog"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/handlers"
	"code.cloudfoundry.org/gorouter/route"
)

// MtlsScopeAuth performs post-selection domain-level scope authorization.
// It checks whether the caller's org/space identity matches the SELECTED
// endpoint's org/space tags, implementing the RFC's post-selection enforcement
// model.
//
// This handler runs AFTER endpoint selection (load balancing) and enforces
// strict scope boundaries. When a route is shared across spaces with scope=space,
// intermittent 403 errors are expected as the RFC acknowledges this as the
// tradeoff for strict per-endpoint authorization.
type MtlsScopeAuth struct {
	config *config.Config
	logger *slog.Logger
}

// NewMtlsScopeAuth creates a new post-selection scope authorization handler.
// Returns NoopPostSelectionHandler when no mTLS domains are configured.
func NewMtlsScopeAuth(cfg *config.Config, logger *slog.Logger) PostSelectionHandler {
	if len(cfg.Domains) == 0 {
		return NoopPostSelectionHandler
	}
	return &MtlsScopeAuth{
		config: cfg,
		logger: logger,
	}
}

// Check performs post-selection scope authorization against the selected endpoint.
// Returns nil if authorized, or an AuthError if the caller's org/space
// does not match the selected endpoint's org/space tags.
func (h *MtlsScopeAuth) Check(endpoint *route.Endpoint, reqInfo *handlers.RequestInfo) error {
	// Get route policy scope from pool
	if reqInfo.RoutePool == nil {
		// This should not happen in normal operation, but if it does,
		// we must deny the request to avoid authorization bypass
		return NewAuthError("internal_error", "route pool missing during authorization")
	}

	routePolicyScope := reqInfo.RoutePool.RoutePolicyScope()
	if routePolicyScope == "" {
		return nil // No scope enforcement configured
	}

	// Scope enforcement requires caller identity
	if reqInfo.CallerIdentity == nil {
		// Defense in depth: identity should have been checked in pre-auth,
		// but explicitly deny here to avoid silent authorization bypass
		return NewAuthError(
			"domain:no_caller_identity",
			"no caller identity present",
		)
	}

	identity := reqInfo.CallerIdentity

	// Perform post-selection scope check against the SELECTED endpoint's tags
	switch routePolicyScope {
	case route.RoutePolicyScopeOrg:
		endpointOrg := endpoint.Tags["organization_id"]
		if endpointOrg != identity.OrgGUID {
			return NewAuthError(
				"domain:scope=org:post-selection",
				fmt.Sprintf("caller org %s does not match selected backend org %s",
					identity.OrgGUID, endpointOrg),
			)
		}

	case route.RoutePolicyScopeSpace:
		endpointSpace := endpoint.Tags["space_id"]
		if endpointSpace != identity.SpaceGUID {
			return NewAuthError(
				"domain:scope=space:post-selection",
				fmt.Sprintf("caller space %s does not match selected backend space %s",
					identity.SpaceGUID, endpointSpace),
			)
		}

	case route.RoutePolicyScopeAny:
		// Any authenticated caller passes scope check
		// Fall through to populate AuthResult

	default:
		// Unknown scope - deny to be safe
		return NewAuthError(
			"domain:scope=unknown:post-selection",
			fmt.Sprintf("unknown route policy scope %q", routePolicyScope),
		)
	}

	// Scope check passed - populate AuthResult for access logs
	if reqInfo.AuthResult == nil {
		reqInfo.AuthResult = &handlers.AuthResult{}
	}
	reqInfo.AuthResult.Outcome = "allowed"
	reqInfo.AuthResult.Rule = fmt.Sprintf("domain:scope=%s", routePolicyScope)

	return nil
}
