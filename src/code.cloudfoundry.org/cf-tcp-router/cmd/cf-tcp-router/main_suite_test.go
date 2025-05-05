package main_test

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"testing"
	"time"

	tls_helpers "code.cloudfoundry.org/cf-routing-test-helpers/tls"
	"code.cloudfoundry.org/cf-tcp-router/config"
	"code.cloudfoundry.org/cf-tcp-router/testutil"
	"code.cloudfoundry.org/cf-tcp-router/utils"
	"code.cloudfoundry.org/localip"
	locket_config "code.cloudfoundry.org/locket/cmd/locket/config"
	"code.cloudfoundry.org/locket/cmd/locket/testrunner"
	routing_api "code.cloudfoundry.org/routing-api"
	routingtestrunner "code.cloudfoundry.org/routing-api/cmd/routing-api/testrunner"
	routing_api_config "code.cloudfoundry.org/routing-api/config"
	"code.cloudfoundry.org/routing-api/models"
	"code.cloudfoundry.org/tlsconfig"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
	"gopkg.in/yaml.v3"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	tcpRouterPath           string
	routingAPIBinPath       string
	haproxyConfigFile       string
	haproxyConfigBackupFile string
	haproxyBaseConfigFile   string

	dbAllocator routingtestrunner.DbAllocator

	dbId string

	locketDBAllocator routingtestrunner.DbAllocator
	locketBinPath     string
	locketProcess     ifrit.Process
	locketPort        uint16
	locketDbConfig    *routing_api_config.SqlDB

	routingAPIAddress              string
	routingAPIArgs                 routingtestrunner.Args
	routingAPIPort                 uint16
	routingAPIMTLSPort             uint16
	routingAPIIP                   string
	routingApiClient               routing_api.Client
	routingAPICAFileName           string
	routingAPICAPrivateKey         *rsa.PrivateKey
	routingAPIClientCertPath       string
	routingAPIClientPrivateKeyPath string

	longRunningProcessPidFile string
	catCmd                    *exec.Cmd
)

func nextAvailPort() uint16 {
	port, err := localip.LocalPort()
	Expect(err).ToNot(HaveOccurred())

	return port
}

func TestTCPRouter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TCPRouter Suite")
}

var _ = SynchronizedBeforeSuite(func() []byte {
	tcpRouter, err := gexec.Build("code.cloudfoundry.org/cf-tcp-router/cmd/cf-tcp-router", "-race")
	Expect(err).NotTo(HaveOccurred())
	routingAPIBin, err := gexec.Build("code.cloudfoundry.org/routing-api/cmd/routing-api", "-race")
	Expect(err).NotTo(HaveOccurred())
	locketBin, err := gexec.Build("code.cloudfoundry.org/locket/cmd/locket", "-race")
	Expect(err).NotTo(HaveOccurred())

	payload, err := json.Marshal(map[string]string{
		"tcp-router":  tcpRouter,
		"routing-api": routingAPIBin,
		"locket":      locketBin,
	})
	Expect(err).NotTo(HaveOccurred())

	return payload
}, func(payload []byte) {
	context := map[string]string{}

	err := json.Unmarshal(payload, &context)
	Expect(err).NotTo(HaveOccurred())

	tcpRouterPath = context["tcp-router"]
	routingAPIBinPath = context["routing-api"]
	locketBinPath = context["locket"]

	setupDB()
	locketPort = uint16(nextAvailPort())
	locketDBAllocator = routingtestrunner.NewDbAllocator()

	locketDbConfig, err = locketDBAllocator.Create()
	Expect(err).NotTo(HaveOccurred())

})

func setupDB() {
	dbAllocator = routingtestrunner.NewDbAllocator()

	dbConfig, err := dbAllocator.Create()
	Expect(err).NotTo(HaveOccurred())
	dbId = dbConfig.Schema
}

func setupLongRunningProcess() {
	catCmd = exec.Command("cat")
	err := catCmd.Start()
	Expect(err).ToNot(HaveOccurred())
	pid := catCmd.Process.Pid

	file, err := os.CreateTemp(os.TempDir(), "test-pid-file")
	Expect(err).ToNot(HaveOccurred())
	_, err = file.WriteString(fmt.Sprintf("%d", pid))
	Expect(err).ToNot(HaveOccurred())
	defer file.Close()

	longRunningProcessPidFile = file.Name()
}

func killLongRunningProcess() {
	isAlive := catCmd.ProcessState == nil
	if isAlive {
		err := catCmd.Process.Kill()
		Expect(err).ToNot(HaveOccurred())
	}
	Expect(os.Remove(longRunningProcessPidFile)).To(Succeed())
}

