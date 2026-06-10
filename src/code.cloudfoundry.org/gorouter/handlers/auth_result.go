package handlers

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
