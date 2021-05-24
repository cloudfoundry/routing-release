package config

import (
	"errors"
	"io/ioutil"

	yaml "gopkg.in/yaml.v2"
)

type RoutingAPIConfig struct {
	URI          string `yaml:"uri"`
	Port         int    `yaml:"port"`
	AuthDisabled bool   `yaml:"auth_disabled"`

	ClientCertificatePath string `yaml:"client_cert_path"`
	ClientPrivateKeyPath  string `yaml:"client_private_key_path"`
	CACertificatePath     string `yaml:"ca_cert_path"`
}

type OAuthConfig struct {
	TokenEndpoint     string `yaml:"token_endpoint"`
	Port              int    `yaml:"port"`
	SkipSSLValidation bool   `yaml:"skip_ssl_validation"`
	ClientName        string `yaml:"client_name"`
	ClientSecret      string `yaml:"client_secret"`
	CACerts           string `yaml:"ca_certs"`
}

type Config struct {
	OAuth                        OAuthConfig      `yaml:"oauth"`
	RoutingAPI                   RoutingAPIConfig `yaml:"routing_api"`
	HaProxyPidFile               string           `yaml:"haproxy_pid_file"`
	IsolationSegments            []string         `yaml:"isolation_segments"`
	ReservedSystemComponentPorts []int            `yaml:"reserved_system_component_ports"`
}

func New(path string) (*Config, error) {
	c := &Config{}
	err := c.initConfigFromFile(path)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) initConfigFromFile(path string) error {
	var e error

	b, e := ioutil.ReadFile(path)
	if e != nil {
		return e
	}

	yaml.Unmarshal(b, &c)

	if c.HaProxyPidFile == "" {
		return errors.New("haproxy_pid_file is required")
	}
	return nil
}