var _ = BeforeEach(func() {
	setupLocket()

	randomFileName := testutil.RandomFileName("haproxy_", ".cfg")
	randomBackupFileName := fmt.Sprintf("%s.bak", randomFileName)
	randomBaseFileName := testutil.RandomFileName("haproxy_base_", ".cfg")
	haproxyConfigFile = path.Join(os.TempDir(), randomFileName)
	haproxyConfigBackupFile = path.Join(os.TempDir(), randomBackupFileName)
	haproxyBaseConfigFile = path.Join(os.TempDir(), randomBaseFileName)

	err := utils.WriteToFile(
		[]byte(
			`global maxconn 4096
defaults
  log global
  timeout connect 300000
  timeout client 300000
  timeout server 300000
  maxconn 2000`),
		haproxyBaseConfigFile)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(utils.FileExists(haproxyBaseConfigFile)).To(BeTrue())

	err = utils.CopyFile(haproxyBaseConfigFile, haproxyConfigFile)
	Expect(err).ShouldNot(HaveOccurred())
	Expect(utils.FileExists(haproxyConfigFile)).To(BeTrue())

	routingAPIPort = nextAvailPort()
	routingAPIMTLSPort = nextAvailPort()
	routingAPIIP = "127.0.0.1"
	routingAPIAddress = fmt.Sprintf("https://%s:%d", routingAPIIP, routingAPIMTLSPort)

	dbCACert := os.Getenv("SQL_SERVER_CA_CERT")

	routingAPICAFileName, routingAPICAPrivateKey = tls_helpers.GenerateCa()
	routingAPIServerCertPath, routingAPIServerKeyPath, _ := tls_helpers.GenerateCertAndKey(routingAPICAFileName, routingAPICAPrivateKey)

	routingAPIArgs, err = routingtestrunner.NewRoutingAPIArgs(
		routingAPIIP,
		routingAPIPort,
		routingAPIMTLSPort,
		dbId,
		dbCACert,
		fmt.Sprintf("localhost:%d", locketPort),
		routingAPICAFileName,
		routingAPIServerCertPath,
		routingAPIServerKeyPath,
	)
	Expect(err).NotTo(HaveOccurred())

	routingAPIClientCertPath, routingAPIClientPrivateKeyPath, _ = tls_helpers.GenerateCertAndKey(routingAPICAFileName, routingAPICAPrivateKey)

	tlsConfig, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentityFromFile(routingAPIClientCertPath, routingAPIClientPrivateKeyPath),
	).Client(
		tlsconfig.WithAuthorityFromFile(routingAPICAFileName),
	)
	Expect(err).NotTo(HaveOccurred())
	routingApiClient = routing_api.NewClientWithTLSConfig(routingAPIAddress, tlsConfig)

	setupLongRunningProcess()
})

var _ = AfterEach(func() {
	err := os.Remove(haproxyConfigFile)
	Expect(err).ShouldNot(HaveOccurred())

	os.Remove(haproxyConfigBackupFile)

	teardownLocket()
	dbAllocator.Reset()
	locketDBAllocator.Reset()
	killLongRunningProcess()
})

var _ = SynchronizedAfterSuite(func() {
	dbAllocator.Delete()
	locketDBAllocator.Delete()
}, func() {
	gexec.CleanupBuildArtifacts()
})

func setupLocket() {
	locketRunner := testrunner.NewLocketRunner(locketBinPath, func(c *locket_config.LocketConfig) {
		switch os.Getenv("DB") {
		case "postgres":
			c.DatabaseConnectionString = "user=postgres password= host=localhost dbname=" + locketDbConfig.Schema
			c.DatabaseDriver = "postgres"
		default:
			c.DatabaseConnectionString = "root:password@/" + locketDbConfig.Schema
			c.DatabaseDriver = "mysql"
		}
		c.ListenAddress = fmt.Sprintf("localhost:%d", locketPort)
	})
	locketProcess = ginkgomon.Invoke(locketRunner)
}

func teardownLocket() {
	ginkgomon.Interrupt(locketProcess, 5*time.Second)
}

func getRouterGroupGuid(routingApiClient routing_api.Client) string {
	var routerGroups []models.RouterGroup
	Eventually(func() error {
		var err error
		routerGroups, err = routingApiClient.RouterGroups()
		return err
	}, "30s", "1s").ShouldNot(HaveOccurred(), "Failed to connect to Routing API server after 30s.")
	Expect(routerGroups).ToNot(HaveLen(0))
	return routerGroups[0].Guid
}

func generateTCPRouterConfigFile(oauthServerPort uint16, uaaCACertsPath string, routingApiAuthDisabled bool, reserved_routing_ports ...uint16) string {
	tcpRouterConfig := config.Config{
		ReservedSystemComponentPorts: reserved_routing_ports,
		OAuth: config.OAuthConfig{
			TokenEndpoint:     "127.0.0.1",
			SkipSSLValidation: false,
			CACerts:           uaaCACertsPath,
			ClientName:        "someclient",
			ClientSecret:      "somesecret",
			Port:              oauthServerPort,
		},
		DrainWaitDuration: 3 * time.Second,
		RoutingAPI: config.RoutingAPIConfig{
			AuthDisabled: routingApiAuthDisabled,
		},
		HaProxyPidFile: longRunningProcessPidFile,
		IsolationSegments: []string{
			"foo-iso-seg",
		},
	}

	tcpRouterConfig.RoutingAPI.URI = "https://127.0.0.1"
	tcpRouterConfig.RoutingAPI.Port = routingAPIMTLSPort
	tcpRouterConfig.RoutingAPI.ClientCertificatePath = routingAPIClientCertPath
	tcpRouterConfig.RoutingAPI.ClientPrivateKeyPath = routingAPIClientPrivateKeyPath
	tcpRouterConfig.RoutingAPI.CACertificatePath = routingAPICAFileName

	bs, err := yaml.Marshal(tcpRouterConfig)
	Expect(err).NotTo(HaveOccurred())

	randomConfigFile, err := os.CreateTemp("", "tcp_router")
	Expect(err).ShouldNot(HaveOccurred())
	// Close file because we write using path instead of file handle
	randomConfigFile.Close()

	configFilePath := randomConfigFile.Name()
	Expect(utils.FileExists(configFilePath)).To(BeTrue())

	err = utils.WriteToFile(bs, configFilePath)
	Expect(err).ShouldNot(HaveOccurred())
	return configFilePath
}
