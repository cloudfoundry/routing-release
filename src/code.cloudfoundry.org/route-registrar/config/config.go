package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"code.cloudfoundry.org/multierror"
)

type MessageBusServerSchema struct {
	Host     string `json:"host"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type RoutingAPISchema struct {
	APIURL            string `json:"api_url"`
	OAuthURL          string `json:"oauth_url"`
	ClientID          string `json:"client_id"`
	ClientSecret      string `json:"client_secret"`
	CACerts           string `json:"ca_certs"`
	SkipSSLValidation bool   `json:"skip_ssl_validation"`

	ClientCertificatePath   string `json:"client_cert_path"`
	ClientPrivateKeyPath    string `json:"client_private_key_path"`
	ServerCACertificatePath string `json:"server_ca_cert_path"`
	MaxTTL                  string `json:"max_ttl,omitempty"`
}

type HealthCheckSchema struct {
	Name       string `json:"name" yaml:"name"`
	ScriptPath string `json:"script_path" yaml:"script_path"`
	Timeout    string `json:"timeout" yaml:"timeout"`
}

type ConfigSchema struct {
	MessageBusServers          []MessageBusServerSchema `json:"message_bus_servers"`
	RoutingAPI                 RoutingAPISchema         `json:"routing_api"`
	Routes                     []RouteSchema            `json:"routes"`
	DynamicConfigGlobs         []string                 `json:"dynamic_config_globs"`
	NATSmTLSConfig             ClientTLSConfigSchema    `json:"nats_mtls_config"`
	Host                       string                   `json:"host"`
	AvailabilityZone           string                   `json:"availability_zone"`
	UnregistrationMessageLimit *int                     `json:"unregistration_message_limit,omitempty"`
}

type RouteSchema struct {
	Type                 string             `json:"type" yaml:"type"`
	Name                 string             `json:"name" yaml:"name"`
	Host                 string             `json:"host" yaml:"host"`
	Port                 *uint16            `json:"port" yaml:"port"`
	Protocol             string             `json:"protocol" yaml:"protocol"`
	SniPort              *uint16            `json:"sni_port" yaml:"sni_port"`
	TLSPort              *uint16            `json:"tls_port" yaml:"tls_port"`
	Tags                 map[string]string  `json:"tags" yaml:"tags"`
	URIs                 []string           `json:"uris" yaml:"uris"`
	RouterGroup          string             `json:"router_group" yaml:"router_group"`
	ExternalPort         *uint16            `json:"external_port,omitempty" yaml:"external_port,omitempty"`
	RouteServiceUrl      string             `json:"route_service_url" yaml:"route_service_url"`
	RegistrationInterval string             `json:"registration_interval,omitempty" yaml:"registration_interval,omitempty"`
	HealthCheck          *HealthCheckSchema `json:"health_check,omitempty" yaml:"health_check,omitempty"`
	ServerCertDomainSAN  string             `json:"server_cert_domain_san,omitempty" yaml:"server_cert_domain_san,omitempty"`
	SniRoutableSan       string             `json:"sni_routable_san,omitempty" yaml:"sni_routable_san,omitempty"`
	SniRewriteSan        string             `json:"sni_rewrite_san,omitempty" yaml:"sni_rewrite_san,omitempty"`
	TerminateFrontendTLS bool               `json:"terminate_frontend_tls,omitempty" yaml:"terminate_frontend_tls,omitempty"`
	EnableBackendTLS     bool               `json:"enable_backend_tls,omitempty" yaml:"enable_backend_tls,omitempty"`
	ALPNs                []string           `json:"alpns,omitempty" yaml:"alpns,omitempty"`
	Options              *Options           `json:"options,omitempty" yaml:"options,omitempty"`
}

type Options struct {
	LoadBalancingAlgorithm LoadBalancingAlgorithm `json:"loadbalancing,omitempty" yaml:"loadbalancing,omitempty"`
}

type LoadBalancingAlgorithm string

var supportedLoadBalancingAlgorithms = []LoadBalancingAlgorithm{RoundRobin, LeastConns}

const (
	RoundRobin LoadBalancingAlgorithm = "round-robin"
	LeastConns LoadBalancingAlgorithm = "least-connection"
)

type ClientTLSConfigSchema struct {
	Enabled  bool   `json:"enabled"`
	CertPath string `json:"cert_path"`
	KeyPath  string `json:"key_path"`
	CAPath   string `json:"ca_path"`
}

type MessageBusServer struct {
	Host     string
	User     string
	Password string
}

type RoutingAPI struct {
	APIURL            string
	OAuthURL          string
	ClientID          string
	ClientSecret      string
	CACerts           string
	SkipSSLValidation bool

	ClientCertificatePath   string
	ClientPrivateKeyPath    string
	ServerCACertificatePath string

	MaxTTL time.Duration
}

type HealthCheck struct {
	Name       string
	ScriptPath string
	Timeout    time.Duration
}

type Config struct {
	MessageBusServers          []MessageBusServer
	RoutingAPI                 RoutingAPI
	Routes                     []Route
	DynamicConfigGlobs         []string
	NATSmTLSConfig             ClientTLSConfig
	Host                       string
	AvailabilityZone           string `json:"availability_zone"`
	UnregistrationMessageLimit int
}

type ClientTLSConfig struct {
	Enabled  bool
	CertPath string
	KeyPath  string
	CAPath   string
}

type Route struct {
	Type                 string
	Name                 string
	Port                 *uint16
	Protocol             string
	TLSPort              *uint16
	Tags                 map[string]string
	URIs                 []string
	RouterGroup          string
	Host                 string
	ExternalPort         *uint16
	RouteServiceUrl      string
	RegistrationInterval time.Duration
	HealthCheck          *HealthCheck
	ServerCertDomainSAN  string
	SniRewriteSan        string
	TerminateFrontendTLS bool
	ALPNs                []string
	EnableBackendTLS     bool
	Options              *Options
}

func NewConfigSchemaFromFile(configFile string) (ConfigSchema, error) {
	var config ConfigSchema

	c, err := os.ReadFile(configFile)
	if err != nil {
		return ConfigSchema{}, err
	}

	err = json.Unmarshal(c, &config)
	if err != nil {
		return ConfigSchema{}, err
	}

	return config, nil
}

func (c ConfigSchema) ParseSchemaAndSetDefaultsToConfig() (*Config, error) {
	errors := multierror.NewMultiError("config")

	if c.UnregistrationMessageLimit == nil {
		defaultUnregistrationLimit := 5
		c.UnregistrationMessageLimit = &defaultUnregistrationLimit
	}

	if *c.UnregistrationMessageLimit <= 0 {
		errors.Add(fmt.Errorf("unregistration_message_limit must be a positive integer"))
	}

	tcp_routes := 0

	routes := []Route{}
	for index, r := range c.Routes {
		route, err := RouteFromSchema(r, index, c.Host)
		if err != nil {
			errors.Add(err)
			continue
		}

		if route.Type == "tcp" {
			tcp_routes++
		}

		routes = append(routes, *route)
	}

	messageBusServers, err := messageBusServersFromSchema(c.MessageBusServers)
	if err != nil && (len(routes)-tcp_routes > 0) {
		errors.Add(err)
	}

	routingAPI, err := routingAPIFromSchema(c.RoutingAPI)
	if err != nil && tcp_routes > 0 {
		errors.Add(err)
	}

	if errors.Length() > 0 {
		return nil, errors
	}

	natsTLSConfig := clientTLSConfigFromSchema(c.NATSmTLSConfig)

	config := Config{
		Host:                       c.Host,
		AvailabilityZone:           c.AvailabilityZone,
		UnregistrationMessageLimit: *c.UnregistrationMessageLimit,
		MessageBusServers:          messageBusServers,
		Routes:                     routes,
		DynamicConfigGlobs:         c.DynamicConfigGlobs,
		NATSmTLSConfig:             natsTLSConfig,
	}
	if routingAPI != nil {
		config.RoutingAPI = *routingAPI
	}

	return &config, nil
}

func nameOrIndex(r RouteSchema, index int) string {
	if r.Name != "" {
		return fmt.Sprintf(`"%s"`, r.Name)
	}

	return strconv.Itoa(index)
}

func parseRegistrationInterval(registrationInterval string) (time.Duration, error) {
	var duration time.Duration

	if registrationInterval == "" {
		return duration, fmt.Errorf("no registration_interval")
	}

	var err error
	duration, err = time.ParseDuration(registrationInterval)
	if err != nil {
		return duration, fmt.Errorf("invalid registration_interval: %s", err.Error())
	}

	if duration <= 0 {
		return duration, fmt.Errorf("invalid registration_interval: interval must be greater than 0")
	}

	return duration, nil
}

func RouteFromSchema(r RouteSchema, index int, host string) (*Route, error) {
	errors := multierror.NewMultiError(fmt.Sprintf("route %s", nameOrIndex(r, index)))

	if r.Type != "tcp" && r.Type != "sni" && r.Name == "" {
		errors.Add(fmt.Errorf("no name"))
	}

	if r.Host == "" {
		if host == "" {
			errors.Add(fmt.Errorf("no host"))
		} else {
			r.Host = host
		}
	}

	if r.Port == nil && r.TLSPort == nil && r.SniPort == nil {
		errors.Add(fmt.Errorf("no port"))
	}
	if r.Port != nil && *r.Port <= 0 {
		errors.Add(fmt.Errorf("invalid port: %d", *r.Port))
	}
	if r.TLSPort != nil && *r.TLSPort <= 0 {
		errors.Add(fmt.Errorf("invalid tls_port: %d", *r.TLSPort))
	}

	if r.Type != "tcp" && r.Type != "sni" {
		if len(r.URIs) == 0 {
			errors.Add(fmt.Errorf("no URIs"))
		}

		for _, u := range r.URIs {
			if u == "" {
				errors.Add(fmt.Errorf("empty URIs"))
				break
			}
		}

		_, err := url.Parse(r.RouteServiceUrl)
		if err != nil {
			errors.Add(err)
		}
	} else {
		if r.RouterGroup == "" {
			errors.Add(fmt.Errorf("missing router_group"))
		}
		if r.ExternalPort != nil && *r.ExternalPort <= 0 {
			errors.Add(fmt.Errorf("invalid port: %d", *r.ExternalPort))
		}
	}

	if r.Protocol != "" && r.Protocol != "http1" && r.Protocol != "http2" {
		errors.Add(fmt.Errorf("unknown protocol: %s", r.Protocol))
	}

	if r.Options != nil {
		if r.Options.LoadBalancingAlgorithm != "" {
			err := validatePerRouteLoadBalancingAlgorithm(r.Options.LoadBalancingAlgorithm)
			if err != nil {
				errors.Add(err)
			}
		}
	}

	registrationInterval, err := parseRegistrationInterval(r.RegistrationInterval)
	if err != nil {
		errors.Add(err)
	}

	var healthCheck *HealthCheck
	if r.HealthCheck != nil {
		healthCheck, err = healthCheckFromSchema(r.HealthCheck, registrationInterval)
		if err != nil {
			errors.Add(err)
		}
	}

	if errors.Length() > 0 {
		return nil, errors
	}

	route := Route{
		Type:                 r.Type,
		Name:                 r.Name,
		Host:                 r.Host,
		Port:                 r.Port,
		Protocol:             r.Protocol,
		TLSPort:              r.TLSPort,
		Tags:                 r.Tags,
		URIs:                 r.URIs,
		RouterGroup:          r.RouterGroup,
		ExternalPort:         r.ExternalPort,
		RouteServiceUrl:      r.RouteServiceUrl,
		ServerCertDomainSAN:  r.ServerCertDomainSAN,
		SniRewriteSan:        r.SniRewriteSan,
		RegistrationInterval: registrationInterval,
		HealthCheck:          healthCheck,
		TerminateFrontendTLS: r.TerminateFrontendTLS,
		ALPNs:                r.ALPNs,
		EnableBackendTLS:     r.EnableBackendTLS,
		Options:              r.Options,
	}

	if r.Type == "sni" {
		route.Port = r.SniPort
		route.ServerCertDomainSAN = r.SniRoutableSan
		route.Type = "tcp"
	}
	return &route, nil
}

func (r *Route) GetALPNs() string {
	slices.Sort(r.ALPNs)
	return strings.Join(r.ALPNs, ",")
}

func validatePerRouteLoadBalancingAlgorithm(loadBalancingAlgo LoadBalancingAlgorithm) error {
	for _, lbAlgo := range supportedLoadBalancingAlgorithms {
		if loadBalancingAlgo == lbAlgo {
			return nil
		}
	}
	return fmt.Errorf("unknown load balancing algorithm: %s. Allowed values: %s", loadBalancingAlgo, supportedLoadBalancingAlgorithms)
}

func healthCheckFromSchema(
	healthCheckSchema *HealthCheckSchema,
	registrationInterval time.Duration,
) (*HealthCheck, error) {
	errors := multierror.NewMultiError("healthcheck")

	healthCheck := &HealthCheck{
		Name:       healthCheckSchema.Name,
		ScriptPath: healthCheckSchema.ScriptPath,
	}

	if healthCheck.Name == "" {
		errors.Add(fmt.Errorf("no name"))
	}

	if healthCheck.ScriptPath == "" {
		errors.Add(fmt.Errorf("no script_path"))
	}

	if healthCheckSchema.Timeout == "" && registrationInterval > 0 {
		if errors.Length() > 0 {
			return nil, errors
		}

		healthCheck.Timeout = registrationInterval / 2
		return healthCheck, nil
	}

	var err error
	healthCheck.Timeout, err = time.ParseDuration(healthCheckSchema.Timeout)
	if err != nil {
		errors.Add(fmt.Errorf("invalid healthcheck timeout: %s", err.Error()))
		return nil, errors
	}

	if healthCheck.Timeout <= 0 {
		errors.Add(fmt.Errorf("invalid healthcheck timeout: %s", healthCheck.Timeout))
		return nil, errors
	}

	if healthCheck.Timeout >= registrationInterval && registrationInterval > 0 {
		errors.Add(fmt.Errorf(
			"invalid healthcheck timeout: %v must be less than the registration interval: %v",
			healthCheck.Timeout,
			registrationInterval,
		))
		return nil, errors
	}

	if errors.Length() > 0 {
		return nil, errors
	}

	return healthCheck, nil
}

func messageBusServersFromSchema(servers []MessageBusServerSchema) ([]MessageBusServer, error) {
	messageBusServers := []MessageBusServer{}
	if len(servers) < 1 {
		return nil, fmt.Errorf("message_bus_servers must have at least one entry")
	}

	for _, m := range servers {
		messageBusServers = append(
			messageBusServers,
			MessageBusServer(m),
		)
	}

	return messageBusServers, nil
}

func parseMaxTTL(max_ttl string) time.Duration {
	ttl, _ := time.ParseDuration(max_ttl)
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}

	return ttl
}

func routingAPIFromSchema(api RoutingAPISchema) (*RoutingAPI, error) {
	if api.APIURL == "" {
		return nil, fmt.Errorf("routing_api must have an api_url")
	}

	apiURL, err := url.Parse(api.APIURL)

	if err != nil {
		return nil, fmt.Errorf("routing_api must a vaid URL")
	}

	if api.OAuthURL == "" {
		return nil, fmt.Errorf("routing_api must have an oauth_url")
	}
	if api.ClientID == "" {
		return nil, fmt.Errorf("routing_api must have a client_id")
	}
	if api.ClientSecret == "" {
		return nil, fmt.Errorf("routing_api must have a client_secret")
	}

	if apiURL.Scheme == "https" {
		if api.ClientCertificatePath == "" {
			return nil, fmt.Errorf("routing_api must have a client_certificate_path")
		}
		if api.ClientPrivateKeyPath == "" {
			return nil, fmt.Errorf("routing_api must have a client_private_key_path")
		}
		if api.ServerCACertificatePath == "" {
			return nil, fmt.Errorf("routing_api must have a server_ca_cert_path")
		}
	}

	maxTTL := parseMaxTTL(api.MaxTTL)

	return &RoutingAPI{
		APIURL:                  api.APIURL,
		OAuthURL:                api.OAuthURL,
		ClientID:                api.ClientID,
		ClientSecret:            api.ClientSecret,
		CACerts:                 api.CACerts,
		SkipSSLValidation:       api.SkipSSLValidation,
		ClientCertificatePath:   api.ClientCertificatePath,
		ClientPrivateKeyPath:    api.ClientPrivateKeyPath,
		ServerCACertificatePath: api.ServerCACertificatePath,
		MaxTTL:                  maxTTL,
	}, nil
}

func clientTLSConfigFromSchema(clientTLSConfigSchema ClientTLSConfigSchema) ClientTLSConfig {
	return ClientTLSConfig(clientTLSConfigSchema)
}
