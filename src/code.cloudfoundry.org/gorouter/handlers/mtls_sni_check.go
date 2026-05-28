package handlers

import (
	"log/slog"
	"net/http"
	"strings"

	"code.cloudfoundry.org/gorouter/config"
	"github.com/urfave/negroni/v3"
)

// mtlsSniCheck validates that the TLS SNI and Host header are consistent with
// the configured mTLS domains. It returns 421 Misdirected Request when a
// mismatch is detected.
//
// This handler runs BEFORE ClientCert/CfIdentity so that 421 responses skip
// certificate processing entirely (optimization from PR #535 thread 11).
//
// It does NOT perform identity or authorization checks — those happen in
// MtlsPreAuth which runs after CfIdentity has extracted the caller identity.
type mtlsSniCheck struct {
	config *config.Config
	logger *slog.Logger
}

// NewMtlsSniCheck creates a new SNI/Host mismatch check handler.
// Returns NoopHandler when no mTLS domains are configured.
func NewMtlsSniCheck(cfg *config.Config, logger *slog.Logger) negroni.Handler {
	if len(cfg.Domains) == 0 {
		return NoopHandler
	}
	return &mtlsSniCheck{
		config: cfg,
		logger: logger,
	}
}

func (h *mtlsSniCheck) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	reqInfo, err := ContextRequestInfo(r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	hostDomain := hostWithoutPort(r.Host)
	connState := GetTLSConnectionState(r)
	reqInfo.TlsSNI = connState.SNI

	isMtlsDomain := h.config.IsMtlsDomain(hostDomain)

	// ── Layer 0: Non-mTLS domain handling ──────────────────────────────────────
	if !isMtlsDomain {
		// If the Host is NOT an mTLS domain but the client connected with client
		// cert enforcement (for a different mTLS domain), verify consistency.
		// This prevents SNI-to-Host confusion attacks.
		if connState.ClientCertRequired && !domainMatches(hostDomain, connState.MtlsDomain) {
			w.WriteHeader(http.StatusMisdirectedRequest) // 421
			return
		}
		next(w, r)
		return
	}

	// ── Layer 0b: mTLS domain - verify certificate was required ────────────────
	// For mTLS domains we verify that the TLS handshake actually enforced client
	// certificate validation for *this* domain. Without this check an attacker
	// could connect with SNI for a non-mTLS domain and then send a Host header
	// pointing at an mTLS domain — bypassing certificate validation entirely.
	if !connState.ClientCertRequired || !domainMatches(hostDomain, connState.MtlsDomain) {
		w.WriteHeader(http.StatusMisdirectedRequest) // 421
		return
	}

	// SNI/Host consistent with mTLS domain — continue to certificate processing
	next(w, r)
}

// domainMatches checks if a hostname matches a domain pattern (supports wildcard domains).
// Wildcard patterns (*.domain) only match a single DNS label, not multiple levels.
// Matching is case-insensitive per RFC 1035 (DNS hostnames).
func domainMatches(hostname, domainPattern string) bool {
	// Normalize to lowercase for case-insensitive matching (RFC 1035)
	hostname = strings.ToLower(hostname)
	domainPattern = strings.ToLower(domainPattern)

	if hostname == domainPattern {
		return true
	}
	if strings.HasPrefix(domainPattern, "*.") {
		suffix := domainPattern[1:] // e.g. ".apps.identity"
		if !strings.HasSuffix(hostname, suffix) {
			return false
		}
		prefix := strings.TrimSuffix(hostname, suffix)
		return !strings.Contains(prefix, ".")
	}
	return false
}
