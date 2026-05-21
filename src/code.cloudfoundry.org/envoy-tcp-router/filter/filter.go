// Package main implements an Envoy TCP (L4) Go filter that logs connection identity
// from mTLS client certificates (per CF RFC 0055) and always allows the connection.
//
// This filter uses the envoy.filters.network.golang extension. When loaded by Envoy
// as a shared library (.so), it intercepts every new downstream TCP connection and
// attempts to extract the CF identity encoded in the client certificate's Subject
// DNs (OU fields: "app:<guid>", "space:<guid>", "organization:<guid>").
//
// For the initial implementation the filter always continues (allows) the connection.
// Route-policy enforcement is deferred to a later iteration.
package main

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"github.com/envoyproxy/envoy/contrib/golang/filters/network/source/go/pkg/network"
)

func init() {
	network.RegisterNetworkFilterConfigFactory("tcp-router-filter", &tcpRouterConfigFactory{})
}

// tcpRouterConfigFactory implements network.ConfigFactory.
type tcpRouterConfigFactory struct{}

func (f *tcpRouterConfigFactory) CreateFactoryFromConfig(config interface{}) network.FilterFactory {
	return &tcpRouterFilterFactory{}
}

// tcpRouterFilterFactory implements network.FilterFactory.
type tcpRouterFilterFactory struct{}

func (f *tcpRouterFilterFactory) CreateFilter(cb api.ConnectionCallback) api.DownstreamFilter {
	return &tcpRouterFilter{}
}

// CallerIdentity holds the CF identity extracted from a Diego client certificate
// following the Subject DN OU convention defined in CF RFC 0055.
type CallerIdentity struct {
	AppGUID   string
	SpaceGUID string
	OrgGUID   string
}

// tcpRouterFilter implements api.DownstreamFilter for the envoy-tcp-router L4 plugin.
// It embeds EmptyDownstreamFilter so that only OnNewConnection needs to be overridden.
type tcpRouterFilter struct {
	api.EmptyDownstreamFilter
}

// OnNewConnection is called by Envoy immediately after a new TCP connection is
// accepted, before any application data is exchanged.
//
// Current behaviour (phase 1):
//   - Logs that a new connection has been received.
//   - Always returns NetworkFilterContinue so the connection is passed through.
//
// NOTE: The DownstreamFilter interface does not expose the peer TLS certificate
// directly.  Identity extraction via the raw peer certificate (see
// extractIdentityFromRawCert below) requires access to the downstream SSL
// connection handle (StreamInfo.DownstreamSslConnection()), which is available
// in HTTP Go filters via FilterCallbackHandler.StreamInfo() but is not yet
// passed to the TCP/network DownstreamFilter callbacks.
// A future iteration can either:
//  1. Promote to an HTTP filter after protocol detection, or
//  2. Expose StreamInfo to DownstreamFilter via a companion callback interface.
func (f *tcpRouterFilter) OnNewConnection() api.FilterStatus {
	api.LogInfo("[TCP-Router-Filter] New connection received. Allowing (phase 1 - log only).")
	return api.NetworkFilterContinue
}

// extractIdentityFromRawCert parses a PEM- or DER-encoded X.509 certificate and
// extracts the CF caller identity from Subject DN OUs as defined in CF RFC 0055:
//
//	OU=app:<app-guid>
//	OU=space:<space-guid>
//	OU=organization:<org-guid>
//
// This helper is wired up in phase 2 once the peer cert is accessible.
func extractIdentityFromRawCert(rawCert string) (*CallerIdentity, error) {
	block, _ := pem.Decode([]byte(rawCert))
	var certBytes []byte
	if block != nil {
		certBytes = block.Bytes
	} else {
		certBytes = []byte(rawCert)
	}

	cert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse x509: %w", err)
	}

	identity := &CallerIdentity{}
	for _, ou := range cert.Subject.OrganizationalUnit {
		switch {
		case strings.HasPrefix(ou, "app:"):
			identity.AppGUID = strings.TrimPrefix(ou, "app:")
		case strings.HasPrefix(ou, "space:"):
			identity.SpaceGUID = strings.TrimPrefix(ou, "space:")
		case strings.HasPrefix(ou, "organization:"):
			identity.OrgGUID = strings.TrimPrefix(ou, "organization:")
		}
	}
	return identity, nil
}

// main is required so the file compiles as package main when building the
// shared library with -buildmode=c-shared.
func main() {}
