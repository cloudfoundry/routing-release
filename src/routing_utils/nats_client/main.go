package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"code.cloudfoundry.org/tlsconfig"
	"github.com/nats-io/nats.go"
	"gopkg.in/yaml.v2"
)

const USAGE = `Usage:
/var/vcap/jobs/gorouter/bin/nats_client [COMMAND]

COMMANDS:
  sub          [SUBJECT] [MESSAGE] 
               (Default) Streams NATS messages from server with provided SUBJECT. Default SUBJECT is 'router.*'
               Example: /var/vcap/jobs/gorouter/bin/nats_client sub 'router.*'

  pub          [SUBJECT] [MESSAGE]
               Publish the provided message JSON to SUBJECT subscription. SUBJECT and MESSAGE are required
               Example: /var/vcap/jobs/gorouter/bin/nats_client pub router.register '{"host":"172.217.6.68","port":80,"uris":["bar.example.com"]}'

  save         <FILE> 
               Save this gorouter's route table to a json file.
               Example: /var/vcap/jobs/gorouter/bin/nats_client save routes.json'

  load         <FILE>
               Load routes from a json file into this gorouter.
               Example: /var/vcap/jobs/gorouter/bin/nats_client load routes.json'
`

// Simple NATS client for debugging
// Uses gorouter.yml for config
func main() {
	//TODO: use a proper arg parser here
	if len(os.Args) < 2 || os.Args[len(os.Args)-1] == "--help" || os.Args[len(os.Args)-1] == "-h" || os.Args[len(os.Args)-1] == "help" {
		fmt.Println(USAGE)
		os.Exit(1)
	}

	configPath := os.Args[1]
	command := "sub"
	if len(os.Args) >= 3 {
		command = os.Args[2]
	}
	if command != "sub" && command != "pub" && command != "save" && command != "load" {
		fmt.Println(USAGE)
		os.Exit(1)
	}

	subject := "router.*"
	if len(os.Args) >= 4 {
		subject = os.Args[3]
	}

	var message string
	if command == "pub" {
		if len(os.Args) >= 5 {
			message = os.Args[4]
		} else {
			fmt.Println(USAGE)
			os.Exit(1)
		}
	}

	var filename string
	if command == "save" || command == "load" {
		if len(os.Args) >= 4 {
			filename = os.Args[3]
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
	defer natsConn.Close()

	if command == "pub" {
		fmt.Fprintf(os.Stderr, "Publishing message to %s\n", subject)
		err := natsConn.Publish(subject, []byte(message))
		if err != nil {
			panic(err)
		}
		fmt.Fprintln(os.Stderr, "Done")
	}

	if command == "sub" {
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

	if command == "save" {
		fmt.Fprintf(os.Stderr, "Saving route table to %s\n", filename)
		err := dumpRoutes(config, filename)
		if err != nil {
			panic(err)
		}
		fmt.Fprintln(os.Stderr, "Done")
	}

	if command == "load" {
		fmt.Fprintf(os.Stderr, "Loading route table from %s\n", filename)
		err := loadRoutes(natsConn, filename)
		if err != nil {
			panic(err)
		}
		fmt.Fprintln(os.Stderr, "Done")
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
	Status                 StatusConfig  `yaml:"status,omitempty"`
	Nats                   NatsConfig    `yaml:"nats,omitempty"`
	NatsClientPingInterval time.Duration `yaml:"nats_client_ping_interval,omitempty"`
}

type StatusConfig struct {
	Host string `yaml:"host"`
	Port uint16 `yaml:"port"`
	User string `yaml:"user"`
	Pass string `yaml:"pass"`
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

func dumpRoutes(config *Config, filename string) error {
	res, err := http.Get(fmt.Sprintf("http://%s:%s@%s:%d/routes", config.Status.User, config.Status.Pass, config.Status.Host, config.Status.Port))

	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code from /routes: %s", res.Status)
	}
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	var jsonObject map[string]interface{}
	dataIn, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(dataIn, &jsonObject)
	if err != nil {
		return err
	}
	// Pretty print json so that humans can change it.
	dataOut, err := json.MarshalIndent(jsonObject, "", "  ")
	if err != nil {
		return err
	}
	_, err = file.Write(dataOut)
	if err != nil {
		return err
	}

	return err
}

// From src/code.cloudfoundry.org/gorouter/mbus/subscriber.go
type RegistryMessage struct {
	Host                    string            `json:"host"`
	Port                    int               `json:"port"`
	Protocol                string            `json:"protocol"`
	TLSPort                 int               `json:"tls_port"`
	Uris                    []string          `json:"uris"`
	Tags                    map[string]string `json:"tags"`
	App                     string            `json:"app"`
	StaleThresholdInSeconds int               `json:"stale_threshold_in_seconds"`
	RouteServiceURL         string            `json:"route_service_url"`
	PrivateInstanceID       string            `json:"private_instance_id"`
	ServerCertDomainSAN     string            `json:"server_cert_domain_san"`
	PrivateInstanceIndex    string            `json:"private_instance_index"`
	IsolationSegment        string            `json:"isolation_segment"`
	EndpointUpdatedAtNs     int64             `json:"endpoint_updated_at_ns"`
}

// From src/code.cloudfoundry.org/gorouter/route/pool.go
type RouteTableEntry struct {
	Address             string            `json:"address"`
	Protocol            string            `json:"protocol"`
	TLS                 bool              `json:"tls"`
	TTL                 int               `json:"ttl"`
	RouteServiceUrl     string            `json:"route_service_url,omitempty"`
	Tags                map[string]string `json:"tags"`
	IsolationSegment    string            `json:"isolation_segment,omitempty"`
	PrivateInstanceId   string            `json:"private_instance_id,omitempty"`
	ServerCertDomainSAN string            `json:"server_cert_domain_san,omitempty"`
}

func loadRoutes(natsConn *nats.Conn, filename string) error {
	var routeTable map[string][]RouteTableEntry

	routesFile, err := os.Open(filename)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(routesFile)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, &routeTable)
	if err != nil {
		return err
	}
	for uri, routes := range routeTable {
		for _, route := range routes {
			host := strings.Split(route.Address, ":")[0]
			port, _ := strconv.Atoi(strings.Split(route.Address, ":")[1])
			tlsPort := 0
			if route.TLS {
				tlsPort = port
			}

			msg := RegistryMessage{
				Host:                    host,
				Port:                    port,
				TLSPort:                 tlsPort,
				Protocol:                route.Protocol,
				Uris:                    []string{uri},
				Tags:                    route.Tags,
				App:                     route.Tags["app_id"],
				StaleThresholdInSeconds: route.TTL,
				PrivateInstanceID:       route.PrivateInstanceId,
				IsolationSegment:        route.IsolationSegment,
				ServerCertDomainSAN:     route.ServerCertDomainSAN,
			}
			msgData, err := json.Marshal(msg)
			if err != nil {
				return err
			}
			err = natsConn.Publish("router.register", msgData)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
