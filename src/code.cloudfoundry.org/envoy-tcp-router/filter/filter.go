// Package main implements an Envoy TCP (L4) Go filter that logs new connections
// and provides a hook for future SAN-based mTLS validation (CF RFC 0055).
//
// This filter is non-terminal (IsTerminalFilter: false). It runs before
// envoy.filters.network.tcp_proxy, which handles the actual upstream connection
// to the CF app container. The upstream cluster name is passed per-filter-chain
// via the plugin_config field (a google.protobuf.StringValue) and is used only
// for the connection log line.
package main

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"github.com/envoyproxy/envoy/contrib/golang/filters/network/source/go/pkg/network"
)

func init() {
	network.RegisterNetworkFilterConfigFactory("tcp-router-filter", &tcpRouterConfigFactory{})
	network.RegisterNetworkFilterConfigParser(&tcpRouterConfigParser{})
}

// tcpRouterConfigParser extracts the upstream cluster name from the plugin_config
// Any field (which wraps a google.protobuf.StringValue).
type tcpRouterConfigParser struct{}

func (p *tcpRouterConfigParser) ParseConfig(any *anypb.Any) interface{} {
	var sv wrapperspb.StringValue
	if err := any.UnmarshalTo(&sv); err != nil {
		fmt.Fprintf(os.Stderr, "[TCP-Router-Filter] failed to parse plugin_config: %v\n", err)
		return ""
	}
	return sv.GetValue()
}

// tcpRouterConfigFactory implements network.ConfigFactory.
type tcpRouterConfigFactory struct{}

func (f *tcpRouterConfigFactory) CreateFactoryFromConfig(config interface{}) network.FilterFactory {
	addr, _ := config.(string)
	return &tcpRouterFilterFactory{upstreamAddr: addr}
}

// tcpRouterFilterFactory implements network.FilterFactory.
type tcpRouterFilterFactory struct {
	upstreamAddr string
}

func (f *tcpRouterFilterFactory) CreateFilter(cb api.ConnectionCallback) api.DownstreamFilter {
	return &tcpRouterFilter{
		downstreamCb: cb,
		upstreamAddr: f.upstreamAddr,
	}
}

// tcpRouterFilter is the per-connection downstream filter. It logs the connection
// and returns Continue so that tcp_proxy can handle the upstream connection.
type tcpRouterFilter struct {
	api.EmptyDownstreamFilter

	downstreamCb api.ConnectionCallback
	upstreamAddr string // cluster name, used for logging
}

// OnNewConnection is called immediately after the downstream TCP connection is
// accepted. It logs the connection identity and passes control to tcp_proxy.
func (f *tcpRouterFilter) OnNewConnection() api.FilterStatus {
	clientAddr, _ := f.downstreamCb.StreamInfo().UpstreamRemoteAddress()
	fmt.Fprintf(os.Stderr, "[TCP-Router-Filter] New connection from %s -> upstream %s\n",
		clientAddr, f.upstreamAddr)
	return api.NetworkFilterContinue
}

// CallerIdentity holds the CF identity extracted from a Diego client certificate
// following the Subject DN OU convention defined in CF RFC 0055.
type CallerIdentity struct {
	AppGUID   string
	SpaceGUID string
	OrgGUID   string
}

// extractIdentityFromRawCert parses a PEM- or DER-encoded X.509 certificate and
// extracts the CF caller identity from Subject DN OUs as defined in CF RFC 0055.
// Reserved for phase 2 when the downstream SSL connection handle is accessible.
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
