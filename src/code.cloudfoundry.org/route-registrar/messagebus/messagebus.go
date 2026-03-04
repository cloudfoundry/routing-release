package messagebus

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/route-registrar/config"
	"github.com/nats-io/nats.go"
)

//go:generate counterfeiter . MessageBus

type MessageBus interface {
	Connect(servers []config.MessageBusServer, tlsConfig *tls.Config) error
	SendMessage(subject string, route config.Route, privateInstanceId string) error
	Close()
}

type msgBus struct {
	natsHost         *atomic.Value
	natsConn         *nats.Conn
	availabilityZone string
	logger           lager.Logger
}

type Message struct {
	URIs                []string          `json:"uris"`
	Host                string            `json:"host"`
	Protocol            string            `json:"protocol,omitempty"`
	Port                *uint16           `json:"port,omitempty"`
	TLSPort             *uint16           `json:"tls_port,omitempty"`
	Tags                map[string]string `json:"tags"`
	RouteServiceUrl     string            `json:"route_service_url,omitempty"`
	PrivateInstanceId   string            `json:"private_instance_id"`
	ServerCertDomainSAN string            `json:"server_cert_domain_san,omitempty"`
	AvailabilityZone    string            `json:"availability_zone,omitempty"`
	Options             map[string]string `json:"options,omitempty"`
	AllowedSources      *AllowedSources   `json:"allowed_sources,omitempty"`
}

// AllowedSources contains authorization rules for which sources can communicate
// with this endpoint on mTLS domains. Per RFC specification:
// - If Any is true, any authenticated app is allowed (mutually exclusive with Apps/Spaces/Orgs)
// - If Any is false, at least one of Apps/Spaces/Orgs must be specified (default-deny)
type AllowedSources struct {
	Apps   []string `json:"apps,omitempty"`
	Spaces []string `json:"spaces,omitempty"`
	Orgs   []string `json:"orgs,omitempty"`
	Any    bool     `json:"any,omitempty"`
}

const LoadBalancingAlgorithm string = "loadbalancing"

func NewMessageBus(logger lager.Logger, availabilityZone string) MessageBus {
	return &msgBus{
		logger:           logger,
		natsHost:         &atomic.Value{},
		availabilityZone: availabilityZone,
	}
}

func (m *msgBus) Connect(servers []config.MessageBusServer, tlsConfig *tls.Config) error {

	var natsServers []string
	var natsHosts []string
	for _, server := range servers {
		natsServers = append(
			natsServers,
			fmt.Sprintf("nats://%s:%s@%s", server.User, server.Password, server.Host),
		)
		natsHosts = append(natsHosts, server.Host)
	}

	opts := nats.GetDefaultOptions()
	opts.Servers = natsServers
	opts.TLSConfig = tlsConfig
	opts.PingInterval = 20 * time.Second

	opts.ClosedCB = func(conn *nats.Conn) {
		m.logger.Error("nats-connection-closed", errors.New("unexpected nats conn closed"), lager.Data{"nats-host": m.natsHost.Load()})
	}

	opts.DisconnectedCB = func(conn *nats.Conn) {
		m.logger.Info("nats-connection-disconnected", lager.Data{"nats-host": m.natsHost.Load()})
	}

	opts.ReconnectedCB = func(conn *nats.Conn) {
		natsHost, err := parseNatsUrl(conn.ConnectedUrl())
		if err != nil {
			m.logger.Error("nats-url-parse-failed", err, lager.Data{"nats-host": natsHost})
		}
		m.natsHost.Store(natsHost)
		m.logger.Info("nats-connection-reconnected", lager.Data{"nats-host": m.natsHost.Load()})
	}

	natsConn, err := opts.Connect()
	if err != nil {
		m.logger.Error("nats-connection-failed", err, lager.Data{"nats-hosts": natsHosts})
		return err
	}

	natsHost, err := parseNatsUrl(natsConn.ConnectedUrl())
	if err != nil {
		m.logger.Error("nats-url-parse-failed", err, lager.Data{"nats-host": natsHost})
	}

	m.natsHost.Store(natsHost)
	m.logger.Info("nats-connection-successful", lager.Data{"nats-host": m.natsHost.Load()})
	m.natsConn = natsConn

	return nil
}

func (m msgBus) SendMessage(subject string, route config.Route, privateInstanceId string) error {
	m.logger.Debug("creating-message", lager.Data{"subject": subject, "route": route, "privateInstanceId": privateInstanceId})

	routeOptions := m.mapRouteOptions(route)
	allowedSources := m.mapAllowedSources(route)

	msg := &Message{
		URIs:                route.URIs,
		Host:                route.Host,
		Port:                route.Port,
		Protocol:            route.Protocol,
		TLSPort:             route.TLSPort,
		Tags:                route.Tags,
		RouteServiceUrl:     route.RouteServiceUrl,
		ServerCertDomainSAN: route.ServerCertDomainSAN,
		PrivateInstanceId:   privateInstanceId,
		AvailabilityZone:    m.availabilityZone,
		Options:             routeOptions,
		AllowedSources:      allowedSources,
	}

	json, err := json.Marshal(msg)
	if err != nil {
		// Untested as we cannot force json.Marshal to return error.
		return err
	}

	m.logger.Debug("publishing-message", lager.Data{"msg": string(json)})

	return m.natsConn.Publish(subject, json)
}

func (m msgBus) mapRouteOptions(route config.Route) map[string]string {
	if route.Options != nil {
		routeOptions := make(map[string]string)
		if route.Options.LoadBalancingAlgorithm != "" {
			routeOptions[LoadBalancingAlgorithm] = string(route.Options.LoadBalancingAlgorithm)
		}
		return routeOptions
	}
	return nil
}

func (m msgBus) mapAllowedSources(route config.Route) *AllowedSources {
	if route.AllowedSources != nil {
		return &AllowedSources{
			Apps:   route.AllowedSources.Apps,
			Spaces: route.AllowedSources.Spaces,
			Orgs:   route.AllowedSources.Orgs,
			Any:    route.AllowedSources.Any,
		}
	}
	return nil
}

func (m msgBus) Close() {
	m.natsConn.Close()
}

func parseNatsUrl(natsUrl string) (string, error) {
	natsURL, err := url.Parse(natsUrl)
	natsHostStr := ""
	if err != nil {
		return "", err
	} else {
		natsHostStr = natsURL.Host
	}

	return natsHostStr, nil
}
