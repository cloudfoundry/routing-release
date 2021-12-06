package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"os"
	"time"

	"code.cloudfoundry.org/tlsconfig"
	"github.com/nats-io/nats.go"
	"gopkg.in/yaml.v2"
)

const USAGE = `Usage:
/var/vcap/jobs/gorouter/bin/nats_client [COMMAND] [SUBJECT] [MESSAGE]

COMMANDS:
  subscribe    (Default) Streams NATS messages from server with provided SUBJECT. Default SUBJECT is 'router.*'
               Example: /var/vcap/jobs/gorouter/bin/nats_client subscribe 'router.*'

  publish      Publish the provided message JSON to SUBJECT subscription. SUBJECT and MESSAGE are required
               Example: /var/vcap/jobs/gorouter/bin/nats_client publish router.register '{"host":"172.217.6.68","port":80,"uris":["bar.example.com"]}'
`

// Simple NATS client for debugging
// Uses gorouter.yml for config
func main() {
	if os.Args[len(os.Args)-1] == "--help" || os.Args[len(os.Args)-1] == "-h" || os.Args[len(os.Args)-1] == "help" {
		fmt.Println(USAGE)
		os.Exit(1)
	}

	configPath := os.Args[1]
	command := "subscribe"
	if len(os.Args) >= 3 {
		command = os.Args[2]
	}
	if command != "subscribe" && command != "publish" {
		fmt.Println(USAGE)
		os.Exit(1)
	}

	subject := "router.*"
	if len(os.Args) >= 4 {
		subject = os.Args[3]
	}

	var message string
	if command == "publish" {
		if len(os.Args) >= 5 {
			message = os.Args[4]
		} else {
			fmt.Println(USAGE)
			os.Exit(1)
		}
	}

	config, err := loadConfig(configPath)
	if err != nil {
		panic(err)
	}

	natsOptions, err := natsOptions(config)
	if err != nil {
		panic(err)
	}

	natsConn, err := natsOptions.Connect()
	if err != nil {
		panic(err)
	}

	if command == "publish" {
		fmt.Fprintf(os.Stderr, "Publishing message to %s\n", subject)
		err := natsConn.Publish(subject, []byte(message))
		if err != nil {
			panic(err)
		}
		fmt.Fprintln(os.Stderr, "Done")
	}

	if command == "subscribe" {
		fmt.Fprintf(os.Stderr, "Subscribing to %s\n", subject)
		subscription, err := natsConn.SubscribeSync(subject)
		if err != nil {
			panic(err)
		}

		for {
			message, err := subscription.NextMsg(time.Second * 60)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error fetching message: %s\n", err.Error())
			} else {
				fmt.Fprintf(os.Stderr, "new message with subject: %s\n", message.Subject)
				fmt.Println(string(message.Data))
			}
		}
	}
}

// From code.cloudfoundry.org/gorouter/mbus/client.go
func natsOptions(c *Config) (nats.Options, error) {
	options := nats.DefaultOptions
	options.Servers = c.NatsServers()
	if c.Nats.TLSEnabled {
		var err error
		options.TLSConfig, err = tlsconfig.Build(
			tlsconfig.WithInternalServiceDefaults(),
			tlsconfig.WithIdentity(c.Nats.ClientAuthCertificate),
		).Client(
			tlsconfig.WithAuthority(c.Nats.CAPool),
		)
		if err != nil {
			return nats.Options{}, err
		}
	}
	options.PingInterval = c.NatsClientPingInterval
	options.MaxReconnect = -1

	return options, nil
}

// From src/code.cloudfoundry.org/gorouter/config/config.go
type Config struct {
	Nats                   NatsConfig    `yaml:"nats,omitempty"`
	NatsClientPingInterval time.Duration `yaml:"nats_client_ping_interval,omitempty"`
}

type NatsConfig struct {
	Hosts                 []NatsHost       `yaml:"hosts"`
	User                  string           `yaml:"user"`
	Pass                  string           `yaml:"pass"`
	TLSEnabled            bool             `yaml:"tls_enabled"`
	CACerts               string           `yaml:"ca_certs"`
	CAPool                *x509.CertPool   `yaml:"-"`
	ClientAuthCertificate tls.Certificate  `yaml:"-"`
	TLSPem                `yaml:",inline"` // embed to get cert_chain and private_key for client authentication
}

type NatsHost struct {
	Hostname string
	Port     uint16
}

type TLSPem struct {
	CertChain  string `yaml:"cert_chain"`
	PrivateKey string `yaml:"private_key"`
}

func loadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	config := &Config{}

	if err := yaml.NewDecoder(file).Decode(config); err != nil {
		return nil, err
	}

	if config.Nats.TLSEnabled {
		certificate, err := tls.X509KeyPair([]byte(config.Nats.CertChain), []byte(config.Nats.PrivateKey))
		if err != nil {
			errMsg := fmt.Sprintf("Error loading NATS key pair: %s", err.Error())
			return nil, fmt.Errorf(errMsg)
		}
		config.Nats.ClientAuthCertificate = certificate

		certPool := x509.NewCertPool()
		if ok := certPool.AppendCertsFromPEM([]byte(config.Nats.CACerts)); !ok {
			return nil, fmt.Errorf("Error while adding CACerts to NATs cert pool: \n%s\n", config.Nats.CACerts)
		}
		config.Nats.CAPool = certPool
	}

	return config, nil
}

func (c *Config) NatsServers() []string {
	var natsServers []string
	for _, host := range c.Nats.Hosts {
		uri := url.URL{
			Scheme: "nats",
			User:   url.UserPassword(c.Nats.User, c.Nats.Pass),
			Host:   fmt.Sprintf("%s:%d", host.Hostname, host.Port),
		}
		natsServers = append(natsServers, uri.String())
	}

	return natsServers
}
