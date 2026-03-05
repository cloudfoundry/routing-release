package handlers

import (
	"crypto/x509"
	"encoding/base64"
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
// Supported XFCC formats:
// 1. Envoy compact format: Hash=<sha256>;Subject="<DN>" - parse OUs from Subject string
// 2. Envoy format with cert: Cert="<PEM-encoded-cert>"
// 3. GoRouter format: raw base64 (no PEM markers) - produced by clientcert.go sanitize()
//
// Expected OU formats:
// - "app:<app-guid>"
// - "space:<space-guid>"
// - "organization:<org-guid>"
func extractIdentityFromXFCC(xfcc string) (*CallerIdentity, error) {
	// Try Envoy compact format first: Subject="<DN>"
	// This is the most efficient format since we don't need to decode a certificate
	if subjectStart := strings.Index(xfcc, "Subject=\""); subjectStart != -1 {
		subjectStart += len("Subject=\"")
		subjectEnd := strings.Index(xfcc[subjectStart:], "\"")
		if subjectEnd == -1 {
			return nil, errors.New("malformed Subject field in XFCC header")
		}
		subjectDN := xfcc[subjectStart : subjectStart+subjectEnd]
		return extractIdentityFromSubjectDN(subjectDN)
	}

	// Try Envoy format with cert: Cert="<PEM>"
	var certDER []byte
	var err error

	if certStart := strings.Index(xfcc, "Cert=\""); certStart != -1 {
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
		certDER = block.Bytes
	} else {
		// GoRouter format: raw base64 without PEM markers
		// The clientcert.go sanitize() function strips PEM markers and newlines
		certDER, err = base64.StdEncoding.DecodeString(strings.TrimSpace(xfcc))
		if err != nil {
			return nil, errors.New("failed to decode base64 certificate: " + err.Error())
		}
	}

	// Parse X.509 certificate
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
		return nil, errors.New("unrecognized DN format")
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
