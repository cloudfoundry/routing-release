package config

import (
	"fmt"
	"io/ioutil"
	"time"

	"gopkg.in/yaml.v2"
)

type HealthCheckEndpoint struct {
	Socket   string `yaml:"socket"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Path     string `yaml:"path"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

var DefaultConfig = Config{
	HealthCheckPollInterval:    10 * time.Second,
	HealthCheckTimeout:         5 * time.Second,
	StartResponseDelayInterval: 5 * time.Second,
	StartupDelayBuffer:         5 * time.Second,
	LogLevel:                   "info",
}

type Config struct {
	ComponentName              string              `yaml:"component_name"`
	FailureCounterFile           string              `yaml:"failure_counter_file"`
	HealthCheckEndpoint        HealthCheckEndpoint `yaml:"healthcheck_endpoint"`
	HealthCheckPollInterval    time.Duration       `yaml:"healthcheck_poll_interval"`
	HealthCheckTimeout         time.Duration       `yaml:"healthcheck_timeout"`
	StartResponseDelayInterval time.Duration       `yaml:"start_response_delay_interval"`
	StartupDelayBuffer         time.Duration       `yaml:"startup_delay_buffer"`
	LogLevel                   string              `yaml:"log_level"`
}

func LoadConfig(configFile string) (Config, error) {
	b, err := ioutil.ReadFile(configFile)
	if err != nil {
		return Config{}, fmt.Errorf("Could not read config file: %s, err: %s", configFile, err.Error())
	}
	var c Config
	err = yaml.Unmarshal(b, &c)
	if err != nil {
		return Config{}, fmt.Errorf("Could not unmarshal config file: %s, err: %s", configFile, err.Error())
	}

	err = c.Validate()
	if err != nil {
		return Config{}, fmt.Errorf("Failed to validate config file: %s, err: %s", configFile, err.Error())
	}

	c.ApplyDefaults()

	return c, nil
}

func (c *Config) ApplyDefaults() {
	if c.HealthCheckPollInterval == 0 {
		c.HealthCheckPollInterval = DefaultConfig.HealthCheckPollInterval
	}

	if c.HealthCheckTimeout == 0 {
		c.HealthCheckTimeout = DefaultConfig.HealthCheckTimeout
	}

	if c.StartResponseDelayInterval == 0 {
		c.StartResponseDelayInterval = DefaultConfig.StartResponseDelayInterval
	}

	if c.StartupDelayBuffer == 0 {
		c.StartupDelayBuffer = DefaultConfig.StartupDelayBuffer
	}

	if c.LogLevel == "" {
		c.LogLevel = DefaultConfig.LogLevel
	}
}

func (c *Config) Validate() error {
	if c.ComponentName == "" {
		return fmt.Errorf("Missing component_name")
	}

	if c.HealthCheckEndpoint.Socket == "" {
		if c.HealthCheckEndpoint.Host == "" {
			return fmt.Errorf("Missing healthcheck endpoint host or socket")
		}
		if c.HealthCheckEndpoint.Port == 0 {
			return fmt.Errorf("Missing healthcheck endpoint port or socket")
		}
	} else {
		if c.HealthCheckEndpoint.Host != "" {
			return fmt.Errorf("Cannot specify both healthcheck endpoint host and socket")
		}
		if c.HealthCheckEndpoint.Port != 0 {
			return fmt.Errorf("Cannot specify both healthcheck endpoint port and socket")
		}
	}

	if c.FailureCounterFile == "" {
		return fmt.Errorf("Missing failure_counter_file")
	}
	return nil
}
