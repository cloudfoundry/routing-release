package messagebus

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/routing-release/route-registrar/config"
	"github.com/nats-io/nats.go"
)

//go:generate counterfeiter . MessageBus

type MessageBus interface {
	Connect(servers []config.MessageBusServer, tlsConfig *tls.Config) error
	SendMessage(subject string, host string, route config.Route, privateInstanceId string) error
	Close()
}

type msgBus struct {
	natsHost *atomic.Value
	natsConn *nats.Conn
	logger   lager.Logger
}

type Message struct {
	URIs                []string          `json:"uris"`
	Host                string            `json:"host"`
	Port                *int              `json:"port,omitempty"`
	TLSPort             *int              `json:"tls_port,omitempty"`
	Tags                map[string]string `json:"tags"`
	RouteServiceUrl     string            `json:"route_service_url,omitempty"`
	PrivateInstanceId   string            `json:"private_instance_id"`
	ServerCertDomainSAN string            `json:"server_cert_domain_san,omitempty"`
}

func NewMessageBus(logger lager.Logger) MessageBus {
	return &msgBus{
		logger:   logger,
		natsHost: &atomic.Value{},
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

	opts := nats.DefaultOptions
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

func (m msgBus) SendMessage(subject string, host string, route config.Route, privateInstanceId string) error {
	m.logger.Debug("creating-message", lager.Data{"subject": subject, "host": host, "route": route, "privateInstanceId": privateInstanceId})

	msg := &Message{
		URIs:                route.URIs,
		Host:                host,
		Port:                route.Port,
		TLSPort:             route.TLSPort,
		Tags:                route.Tags,
		RouteServiceUrl:     route.RouteServiceUrl,
		ServerCertDomainSAN: route.ServerCertDomainSAN,
		PrivateInstanceId:   privateInstanceId,
	}

	json, err := json.Marshal(msg)
	if err != nil {
		// Untested as we cannot force json.Marshal to return error.
		return err
	}

	m.logger.Debug("publishing-message", lager.Data{"msg": string(json)})

	return m.natsConn.Publish(subject, json)
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
