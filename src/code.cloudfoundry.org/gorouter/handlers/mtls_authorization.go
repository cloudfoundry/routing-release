package handlers

import (
	"log/slog"
	"net/http"
	"slices"

	"github.com/urfave/negroni/v3"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/logger"
)

// mtlsAuthorization enforces authorization checks on mTLS domains by verifying
// that the calling application is in the allowed sources list for the target endpoint.
type mtlsAuthorization struct {
	config *config.Config
	logger *slog.Logger
}

// NewMtlsAuthorization creates a new mTLS authorization handler
func NewMtlsAuthorization(cfg *config.Config, logger *slog.Logger) negroni.Handler {
	return &mtlsAuthorization{
		config: cfg,
		logger: logger,
	}
}

func (h *mtlsAuthorization) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	reqInfo, err := ContextRequestInfo(r)
	if err != nil {
		// If RequestInfo is not available, return 500
		h.logger.Error("mtls-authorization-failed", logger.ErrAttr(err), slog.String("reason", "request-info-missing"))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Check if this is an mTLS domain
	if !h.config.IsMtlsDomain(r.Host) {
		// Not an mTLS domain, no authorization required
		next(w, r)
		return
	}

	// On mTLS domains, we need a valid endpoint to check authorization
	if reqInfo.RouteEndpoint == nil {
		h.logger.Info("mtls-authorization-denied",
			slog.String("host", r.Host),
			slog.String("reason", "no-endpoint"))
		w.WriteHeader(http.StatusNotFound)
		return
	}

	endpoint := reqInfo.RouteEndpoint

	// If endpoint has no allowed sources, deny by default on mTLS domains
	// Per RFC: if Any is not set and no Apps/Spaces/Orgs are specified, default-deny
	if endpoint.AllowedSources == nil {
		h.logger.Info("mtls-authorization-denied",
			slog.String("host", r.Host),
			slog.String("endpoint-app", endpoint.ApplicationId),
			slog.String("reason", "no-allowed-sources"))
		w.WriteHeader(http.StatusForbidden)
		return
	}

	allowedSources := endpoint.AllowedSources

	// If Any is true, allow any authenticated app
	if allowedSources.Any {
		// Check that caller identity exists (authenticated)
		if reqInfo.CallerIdentity == nil {
			h.logger.Info("mtls-authorization-denied",
				slog.String("host", r.Host),
				slog.String("endpoint-app", endpoint.ApplicationId),
				slog.String("reason", "no-caller-identity"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Any authenticated app is allowed
		h.logger.Debug("mtls-authorization-granted",
			slog.String("host", r.Host),
			slog.String("endpoint-app", endpoint.ApplicationId),
			slog.String("caller-app", reqInfo.CallerIdentity.AppGUID),
			slog.String("reason", "any-authenticated-app"))
		next(w, r)
		return
	}

	// If Any is false, check specific Apps/Spaces/Orgs
	// At least one of Apps/Spaces/Orgs must be specified (RFC requirement)
	if len(allowedSources.Apps) == 0 && len(allowedSources.Spaces) == 0 && len(allowedSources.Orgs) == 0 {
		h.logger.Info("mtls-authorization-denied",
			slog.String("host", r.Host),
			slog.String("endpoint-app", endpoint.ApplicationId),
			slog.String("reason", "empty-allowed-sources"))
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// Check if caller identity was extracted from client certificate
	if reqInfo.CallerIdentity == nil {
		h.logger.Info("mtls-authorization-denied",
			slog.String("host", r.Host),
			slog.String("endpoint-app", endpoint.ApplicationId),
			slog.String("reason", "no-caller-identity"))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	identity := reqInfo.CallerIdentity

	// Check if caller's app GUID is in the allowed apps list
	if slices.Contains(allowedSources.Apps, identity.AppGUID) {
		h.logger.Debug("mtls-authorization-granted",
			slog.String("host", r.Host),
			slog.String("endpoint-app", endpoint.ApplicationId),
			slog.String("caller-app", identity.AppGUID),
			slog.String("reason", "app-in-allowed-list"))
		next(w, r)
		return
	}

	// Check if caller's space GUID is in the allowed spaces list
	if identity.SpaceGUID != "" && slices.Contains(allowedSources.Spaces, identity.SpaceGUID) {
		h.logger.Debug("mtls-authorization-granted",
			slog.String("host", r.Host),
			slog.String("endpoint-app", endpoint.ApplicationId),
			slog.String("caller-app", identity.AppGUID),
			slog.String("caller-space", identity.SpaceGUID),
			slog.String("reason", "space-in-allowed-list"))
		next(w, r)
		return
	}

	// Check if caller's org GUID is in the allowed orgs list
	if identity.OrgGUID != "" && slices.Contains(allowedSources.Orgs, identity.OrgGUID) {
		h.logger.Debug("mtls-authorization-granted",
			slog.String("host", r.Host),
			slog.String("endpoint-app", endpoint.ApplicationId),
			slog.String("caller-app", identity.AppGUID),
			slog.String("caller-org", identity.OrgGUID),
			slog.String("reason", "org-in-allowed-list"))
		next(w, r)
		return
	}

	// Caller not authorized
	h.logger.Info("mtls-authorization-denied",
		slog.String("host", r.Host),
		slog.String("endpoint-app", endpoint.ApplicationId),
		slog.String("caller-app", identity.AppGUID),
		slog.String("caller-space", identity.SpaceGUID),
		slog.String("caller-org", identity.OrgGUID),
		slog.String("reason", "not-in-allowed-sources"))
	w.WriteHeader(http.StatusForbidden)
}
