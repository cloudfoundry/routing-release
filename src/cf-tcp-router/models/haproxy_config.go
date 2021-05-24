package models

import (
	"code.cloudfoundry.org/lager"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type HAProxyConfig map[HAProxyInboundPort]HAProxyFrontend
type HAProxyFrontend map[SniHostname]HAProxyBackend
type HAProxyBackend []HAProxyServer
type HAProxyServer BackendServerKey
type HAProxyInboundPort uint16

func NewHAProxyConfig(routingTable RoutingTable, logger lager.Logger) HAProxyConfig {
	conf := HAProxyConfig{}

	for routingKey := range routingTable.Entries {
		if routingKey.Port == 0 {
			logError(logger, "frontend_configuration.port", routingKey, routingKey.Port)
			continue
		}

		if routingKey.SniHostname != "" && !isValidDNSName(string(routingKey.SniHostname)) {
			logError(logger, "frontend_configuration.sni_hostname", routingKey, routingKey.SniHostname)
			continue
		}

		entry := routingTable.Entries[routingKey]
		backends := make(HAProxyBackend, 0)

		for backendKey := range entry.Backends {
			if backendKey.Port == 0 {
				logError(logger, "backend_configuration.port", routingKey, backendKey.Port)
				continue
			}

			if backendKey.Address == "" || !isValidDNSName(backendKey.Address) {
				logError(logger, "backend_configuration.address", routingKey, backendKey.Address)
				continue
			}
			backends = append(backends, HAProxyServer(backendKey))
		}

		if len(backends) == 0 {
			logError(logger, "backend_configuration.servers", routingKey, "[]")
			continue
		}

		// Sort servers by address, then port for determinism's sake
		sort.SliceStable(backends, func(i, j int) bool {
			if backends[i].Address == backends[j].Address {
				return backends[i].Port < backends[j].Port
			}

			return backends[i].Address < backends[j].Address
		})

		frontend, frontendExists := conf[HAProxyInboundPort(routingKey.Port)]
		if !frontendExists {
			frontend = HAProxyFrontend{}
			conf[HAProxyInboundPort(routingKey.Port)] = frontend
		}

		frontend[routingKey.SniHostname] = backends
	}

	return conf
}

func (hf HAProxyFrontend) ContainsSNIRoutes() bool {
	for hostname := range hf {
		if hostname != "" {
			return true
		}
	}
	return false
}

// Stolen with gratitude from https://github.com/asaskevich/govalidator/blob/v11/patterns.go#L33
var validDNSNameRegexp = regexp.MustCompile(`^([a-zA-Z0-9_]{1}[a-zA-Z0-9_-]{0,62}){1}(\.[a-zA-Z0-9_]{1}[a-zA-Z0-9_-]{0,62})*[\._]?$`)

func isValidDNSName(hostname string) bool {
	if len(strings.Replace(hostname, ".", "", -1)) > 255 {
		return false
	}

	return validDNSNameRegexp.MatchString(hostname)
}

type ErrInvalidField struct {
	Field      string
	RoutingKey RoutingKey
	Value      interface{}
}

func (err ErrInvalidField) Error() string {
	return fmt.Sprintf(
		"Skipping invalid routing table entry for port: %d/sni_hostname: \"%s\". Field: %s. Value: %s.",
		err.RoutingKey.Port,
		err.RoutingKey.SniHostname,
		err.Field,
		err.Value)
}

func logError(logger lager.Logger, field string, routingKey RoutingKey, value interface{}) {
	logger.Error("skipping-invalid-routing-table-entry", ErrInvalidField{
		Field:      field,
		RoutingKey: routingKey,
		Value:      value,
	})
}
