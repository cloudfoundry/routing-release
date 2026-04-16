package handlers

import (
	"log/slog"
	"strings"

	"code.cloudfoundry.org/gorouter/route"
)

// domainMatches checks if a hostname matches a domain pattern (supports wildcard domains).
// Examples:
//   - domainMatches("mtls-backend.apps.identity", "*.apps.identity") => true
//   - domainMatches("mtls-backend.apps.identity", "mtls-backend.apps.identity") => true
//   - domainMatches("foo.bar.com", "*.apps.identity") => false
func domainMatches(hostname, domainPattern string) bool {
	// Exact match
	if hostname == domainPattern {
		return true
	}
	// Wildcard match
	if strings.HasPrefix(domainPattern, "*.") {
		suffix := domainPattern[1:] // Remove the '*'
		return strings.HasSuffix(hostname, suffix)
	}
	return false
}

// evaluateAccessRules checks whether the caller identity satisfies any of the
// access rules. Rules use the selector syntax from the RFC:
//
//	cf:any             — allow any authenticated caller
//	cf:app:<guid>      — allow a specific app
//	cf:space:<guid>    — allow all apps in a space
//	cf:org:<guid>      — allow all apps in an org
//
// Returns the matched selector string and true on success; empty string and false
// if no rule matches.
func evaluateAccessRules(rules []string, identity *CallerIdentity) (string, bool) {
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		switch {
		case rule == "cf:any":
			return rule, true
		case strings.HasPrefix(rule, "cf:app:"):
			guid := strings.TrimPrefix(rule, "cf:app:")
			if guid == identity.AppGUID {
				return rule, true
			}
		case strings.HasPrefix(rule, "cf:space:"):
			guid := strings.TrimPrefix(rule, "cf:space:")
			if guid != "" && guid == identity.SpaceGUID {
				return rule, true
			}
		case strings.HasPrefix(rule, "cf:org:"):
			guid := strings.TrimPrefix(rule, "cf:org:")
			if guid != "" && guid == identity.OrgGUID {
				return rule, true
			}
		}
	}
	return "", false
}

// setRouteEndpointForAccessLog sets the RouteEndpoint on reqInfo so that access
// logs are emitted to the target app even when the request is denied before the
// proxy has a chance to select an endpoint.
func setRouteEndpointForAccessLog(reqInfo *RequestInfo, pool *route.EndpointPool, logger *slog.Logger) {
	if pool == nil || reqInfo.RouteEndpoint != nil {
		return
	}
	iter := pool.Endpoints(logger, "", false, route.RoutingProperties{})
	if endpoint := iter.Next(0); endpoint != nil {
		reqInfo.RouteEndpoint = endpoint
	}
}
