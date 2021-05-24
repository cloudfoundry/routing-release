package testrunner

import (
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"time"

	"code.cloudfoundry.org/routing-release/cf-tcp-router/utils"
	"code.cloudfoundry.org/locket"
	"code.cloudfoundry.org/locket/cmd/locket/testrunner"
	"code.cloudfoundry.org/routing-release/routing-api/config"
	"code.cloudfoundry.org/routing-release/routing-api/models"
	"code.cloudfoundry.org/routing-release/routing-api/test_helpers"
	"github.com/tedsuo/ifrit/ginkgomon"
	yaml "gopkg.in/yaml.v2"
)

var dbEnv = os.Getenv("DB")

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
	switch dbEnv {
	case "postgres":
		dbAllocator = NewPostgresAllocator()
	default:
		dbAllocator = NewMySQLAllocator()
	}
	return dbAllocator
}

func NewRoutingAPIArgs(
	ip string,
	port int,
	mtlsPort int,
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
	port int,
	mtlsPort int,
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
		MetronConfig: config.MetronConfig{
			Address: "1.2.3.4",
			Port:    "4567",
		},
		API: config.APIConfig{
			ListenPort:         port,
			HTTPEnabled:        true,
			MTLSListenPort:     mtlsPort,
			MTLSClientCAPath:   mtlsClientCAPath,
			MTLSServerCertPath: mtlsServerCertPath,
			MTLSServerKeyPath:  mtlsServerKeyPath,
		},
		MetricsReportingIntervalString:  "500ms",
		StatsdEndpoint:                  "localhost:8125",
		StatsdClientFlushIntervalString: "10ms",
		SystemDomain:                    "example.com",
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

	switch dbEnv {
	case "postgres":
		routingAPIConfig.SqlDB = config.SqlDB{
			Username: "postgres",
			Password: "",
			Schema:   dbId,
			Port:     5432,
			Host:     "localhost",
			Type:     "postgres",
			CACert:   dbCACert,
		}
	default:
		routingAPIConfig.SqlDB = config.SqlDB{
			Username: "root",
			Password: "password",
			Schema:   dbId,
			Port:     3306,
			Host:     "localhost",
			Type:     "mysql",
			CACert:   dbCACert,
		}
	}

	routingAPIConfigBytes, err := yaml.Marshal(routingAPIConfig)
	if err != nil {
		return "", err
	}

	configFile, err := ioutil.TempFile("", "routing-api-config")
	if err != nil {
		return "", err
	}
	if err := configFile.Close(); err != nil {
		return "", err
	}
	configFilePath := configFile.Name()

	err = utils.WriteToFile(routingAPIConfigBytes, configFilePath)
	return configFilePath, err
}
