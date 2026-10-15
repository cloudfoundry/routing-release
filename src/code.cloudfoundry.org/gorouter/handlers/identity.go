package handlers

import (
	"crypto/x509"
	"errors"
	"net/http"
	"strings"

	"code.cloudfoundry.org/gorouter/config"
	"github.com/urfave/negroni/v3"
)

// CallerIdentity represents the identity of the calling application extracted from mTLS
// certificate. The certificate OU field contains:
// - app:<app-guid> for the application GUID
// - space:<space-guid> for the space GUID
// - organization:<org-guid> for the organization GUID
type CallerIdentity struct {
	AppGUID   string
	SpaceGUID string
	OrgGUID   string
}

// cfIdentityHandler extracts the caller identity from the TLS-verified peer
// certificate on mTLS domains. It parses CF app instance identity certificates
// (which encode app/space/org GUIDs in the Subject OU field). The identity is
// stored in the RequestInfo context for use by authorization handlers.
//
// Security: This handler only extracts identity when:
// 1. TLS was used for the connection
// 2. The request is for a configured mTLS domain
// Identity is always derived from r.TLS.PeerCertificates[0], never from the
// client-controlled X-Forwarded-Client-Cert header, since that header is not
// cryptographically verified and can be forwarded or spoofed depending on the
// domain's forwarded_client_cert mode.
type cfIdentityHandler struct {
	config *config.Config
}

// NewCfIdentity creates a new CF app identity extraction handler.
// Returns NoopHandler when no mTLS domains are configured.
func NewCfIdentity(cfg *config.Config) negroni.Handler {
	if len(cfg.Domains) == 0 {
		return NoopHandler
	}
	return &cfIdentityHandler{config: cfg}
}

func (h *cfIdentityHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	// Only extract identity when TLS was used
	if r.TLS == nil {
		next(w, r)
		return
	}

	// Only extract identity on mTLS domains
	hostDomain := hostWithoutPort(r.Host)
	domainConfig := h.config.GetMtlsDomainConfig(hostDomain)
	if domainConfig == nil {
		next(w, r)
		return
	}

	reqInfo, err := ContextRequestInfo(r)
	if err != nil {
		next(w, r)
		return
	}

	// Derive identity from the TLS-verified peer certificate presented on
	// this connection, never from the client-controlled X-Forwarded-Client-Cert
	// header. The header can be left untouched or forwarded by clientCert
	// depending on the domain's forwarded_client_cert mode, so it is not a
	// trustworthy source of identity; r.TLS.PeerCertificates[0] is the
	// certificate this connection actually authenticated with, verified
	// against the domain's CA pool during the TLS handshake.
	if len(r.TLS.PeerCertificates) > 0 {
		identity, err := extractIdentityFromCert(r.TLS.PeerCertificates[0])
		if err == nil {
			reqInfo.CallerIdentity = identity
		}
		// If extraction fails, continue without setting identity
		// The authorization handler will deny access if identity is required
	}

	next(w, r)
}

// extractIdentityFromCert extracts GUIDs from an X.509 certificate's OU fields
func extractIdentityFromCert(cert *x509.Certificate) (*CallerIdentity, error) {
	identity := &CallerIdentity{}
	for _, ou := range cert.Subject.OrganizationalUnit {
		if strings.HasPrefix(ou, "app:") {
			appGUID := strings.TrimPrefix(ou, "app:")
			if appGUID != "" {
				identity.AppGUID = appGUID
			}
		} else if strings.HasPrefix(ou, "space:") {
			spaceGUID := strings.TrimPrefix(ou, "space:")
			if spaceGUID != "" {
				identity.SpaceGUID = spaceGUID
			}
		} else if strings.HasPrefix(ou, "organization:") {
			orgGUID := strings.TrimPrefix(ou, "organization:")
			if orgGUID != "" {
				identity.OrgGUID = orgGUID
			}
		}
	}

	// At minimum, require app GUID to be present
	if identity.AppGUID == "" {
		return nil, errors.New("no app GUID found in certificate OU")
	}

	return identity, nil
}
