package handlers

import (
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
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

// cfIdentityHandler extracts the caller identity from the X-Forwarded-Client-Cert header
// on mTLS domains. It parses CF app instance identity certificates (which encode
// app/space/org GUIDs in the Subject OU field). The identity is stored in the
// RequestInfo context for use by authorization handlers.
//
// Security: This handler only extracts identity when:
// 1. TLS was used for the connection
// 2. The request is for a configured mTLS domain
// This prevents spoofing of identity values via crafted XFCC headers on non-mTLS routes.
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

	// Extract identity from X-Forwarded-Client-Cert header using the configured format
	xfccHeader := r.Header.Get("X-Forwarded-Client-Cert")
	if xfccHeader != "" {
		identity, err := extractIdentityFromXFCC(xfccHeader, domainConfig.XFCCFormat)
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
// The format parameter determines how the XFCC header is parsed:
//   - "envoy": Parses Hash=<sha256>;Subject="<DN>" format, extracting OUs from the Subject DN
//   - "raw": Decodes raw base64 certificate (produced by clientcert.go sanitize())
//
// Expected OU formats:
// - "app:<app-guid>"
// - "space:<space-guid>"
// - "organization:<org-guid>"
func extractIdentityFromXFCC(xfcc string, format string) (*CallerIdentity, error) {
	switch format {
	case config.XFCC_FORMAT_ENVOY:
		return extractIdentityFromEnvoyXFCC(xfcc)
	default:
		// "raw" format: base64-encoded DER certificate
		return extractIdentityFromRawXFCC(xfcc)
	}
}

// extractIdentityFromEnvoyXFCC parses the envoy compact format:
// Hash=<sha256>;Subject="<DN>"
func extractIdentityFromEnvoyXFCC(xfcc string) (*CallerIdentity, error) {
	// Parse Subject="<DN>" field
	subjectStart := strings.Index(xfcc, "Subject=\"")
	if subjectStart == -1 {
		return nil, errors.New("envoy format XFCC missing Subject field")
	}
	subjectStart += len("Subject=\"")
	subjectEnd := strings.Index(xfcc[subjectStart:], "\"")
	if subjectEnd == -1 {
		return nil, errors.New("malformed Subject field in XFCC header")
	}
	if subjectEnd == 0 {
		return nil, errors.New("empty Subject field in XFCC header")
	}
	subjectDN := xfcc[subjectStart : subjectStart+subjectEnd]
	return extractIdentityFromSubjectDN(subjectDN)
}

// extractIdentityFromRawXFCC parses raw base64 format (no PEM markers)
// produced by clientcert.go sanitize()
func extractIdentityFromRawXFCC(xfcc string) (*CallerIdentity, error) {
	certDER, err := base64.StdEncoding.DecodeString(strings.TrimSpace(xfcc))
	if err != nil {
		return nil, errors.New("failed to decode base64 certificate: " + err.Error())
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, err
	}

	return extractIdentityFromCert(cert)
}

// extractIdentityFromSubjectDN parses a Subject DN string and extracts GUIDs
// DN format: "CN=instance-id,OU=app:guid,OU=space:guid,OU=organization:guid"
func extractIdentityFromSubjectDN(subjectDN string) (*CallerIdentity, error) {
	identity := &CallerIdentity{}

	// Split DN into RDNs (Relative Distinguished Names)
	// Handle both comma and slash separators
	var rdns []string
	if strings.Contains(subjectDN, ",") {
		rdns = strings.Split(subjectDN, ",")
	} else if strings.Contains(subjectDN, "/") {
		// Some formats use "/" as separator
		rdns = strings.Split(subjectDN, "/")
	} else {
		return nil, fmt.Errorf("unrecognized DN format: %q", subjectDN)
	}

	for _, rdn := range rdns {
		rdn = strings.TrimSpace(rdn)
		if rdn == "" {
			continue
		}

		// Parse OU fields
		if strings.HasPrefix(rdn, "OU=") {
			ouValue := strings.TrimPrefix(rdn, "OU=")
			if strings.HasPrefix(ouValue, "app:") {
				appGUID := strings.TrimPrefix(ouValue, "app:")
				if appGUID != "" {
					identity.AppGUID = appGUID
				}
			} else if strings.HasPrefix(ouValue, "space:") {
				spaceGUID := strings.TrimPrefix(ouValue, "space:")
				if spaceGUID != "" {
					identity.SpaceGUID = spaceGUID
				}
			} else if strings.HasPrefix(ouValue, "organization:") {
				orgGUID := strings.TrimPrefix(ouValue, "organization:")
				if orgGUID != "" {
					identity.OrgGUID = orgGUID
				}
			}
		}
	}

	// At minimum, require app GUID to be present
	if identity.AppGUID == "" {
		return nil, errors.New("no app GUID found in Subject DN")
	}

	return identity, nil
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
