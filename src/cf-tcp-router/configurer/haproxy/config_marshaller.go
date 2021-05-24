package haproxy

import (
	"code.cloudfoundry.org/routing-release/cf-tcp-router/models"
	"fmt"
	"sort"
	"strings"
)

//go:generate counterfeiter -o fakes/fake_config_marshaller.go . ConfigMarshaller
type ConfigMarshaller interface {
	Marshal(models.HAProxyConfig) string
}

type configMarshaller struct{}

func NewConfigMarshaller() ConfigMarshaller {
	return configMarshaller{}
}

func (cm configMarshaller) Marshal(conf models.HAProxyConfig) string {
	var output strings.Builder
	sortedPorts := sortedHAProxyInboundPorts(conf)
	for inboundPortIdx := range sortedPorts {
		port := sortedPorts[inboundPortIdx]
		frontend := conf[port]

		output.WriteString(cm.marshalHAProxyFrontend(port, frontend))
	}
	return output.String()
}

func (cm configMarshaller) marshalHAProxyFrontend(port models.HAProxyInboundPort, frontend models.HAProxyFrontend) string {
	var (
		frontendStanza strings.Builder
		backendStanzas strings.Builder
	)
	frontendStanza.WriteString(fmt.Sprintf("\nfrontend frontend_%d", port))
	frontendStanza.WriteString("\n  mode tcp")
	frontendStanza.WriteString(fmt.Sprintf("\n  bind :%d", port))

	if frontend.ContainsSNIRoutes() {
		frontendStanza.WriteString("\n  tcp-request inspect-delay 5s")
		frontendStanza.WriteString("\n  tcp-request content accept if { req.ssl_hello_type gt 0 }")
	}

	sortedHostnames := sortedSniHostnames(frontend)
	for hostnameIdx := range sortedHostnames {
		var backendCfgName string
		hostname := sortedHostnames[hostnameIdx]

		if hostname == "" { // The non-SNI route gets a default_backend because none of the `use_backend if {...}` predicates will succeed
			backendCfgName = fmt.Sprintf("backend_%d", port)
			frontendStanza.WriteString(fmt.Sprintf("\n  default_backend %s", backendCfgName))

		} else { // SNI routes use named backends
			backendCfgName = fmt.Sprintf("backend_%d_%s", port, hostname)
			frontendStanza.WriteString(fmt.Sprintf("\n  use_backend %s if { req.ssl_sni %s }", backendCfgName, hostname))
		}

		backend := frontend[hostname]
		backendStanzas.WriteString(cm.marshalHAProxyBackend(backendCfgName, backend))
	}

	frontendStanza.WriteString("\n")
	frontendStanza.WriteString(backendStanzas.String())
	return frontendStanza.String()
}

func (cm configMarshaller) marshalHAProxyBackend(backendName string, backend models.HAProxyBackend) string {
	var output strings.Builder
	output.WriteString(fmt.Sprintf("\nbackend %s", backendName))
	output.WriteString("\n  mode tcp")

	for _, server := range backend {
		output.WriteString(fmt.Sprintf("\n  server server_%s_%d %s:%d", server.Address, server.Port, server.Address, server.Port))
	}

	output.WriteString("\n")
	return output.String()
}

func sortedHAProxyInboundPorts(conf models.HAProxyConfig) []models.HAProxyInboundPort {
	keys := make([]models.HAProxyInboundPort, len(conf))
	i := 0
	for k := range conf {
		keys[i] = k
		i++
	}

	sort.SliceStable(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func sortedSniHostnames(frontend models.HAProxyFrontend) []models.SniHostname {
	keys := make([]models.SniHostname, len(frontend))
	i := 0
	for k := range frontend {
		keys[i] = k
		i++
	}

	sort.SliceStable(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
