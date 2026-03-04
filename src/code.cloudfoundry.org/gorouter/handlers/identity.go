package handlers

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"strings"

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

// identityHandler extracts the caller identity from the X-Forwarded-Client-Cert header
// on mTLS domains. The identity is stored in the RequestInfo context for use by
// authorization handlers.
type identityHandler struct{}

// NewIdentity creates a new identity extraction handler
func NewIdentity() negroni.Handler {
	return &identityHandler{}
}

func (h *identityHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	reqInfo, err := ContextRequestInfo(r)
	if err != nil {
		// If RequestInfo is not available, continue without setting identity
		next(w, r)
		return
	}

	// Extract identity from X-Forwarded-Client-Cert header
	xfccHeader := r.Header.Get("X-Forwarded-Client-Cert")
	if xfccHeader != "" {
		identity, err := extractIdentityFromXFCC(xfccHeader)
		if err == nil {
			reqInfo.CallerIdentity = identity
		}
		// If extraction fails, continue without setting identity
		// The authorization handler will deny access if identity is required
	}

	next(w, r)
}

// extractIdentityFromXFCC parses the X-Forwarded-Client-Cert header and extracts
// the application, space, and organization GUIDs from the client certificate's
// OU (Organizational Unit) field.
//
// Expected XFCC format: Cert="<PEM-encoded-cert>"
// Expected cert OU formats:
// - "app:<app-guid>"
// - "space:<space-guid>"
// - "organization:<org-guid>"
func extractIdentityFromXFCC(xfcc string) (*CallerIdentity, error) {
	// Parse XFCC header to extract PEM certificate
	// Format: Cert="<PEM>"
	certStart := strings.Index(xfcc, "Cert=\"")
	if certStart == -1 {
		return nil, errors.New("no Cert field in XFCC header")
	}

	certStart += len("Cert=\"")
	certEnd := strings.Index(xfcc[certStart:], "\"")
	if certEnd == -1 {
		return nil, errors.New("malformed Cert field in XFCC header")
	}

	pemData := xfcc[certStart : certStart+certEnd]

	// Decode PEM block
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, errors.New("failed to decode PEM certificate")
	}

	// Parse X.509 certificate
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	// Extract GUIDs from OU fields
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
