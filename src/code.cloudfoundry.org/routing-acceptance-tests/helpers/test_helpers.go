package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/routing-api/uaaclient"

	"github.com/cloudfoundry/cf-test-helpers/v2/cf"
	"github.com/cloudfoundry/cf-test-helpers/v2/config"
	cfworkflow_helpers "github.com/cloudfoundry/cf-test-helpers/v2/workflowhelpers"
	uuid "github.com/nu7hatch/gouuid"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

type RoutingConfig struct {
	*config.Config
	RoutingApiUrl     string       `json:"-"` //"-" is used for ignoring field
	Addresses         []string     `json:"addresses"`
	OAuth             *OAuthConfig `json:"oauth"`
	IncludeHttpRoutes bool         `json:"include_http_routes"`
	TcpAppDomain      string       `json:"tcp_apps_domain"`
	LBConfigured      bool         `json:"lb_configured"`
	TCPRouterGroup    string       `json:"tcp_router_group"`
}

type OAuthConfig struct {
	TokenEndpoint string `json:"token_endpoint"`
	ClientName    string `json:"client_name"`
	ClientSecret  string `json:"client_secret"`
	Port          uint16 `json:"port"`
}

func loadDefaultTimeout(conf *RoutingConfig) {
	if conf.DefaultTimeout <= 0 {
		conf.DefaultTimeout = 120
	}

	if conf.CfPushTimeout <= 0 {
		conf.CfPushTimeout = 120
	}
}
func LoadConfig() RoutingConfig {
	loadedConfig := loadConfigJsonFromPath()

	loadedConfig.Config = config.LoadConfig()
	loadDefaultTimeout(&loadedConfig)

	if loadedConfig.OAuth == nil {
		panic("missing configuration oauth")
	}

	if len(loadedConfig.Addresses) == 0 {
		panic("missing configuration 'addresses'")
	}

	if loadedConfig.AppsDomain == "" {
		panic("missing configuration apps_domain")
	}

	if loadedConfig.ApiEndpoint == "" {
		panic("missing configuration api")
	}

	if loadedConfig.TCPRouterGroup == "" {
		panic("missing configuration tcp_router_group")
	}

	loadedConfig.RoutingApiUrl = fmt.Sprintf("https://%s", loadedConfig.ApiEndpoint)

	return loadedConfig
}

func ValidateRouterGroupName(context cfworkflow_helpers.UserContext, tcpRouterGroup string) {
	var routerGroupOutput string
	cfworkflow_helpers.AsUser(context, context.Timeout, func() {
		routerGroupOutput = string(cf.Cf("router-groups").Wait(context.Timeout).Out.Contents())
	})

	Expect(routerGroupOutput).To(MatchRegexp(fmt.Sprintf("%s\\s+tcp", tcpRouterGroup)), fmt.Sprintf("Router group %s of type tcp doesn't exist", tcpRouterGroup))
}

func NewTokenFetcher(routerApiConfig RoutingConfig, logger lager.Logger) uaaclient.TokenFetcher {
	u, err := url.Parse(routerApiConfig.OAuth.TokenEndpoint)
	Expect(err).ToNot(HaveOccurred())

	cfg := uaaclient.Config{
		TokenEndpoint:     u.Host,
		Port:              routerApiConfig.OAuth.Port,
		SkipSSLValidation: routerApiConfig.SkipSSLValidation,
		ClientName:        routerApiConfig.OAuth.ClientName,
		ClientSecret:      routerApiConfig.OAuth.ClientSecret,
	}

	uaaTokenFetcher, err := uaaclient.NewTokenFetcher(false, cfg, clock.NewClock(), 3, 500*time.Millisecond, 30, logger)
	Expect(err).ToNot(HaveOccurred())

	_, err = uaaTokenFetcher.FetchToken(context.Background(), true)
	Expect(err).ToNot(HaveOccurred())

	return uaaTokenFetcher
}

func UpdateOrgQuota(context cfworkflow_helpers.UserContext) {
	err := os.Setenv("CF_TRACE", "false")
	Expect(err).NotTo(HaveOccurred())
	cfworkflow_helpers.AsUser(context, context.Timeout, func() {
		orgGuid := strings.TrimSpace(string(cf.Cf("org", context.Org, "--guid").Wait(context.Timeout).Out.Contents()))
		f, err := os.CreateTemp("", "curl-json")
		Expect(err).NotTo(HaveOccurred())
		defer f.Close()
		defer os.Remove(f.Name())
		data := []byte(`{"routes": {"total_reserved_route_ports":-1} }`)
		_, err = f.Write(data)
		Expect(err).NotTo(HaveOccurred())

		Eventually(cf.Cf("curl", fmt.Sprintf("/v3/organization_quotas/%s", orgGuid), "-X", "PATCH", "-d", fmt.Sprintf("@%s", f.Name())), context.Timeout).Should(gexec.Exit(0))
	})
}

func loadConfigJsonFromPath() RoutingConfig {
	var config RoutingConfig

	path := configPath()

	configFile, err := os.Open(path)
	if err != nil {
		panic(err)
	}

	decoder := json.NewDecoder(configFile)
	err = decoder.Decode(&config)
	if err != nil {
		panic(err)
	}

	return config
}

func configPath() string {
	path := os.Getenv("CONFIG")
	if path == "" {
		panic("Must set $CONFIG to point to an integration config .json file.")
	}

	return path
}

func RandomName() string {
	guid, err := uuid.NewV4()
	if err != nil {
		panic(err)
	}

	return guid.String()
}
