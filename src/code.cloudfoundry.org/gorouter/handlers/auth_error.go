package handlers

import (
	"fmt"
	"net/http"
)

// AuthError represents an authorization failure with structured metadata
// for logging and error responses. Used for both mTLS and future authentication
// methods like SPIFFE JWT tokens.
type AuthError struct {
	// Rule is the authorization rule that failed (e.g., "domain:scope=org:post-selection")
	Rule string
	// Reason is a human-readable explanation of why authorization failed
	Reason string
	// HTTPStatus is the HTTP status code to return (typically 403 Forbidden)
	HTTPStatus int
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("authorization denied: %s (rule: %s)", e.Reason, e.Rule)
}

// ClientMessage returns a generic error message safe for client responses.
// This prevents leaking internal rule names, app GUIDs, or authorization logic.
func (e *AuthError) ClientMessage() string {
	// Return a generic message based on the HTTP status
	switch e.HTTPStatus {
	case http.StatusForbidden:
		return "Forbidden"
	case http.StatusMisdirectedRequest:
		return "Misdirected Request"
	default:
		return http.StatusText(e.HTTPStatus)
	}
}

// NewAuthError creates a new authorization error with 403 Forbidden status
func NewAuthError(rule, reason string) *AuthError {
	return &AuthError{
		Rule:       rule,
		Reason:     reason,
		HTTPStatus: http.StatusForbidden,
	}
}

// NewAuthErrorWithStatus creates a new authorization error with a custom HTTP status
func NewAuthErrorWithStatus(rule, reason string, status int) *AuthError {
	return &AuthError{
		Rule:       rule,
		Reason:     reason,
		HTTPStatus: status,
	}
}

// AuthResult captures the outcome of identity-aware routing authorization.
// This is populated on RequestInfo and flows into access logs.
type AuthResult struct {
	// Outcome is "allowed" or "denied"; empty if no authorization was performed.
	Outcome string
	// Rule identifies which rule matched or caused denial, e.g.
	// "route:cf:app:<guid>", "domain:scope=org", "identity_extraction".
	Rule string
	// DeniedReason is a human-readable explanation for denial, empty on allow.
	DeniedReason string
}
