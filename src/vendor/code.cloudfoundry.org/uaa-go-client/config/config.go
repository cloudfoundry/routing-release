package config

import (
	"errors"
	"net/url"
	"time"
)

const (
	DefaultExpirationBufferInSec = 30
	DefaultRequestTimeout        = 0 * time.Second
)

type Config struct {
	UaaEndpoint                   string `yaml:"uaa_endpoint"`
	ClientName                    string `yaml:"client_name"`
	ClientSecret                  string `yaml:"client_secret"`
	CACerts                       string `yaml:"ca_certs"`
	MaxNumberOfRetries            uint32
	RetryInterval                 time.Duration
	ExpirationBufferInSec         int64
	SkipVerification              bool
	InsecureAllowAnySigningMethod bool
	RequestTimeout                time.Duration
}

func (c *Config) CheckEndpoint() (*url.URL, error) {
	if c.UaaEndpoint == "" {
		return nil, errors.New("UAA endpoint cannot be empty")
	}

	uri, err := url.Parse(c.UaaEndpoint)
	if err != nil {
		return nil, errors.New("UAA endpoint invalid")
	}
	return uri, nil
}

func (c *Config) CheckCredentials() error {

	if c.ClientName == "" {
		return errors.New("OAuth Client ID cannot be empty")
	}

	if c.ClientSecret == "" {
		return errors.New("OAuth Client Secret cannot be empty")
	}

	return nil
}
