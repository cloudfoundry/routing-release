package handlers

import (
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/urfave/negroni/v3"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/errorwriter"
	log "code.cloudfoundry.org/gorouter/logger"
	"code.cloudfoundry.org/gorouter/routeservice"
)

const xfcc = "X-Forwarded-Client-Cert"

type clientCert struct {
	skipSanitization  func(req *http.Request) bool
	forceDeleteHeader func(req *http.Request) (bool, error)
	forwardingMode    string
	config            *config.Config
	logger            *slog.Logger
	errorWriter       errorwriter.ErrorWriter
}

func NewClientCert(
	skipSanitization func(req *http.Request) bool,
	forceDeleteHeader func(req *http.Request) (bool, error),
	forwardingMode string,
	cfg *config.Config,
	logger *slog.Logger,
	ew errorwriter.ErrorWriter,
) negroni.Handler {
	return &clientCert{
		skipSanitization:  skipSanitization,
		forceDeleteHeader: forceDeleteHeader,
		forwardingMode:    forwardingMode,
		config:            cfg,
		logger:            logger,
		errorWriter:       ew,
	}
}

func (c *clientCert) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	logger := LoggerWithTraceInfo(c.logger, r)
	skip := c.skipSanitization(r)

	// Determine forwarding mode and XFCC format - use domain-specific if on mTLS domain
	forwardingMode := c.forwardingMode
	xfccFormat := config.XFCC_FORMAT_RAW // Default for non-mTLS domains
	mtlsDomainConfig := c.config.GetMtlsDomainConfig(r.Host)
	if mtlsDomainConfig != nil {
		forwardingMode = mtlsDomainConfig.ForwardedClientCert
		xfccFormat = mtlsDomainConfig.XFCCFormat
	}

	if !skip {
		switch forwardingMode {
		case config.FORWARD:
			if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
				r.Header.Del(xfcc)
			}
		case config.SANITIZE_SET:
			r.Header.Del(xfcc)
			if r.TLS != nil {
				if xfccFormat == config.XFCC_FORMAT_ENVOY {
					replaceXFCCHeaderEnvoyFormat(r)
				} else {
					replaceXFCCHeader(r)
				}
			}
		}
	}

	delete, err := c.forceDeleteHeader(r)
	if err != nil {
		c.logger.Error("signature-validation-failed", log.ErrAttr(err))
		if errors.Is(err, routeservice.ErrExpired) {
			c.errorWriter.WriteError(
				rw,
				http.StatusGatewayTimeout,
				fmt.Sprintf("Failed to validate Route Service Signature: %s", err.Error()),
				logger,
			)
		} else {
			c.errorWriter.WriteError(
				rw,
				http.StatusBadGateway,
				fmt.Sprintf("Failed to validate Route Service Signature: %s", err.Error()),
				logger,
			)
		}
		return
	}
	if delete {
		r.Header.Del(xfcc)
	}
	next(rw, r)
}

func replaceXFCCHeader(r *http.Request) {
	// we only care about the first cert at this moment
	if len(r.TLS.PeerCertificates) > 0 {
		cert := r.TLS.PeerCertificates[0]
		b := pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}
		certPEM := pem.EncodeToMemory(&b)
		r.Header.Add(xfcc, sanitize(certPEM))
	}
}

// replaceXFCCHeaderEnvoyFormat sets the X-Forwarded-Client-Cert header using Envoy's
// compact format: Hash=<sha256>;Subject="<DN>"
// This is significantly smaller than the raw certificate format (~300 bytes vs ~1.5KB)
func replaceXFCCHeaderEnvoyFormat(r *http.Request) {
	if len(r.TLS.PeerCertificates) > 0 {
		cert := r.TLS.PeerCertificates[0]
		r.Header.Add(xfcc, formatXFCCEnvoy(cert))
	}
}

// formatXFCCEnvoy generates the Envoy-style XFCC header value:
// Hash=<sha256-hex>;Subject="<X.509 DN>"
func formatXFCCEnvoy(cert *x509.Certificate) string {
	// Calculate SHA-256 hash of the DER-encoded certificate
	hash := sha256.Sum256(cert.Raw)
	hashHex := hex.EncodeToString(hash[:])

	// Format Subject DN using standard X.509 format
	subject := formatSubjectDN(cert.Subject)

	return fmt.Sprintf("Hash=%s;Subject=\"%s\"", hashHex, subject)
}

// formatSubjectDN formats an X.509 Distinguished Name in the standard format
// e.g., "CN=instance-id,OU=app:guid,OU=space:guid,OU=organization:guid"
func formatSubjectDN(name pkix.Name) string {
	var parts []string

	// Add CN first (if present)
	if name.CommonName != "" {
		parts = append(parts, "CN="+name.CommonName)
	}

	// Add OUs (preserve order from certificate)
	for _, ou := range name.OrganizationalUnit {
		parts = append(parts, "OU="+ou)
	}

	// Add O (Organization)
	for _, o := range name.Organization {
		parts = append(parts, "O="+o)
	}

	// Add L (Locality)
	for _, l := range name.Locality {
		parts = append(parts, "L="+l)
	}

	// Add ST (State/Province)
	for _, st := range name.Province {
		parts = append(parts, "ST="+st)
	}

	// Add C (Country)
	for _, c := range name.Country {
		parts = append(parts, "C="+c)
	}

	return strings.Join(parts, ",")
}

func sanitize(cert []byte) string {
	s := string(cert)
	r := strings.NewReplacer("-----BEGIN CERTIFICATE-----", "",
		"-----END CERTIFICATE-----", "",
		"\n", "")
	return r.Replace(s)
}
