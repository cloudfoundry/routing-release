package handlers

import (
	"log/slog"
	"net/http"

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

	// If endpoint has no allowed sources list, deny by default on mTLS domains
	if endpoint.AllowedSourceAppGUIDs == nil || len(endpoint.AllowedSourceAppGUIDs) == 0 {
		h.logger.Info("mtls-authorization-denied",
			slog.String("host", r.Host),
			slog.String("endpoint-app", endpoint.ApplicationId),
			slog.String("reason", "no-allowed-sources"))
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

	// Verify the calling app GUID is in the allowed sources list
	callerAppGUID := reqInfo.CallerIdentity.AppGUID
	allowed := false
	for _, allowedGUID := range endpoint.AllowedSourceAppGUIDs {
		if allowedGUID == callerAppGUID {
			allowed = true
			break
		}
	}

	if !allowed {
		h.logger.Info("mtls-authorization-denied",
			slog.String("host", r.Host),
			slog.String("endpoint-app", endpoint.ApplicationId),
			slog.String("caller-app", callerAppGUID),
			slog.String("reason", "app-not-in-allowed-sources"))
		w.WriteHeader(http.StatusForbidden)
		return
	}

	// Authorization successful
	h.logger.Debug("mtls-authorization-granted",
		slog.String("host", r.Host),
		slog.String("endpoint-app", endpoint.ApplicationId),
		slog.String("caller-app", callerAppGUID))

	next(w, r)
}
