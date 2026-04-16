package handlers

import (
	"fmt"
	"log/slog"

	"code.cloudfoundry.org/gorouter/config"
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
func NewMtlsScopeAuth(cfg *config.Config, logger *slog.Logger) *MtlsScopeAuth {
	return &MtlsScopeAuth{
		config: cfg,
		logger: logger,
	}
}

// Check performs post-selection scope authorization against the selected endpoint.
// Returns nil if authorized, or an AuthError if the caller's org/space
// does not match the selected endpoint's org/space tags.
func (h *MtlsScopeAuth) Check(endpoint *route.Endpoint, reqInfo *RequestInfo) error {
	// Get access scope from pool
	if reqInfo.RoutePool == nil {
		return nil // Should not happen, but be defensive
	}

	accessScope := reqInfo.RoutePool.AccessScope()
	if accessScope == "" {
		return nil // No scope enforcement configured
	}

	// Scope enforcement requires caller identity
	if reqInfo.CallerIdentity == nil {
		return nil // Identity check should have failed earlier in pre-auth
	}

	identity := reqInfo.CallerIdentity
	poolHost := reqInfo.RoutePool.Host()

	// Perform post-selection scope check against the SELECTED endpoint's tags
	switch accessScope {
	case route.AccessScopeOrg:
		endpointOrg := endpoint.Tags["organization_id"]
		if endpointOrg != identity.OrgGUID {
			h.logger.Info("mtls-scope-auth-denied",
				slog.String("route", poolHost),
				slog.String("scope", "org"),
				slog.String("caller-org", identity.OrgGUID),
				slog.String("endpoint-org", endpointOrg),
				slog.String("endpoint", endpoint.CanonicalAddr()))

			return NewAuthError(
				"domain:scope=org:post-selection",
				fmt.Sprintf("caller org %s does not match selected backend org %s",
					identity.OrgGUID, endpointOrg),
			)
		}

	case route.AccessScopeSpace:
		endpointSpace := endpoint.Tags["space_id"]
		if endpointSpace != identity.SpaceGUID {
			h.logger.Info("mtls-scope-auth-denied",
				slog.String("route", poolHost),
				slog.String("scope", "space"),
				slog.String("caller-space", identity.SpaceGUID),
				slog.String("endpoint-space", endpointSpace),
				slog.String("endpoint", endpoint.CanonicalAddr()))

			return NewAuthError(
				"domain:scope=space:post-selection",
				fmt.Sprintf("caller space %s does not match selected backend space %s",
					identity.SpaceGUID, endpointSpace),
			)
		}

	case route.AccessScopeAny:
		// Any authenticated caller passes scope check
		return nil

	default:
		// Unknown scope - deny to be safe
		h.logger.Warn("mtls-scope-auth-denied",
			slog.String("route", poolHost),
			slog.String("unknown-scope", accessScope))

		return NewAuthError(
			"domain:scope=unknown:post-selection",
			fmt.Sprintf("unknown access scope %q", accessScope),
		)
	}

	// Scope check passed
	h.logger.Debug("mtls-scope-auth-granted",
		slog.String("route", poolHost),
		slog.String("scope", accessScope),
		slog.String("endpoint", endpoint.CanonicalAddr()))

	return nil
}
