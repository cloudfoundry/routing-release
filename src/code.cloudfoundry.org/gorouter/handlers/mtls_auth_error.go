package handlers

import (
	"fmt"
	"net/http"
)

// MtlsAuthError represents an mTLS authorization failure with structured metadata
// for logging and error responses.
type MtlsAuthError struct {
	// Rule is the authorization rule that failed (e.g., "domain:scope=org:post-selection")
	Rule string
	// Reason is a human-readable explanation of why authorization failed
	Reason string
	// HTTPStatus is the HTTP status code to return (typically 403 Forbidden)
	HTTPStatus int
}

func (e *MtlsAuthError) Error() string {
	return fmt.Sprintf("mTLS authorization denied: %s (rule: %s)", e.Reason, e.Rule)
}

// NewMtlsAuthError creates a new authorization error with 403 Forbidden status
func NewMtlsAuthError(rule, reason string) *MtlsAuthError {
	return &MtlsAuthError{
		Rule:       rule,
		Reason:     reason,
		HTTPStatus: http.StatusForbidden,
	}
}

// NewMtlsAuthErrorWithStatus creates a new authorization error with a custom HTTP status
func NewMtlsAuthErrorWithStatus(rule, reason string, status int) *MtlsAuthError {
	return &MtlsAuthError{
		Rule:       rule,
		Reason:     reason,
		HTTPStatus: status,
	}
}
