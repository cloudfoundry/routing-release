package testrunner

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"time"

	"code.cloudfoundry.org/locket"
	routingAPI "code.cloudfoundry.org/routing-api"
	"code.cloudfoundry.org/routing-api/config"
	"code.cloudfoundry.org/routing-api/models"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

type RoutingAPITestConfig struct {
	Port                 uint16
	StatsdPort           uint16
	UAAPort              uint16
	AdminPort            uint16
	LocketConfig         locket.ClientLocketConfig
	CACertsPath          string
	Schema               string
	UseSQL               bool
	APIServerHTTPEnabled bool
	APIServerMTLSPort    uint16
	APIServerCertPath    string
	APIServerKeyPath     string
	APICAPath            string
}

func GetRoutingAPITestConfig(
	routingAPIPort uint16,
	routingAPIAdminPort uint16,
	routingAPImTLSPort uint16,
	oAuthServerPort uint16,
	uaaCACertsPath string,
	databaseName string,
	mTLSAPIServerCertPath string,
	mTLSAPIServerKeyPath string,
	apiCAPath string,
	locketConfig locket.ClientLocketConfig,
) RoutingAPITestConfig {
	return RoutingAPITestConfig{
		APIServerHTTPEnabled: true,
		Port:                 routingAPIPort,
		// #nosec G115 -if we have negative or >65k parallel processes for testing, we have a serious problem
		StatsdPort:        StatsdPort + uint16(ginkgo.GinkgoParallelProcess()),
		AdminPort:         routingAPIAdminPort,
		UAAPort:           oAuthServerPort,
		CACertsPath:       uaaCACertsPath,
		Schema:            databaseName,
		UseSQL:            true,
		LocketConfig:      locketConfig,
		APIServerMTLSPort: routingAPImTLSPort,
		APIServerCertPath: mTLSAPIServerCertPath,
		APIServerKeyPath:  mTLSAPIServerKeyPath,
		APICAPath:         apiCAPath,
	}
}

func GetRoutingAPIConfig(testConfig RoutingAPITestConfig) *config.Config {
	routingAPIConfig := &config.Config{
		API: config.APIConfig{
			ListenPort:         testConfig.Port,
			HTTPEnabled:        testConfig.APIServerHTTPEnabled,
			MTLSListenPort:     testConfig.APIServerMTLSPort,
			MTLSClientCAPath:   testConfig.APICAPath,
			MTLSServerCertPath: testConfig.APIServerCertPath,
			MTLSServerKeyPath:  testConfig.APIServerKeyPath,
		},
		AdminPort:                       testConfig.AdminPort,
		DebugAddress:                    "1.2.3.4:1234",
		LogGuid:                         "my_logs",
		MetronConfig:                    MetronConfig,
		SystemDomain:                    SystemDomain,
		MetricsReportingIntervalString:  MetricsReportingIntervalString,
		StatsdEndpoint:                  fmt.Sprintf("%s:%d", Host, testConfig.StatsdPort),
		StatsdClientFlushIntervalString: StatsdClientFlushIntervalString,
		OAuth: config.OAuthConfig{
			TokenEndpoint:     "127.0.0.1",
			Port:              testConfig.UAAPort,
			SkipSSLValidation: false,
			CACerts:           testConfig.CACertsPath,
		},
		RouterGroups: models.RouterGroups{
			models.RouterGroup{
				Name:            "default-tcp",
				Type:            "tcp",
				ReservablePorts: "1024-65535",
			},
		},
		RetryInterval: 50 * time.Millisecond,
		UUID:          "fake-uuid",
		Locket:        testConfig.LocketConfig,
	}
	switch Database {
	case Postgres:
		routingAPIConfig.SqlDB = config.SqlDB{
			Type:              Postgres,
			Host:              Host,
			Port:              PostgresPort,
			Schema:            testConfig.Schema,
			Username:          PostgresUsername,
			Password:          PostgresPassword,
			CACert:            os.Getenv("SQL_SERVER_CA_CERT"),
			SkipSSLValidation: os.Getenv("DB_SKIP_SSL_VALIDATION") == "true",
		}
	default:
		routingAPIConfig.SqlDB = config.SqlDB{
			Type:              MySQL,
			Host:              Host,
			Port:              MySQLPort,
			Schema:            testConfig.Schema,
			Username:          MySQLUserName,
			Password:          MySQLPassword,
			CACert:            os.Getenv("SQL_SERVER_CA_CERT"),
			SkipSSLValidation: os.Getenv("DB_SKIP_SSL_VALIDATION") == "true",
		}
	}

	return routingAPIConfig
}

func RoutingApiClientWithPort(routingAPIPort uint16, routingAPIIP string) routingAPI.Client {
	routingAPIAddress := fmt.Sprintf("%s:%d", routingAPIIP, routingAPIPort)

	routingAPIURL := &url.URL{
		Scheme: "http",
		Host:   routingAPIAddress,
	}

	return routingAPI.NewClient(routingAPIURL.String(), false)
}

func RoutingAPISession(routingAPIBinPath string, args ...string) *gexec.Session {
	session, err := gexec.Start(exec.Command(routingAPIBinPath, args...), ginkgo.GinkgoWriter, ginkgo.GinkgoWriter)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	return session
}
