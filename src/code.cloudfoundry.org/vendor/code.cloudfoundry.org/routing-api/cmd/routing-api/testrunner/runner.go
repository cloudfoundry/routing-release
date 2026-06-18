package testrunner

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"code.cloudfoundry.org/locket"
	"code.cloudfoundry.org/locket/cmd/locket/testrunner"
	"code.cloudfoundry.org/routing-api/config"
	"code.cloudfoundry.org/routing-api/models"
	"code.cloudfoundry.org/routing-api/test_helpers"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
	"gopkg.in/yaml.v3"
)

type Args struct {
	ConfigPath string
	DevMode    bool
	IP         string
}

func (args Args) ArgSlice() []string {
	return []string{
		"-ip", args.IP,
		"-config", args.ConfigPath,
		"-logLevel=debug",
		"-devMode=" + strconv.FormatBool(args.DevMode),
	}
}

func (args Args) Port() uint16 {
	cfg, err := config.NewConfigFromFile(args.ConfigPath, true)
	if err != nil {
		panic(err.Error())
	}

	return uint16(cfg.API.ListenPort)
}

func NewDbAllocator() DbAllocator {
	var dbAllocator DbAllocator
	switch Database {
	case Postgres:
		dbAllocator = NewPostgresAllocator()
	default:
		dbAllocator = NewMySQLAllocator()
	}
	return dbAllocator
}

func NewRoutingAPIArgs(
	ip string,
	port uint16,
	mtlsPort uint16,
	dbId string,
	dbCACert string,
	locketAddr string,
	mtlsClientCAPath string,
	mtlsServerCertPath string,
	mtlsServerKeyPath string,
) (Args, error) {
	configPath, err := createConfig(
		port,
		mtlsPort,
		dbId,
		dbCACert,
		locketAddr,
		mtlsClientCAPath,
		mtlsServerCertPath,
		mtlsServerKeyPath,
	)
	if err != nil {
		return Args{}, err
	}
	return Args{
		IP:         ip,
		ConfigPath: configPath,
		DevMode:    true,
	}, nil
}

func New(binPath string, args Args) *ginkgomon.Runner {
	cmd := exec.Command(binPath, args.ArgSlice()...)
	return ginkgomon.New(ginkgomon.Config{
		Name:              "routing-api",
		Command:           cmd,
		StartCheck:        "routing-api.started",
		StartCheckTimeout: 30 * time.Second,
	})
}

func createConfig(
	port uint16,
	mtlsPort uint16,
	dbId string,
	dbCACert string,
	locketAddr string,
	mtlsClientCAPath string,
	mtlsServerCertPath string,
	mtlsServerKeyPath string,
) (string, error) {
	adminPort := test_helpers.NextAvailPort()
	locketConfig := testrunner.ClientLocketConfig()

	routingAPIConfig := config.Config{
		LogGuid: "my_logs",
		UUID:    "routing-api-uuid",
		Locket: locket.ClientLocketConfig{
			LocketAddress:        locketAddr,
			LocketCACertFile:     locketConfig.LocketCACertFile,
			LocketClientCertFile: locketConfig.LocketClientCertFile,
			LocketClientKeyFile:  locketConfig.LocketClientKeyFile,
		},
		MetronConfig: MetronConfig,
		API: config.APIConfig{
			ListenPort:         port,
			HTTPEnabled:        true,
			MTLSListenPort:     mtlsPort,
			MTLSClientCAPath:   mtlsClientCAPath,
			MTLSServerCertPath: mtlsServerCertPath,
			MTLSServerKeyPath:  mtlsServerKeyPath,
		},
		MetricsReportingIntervalString:  MetricsReportingIntervalString,
		StatsdEndpoint:                  fmt.Sprintf("%s:%d", Host, StatsdPort),
		StatsdClientFlushIntervalString: StatsdClientFlushIntervalString,
		SystemDomain:                    SystemDomain,
		AdminPort:                       adminPort,
		RouterGroups: models.RouterGroups{
			{
				Name:            "default-tcp",
				Type:            "tcp",
				ReservablePorts: "1024-65535",
			},
		},
		RetryInterval: 50 * time.Millisecond,
	}

	switch Database {
	case Postgres:
		routingAPIConfig.SqlDB = config.SqlDB{
			Type:     Postgres,
			Username: PostgresUsername,
			Password: PostgresPassword,
			Schema:   dbId,
			Port:     PostgresPort,
			Host:     Host,
			CACert:   dbCACert,
		}
	default:
		routingAPIConfig.SqlDB = config.SqlDB{
			Type:     MySQL,
			Username: MySQLUserName,
			Password: MySQLPassword,
			Schema:   dbId,
			Port:     MySQLPort,
			Host:     Host,
			CACert:   dbCACert,
		}
	}

	routingAPIConfigBytes, err := yaml.Marshal(routingAPIConfig)
	if err != nil {
		return "", err
	}

	configFile, err := os.CreateTemp("", "routing-api-config")
	if err != nil {
		return "", err
	}
	if err := configFile.Close(); err != nil {
		return "", err
	}
	configFilePath := configFile.Name()

	err = os.WriteFile(configFilePath, routingAPIConfigBytes, 0644)
	return configFilePath, err
}
