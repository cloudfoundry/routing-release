package integration

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	tls_helpers "code.cloudfoundry.org/cf-routing-test-helpers/tls"
	"code.cloudfoundry.org/route-registrar/config"
	"code.cloudfoundry.org/route-registrar/messagebus"
	"code.cloudfoundry.org/tlsconfig"
	"github.com/nats-io/nats.go"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Main", func() {
	var (
		natsCmd                           *exec.Cmd
		testSpyClient                     *nats.Conn
		natsCAPath                        string
		mtlsNATSCertPath, mtlsNATSKeyPath string
	)

	BeforeEach(func() {
		natsHost := "127.0.0.1"

		// The server cert and client cert are the same
		natsCAPath, mtlsNATSCertPath, mtlsNATSKeyPath, _ = tls_helpers.GenerateCaAndMutualTlsCerts()

		natsCmd = startNatsTLS(natsHost, natsPort, natsCAPath, mtlsNATSCertPath, mtlsNATSKeyPath)

		rootConfig := initConfig()
		rootConfig.MessageBusServers = []config.MessageBusServerSchema{
			{
				Host: fmt.Sprintf("%s:%d", natsHost, natsPort),
			},
		}
		rootConfig.NATSmTLSConfig = config.ClientTLSConfigSchema{
			CertPath: mtlsNATSCertPath,
			KeyPath:  mtlsNATSKeyPath,
			CAPath:   natsCAPath,
		}
		writeConfig(rootConfig)

		servers := []string{
			fmt.Sprintf(
				"nats://%s:%d",
				natsHost,
				natsPort,
			),
		}

		opts := nats.GetDefaultOptions()
		opts.Servers = servers

		spyClientTLSConfig, err := tlsconfig.Build(
			tlsconfig.WithInternalServiceDefaults(),
			tlsconfig.WithIdentityFromFile(mtlsNATSCertPath, mtlsNATSKeyPath),
		).Client(
			tlsconfig.WithAuthorityFromFile(natsCAPath),
		)
		Expect(err).NotTo(HaveOccurred())

		opts.TLSConfig = spyClientTLSConfig

		Eventually(func() error {
			testSpyClient, err = opts.Connect()
			return err
		}).ShouldNot(HaveOccurred())

	})

	AfterEach(func() {
		testSpyClient.Close()
		Expect(os.Remove(mtlsNATSCertPath)).To(Succeed())
		Expect(os.Remove(mtlsNATSKeyPath)).To(Succeed())
		Expect(natsCmd.Process.Kill()).To(Succeed())
	})

	It("Writes pid to the provided pidfile", func() {
		command := exec.Command(
			routeRegistrarBinPath,
			fmt.Sprintf("-pidfile=%s", pidFile),
			fmt.Sprintf("-configPath=%s", configFile),
		)
		session, err := gexec.Start(command, GinkgoWriter, GinkgoWriter)
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(session.Out).Should(gbytes.Say("Initializing"))
		Eventually(session.Out).Should(gbytes.Say("Writing pid"))
		Eventually(session.Out).Should(gbytes.Say("Running"))

		session.Kill().Wait()
		Eventually(session).Should(gexec.Exit())

		pidFileContents, err := os.ReadFile(pidFile)
		Expect(err).ShouldNot(HaveOccurred())

		Expect(len(pidFileContents)).To(BeNumerically(">", 0))
	})

	It("registers routes via NATS", func() {
		const (
			topic = "router.register"
		)

		registered := make(chan string)
		testSpyClient.Subscribe(topic, func(msg *nats.Msg) {
			registered <- string(msg.Data)
		})

		command := exec.Command(
			routeRegistrarBinPath,
			fmt.Sprintf("-configPath=%s", configFile),
		)
		session, err := gexec.Start(command, GinkgoWriter, GinkgoWriter)
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(session.Out).Should(gbytes.Say("Initializing"))
		Eventually(session.Out).Should(gbytes.Say("Running"))
		Eventually(session.Out, 10*time.Second).Should(gbytes.Say("Registering"))

		var receivedMessage string
		Eventually(registered, 10*time.Second).Should(Receive(&receivedMessage))

		i12345 := uint16(12345)
		expectedRegistryMessage := messagebus.Message{
			URIs: []string{"uri-1", "uri-2"},
			Host: "127.0.0.1",
			Port: &i12345,
			Tags: map[string]string{"tag1": "val1", "tag2": "val2"},
		}

		var registryMessage messagebus.Message
		err = json.Unmarshal([]byte(receivedMessage), &registryMessage)
		Expect(err).ShouldNot(HaveOccurred())

		Expect(registryMessage.URIs).To(Equal(expectedRegistryMessage.URIs))
		Expect(registryMessage.Port).To(Equal(expectedRegistryMessage.Port))
		Expect(registryMessage.Tags).To(Equal(expectedRegistryMessage.Tags))

		session.Kill().Wait()
		Eventually(session).Should(gexec.Exit())
	})

	It("Starts correctly and shuts down on SIGINT", func() {
		command := exec.Command(
			routeRegistrarBinPath,
			fmt.Sprintf("-configPath=%s", configFile),
		)
		session, err := gexec.Start(command, GinkgoWriter, GinkgoWriter)
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(session.Out).Should(gbytes.Say("Initializing"))
		Eventually(session.Out).Should(gbytes.Say("Running"))
		Eventually(session.Out, 10*time.Second).Should(gbytes.Say("Registering"))

		session.Interrupt().Wait(10 * time.Second)
		Eventually(session.Out).Should(gbytes.Say("Caught signal"))
		Eventually(session.Out).Should(gbytes.Say("Unregistering"))
		Eventually(session).Should(gexec.Exit())
		Expect(session.ExitCode()).To(BeZero())
	})

	It("Starts correctly and shuts down on SIGTERM", func() {
		command := exec.Command(
			routeRegistrarBinPath,
			fmt.Sprintf("-configPath=%s", configFile),
		)
		session, err := gexec.Start(command, GinkgoWriter, GinkgoWriter)
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(session.Out).Should(gbytes.Say("Initializing"))
		Eventually(session.Out).Should(gbytes.Say("Running"))
		Eventually(session.Out, 10*time.Second).Should(gbytes.Say("Registering"))

		session.Terminate().Wait(10 * time.Second)
		Eventually(session.Out).Should(gbytes.Say("Caught signal"))
		Eventually(session.Out).Should(gbytes.Say("Unregistering"))
		Eventually(session).Should(gexec.Exit())
		Expect(session.ExitCode()).To(BeZero())
	})

	Context("When the config validation fails", func() {
		BeforeEach(func() {
			rootConfig := initConfig()

			rootConfig.Routes[0].RegistrationInterval = "asdf"
			writeConfig(rootConfig)
		})

		It("exits with error", func() {
			command := exec.Command(
				routeRegistrarBinPath,
				fmt.Sprintf("-configPath=%s", configFile),
			)
			session, err := gexec.Start(command, GinkgoWriter, GinkgoWriter)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(session.Out).Should(gbytes.Say("Initializing"))
			Eventually(session.Err).Should(gbytes.Say(`1 error with 'route "My route"'`))
			Eventually(session.Err).Should(gbytes.Say("registration_interval: time: invalid duration \"asdf\""))

			Eventually(session).Should(gexec.Exit())
			Expect(session.ExitCode()).ToNot(BeZero())
		})
	})

})

func initConfig() config.ConfigSchema {
	aPort := uint16(12345)

	registrationInterval := "1s"

	messageBusServers := []config.MessageBusServerSchema{
		{
			Host:     fmt.Sprintf("127.0.0.1:%d", natsPort),
			User:     "nats",
			Password: "nats",
		},
	}

	routes := []config.RouteSchema{
		{
			Name:                 "My route",
			Port:                 &aPort,
			URIs:                 []string{"uri-1", "uri-2"},
			Tags:                 map[string]string{"tag1": "val1", "tag2": "val2"},
			RegistrationInterval: registrationInterval,
		},
	}

	return config.ConfigSchema{
		MessageBusServers: messageBusServers,
		Host:              "127.0.0.1",
		Routes:            routes,
	}
}

func writeConfig(config config.ConfigSchema) {
	fileToWrite, err := os.Create(configFile)
	Expect(err).ShouldNot(HaveOccurred())

	data, err := json.Marshal(config)
	Expect(err).ShouldNot(HaveOccurred())

	_, err = fileToWrite.Write(data)
	Expect(err).ShouldNot(HaveOccurred())
}

func startNatsTLS(host string, port uint16, caFile, certFile, keyFile string) *exec.Cmd {
	fmt.Fprintf(GinkgoWriter, "Starting nats-server on port %d\n", port)
	natsServer, exists := os.LookupEnv("NATS_SERVER_BINARY")
	if !exists {
		fmt.Println("You need nats-server installed and set NATS_SERVER_BINARY env variable")
		os.Exit(1)
	}

	cmd := exec.Command(
		natsServer,
		"-p", fmt.Sprintf("%d", port),
		"--tlsverify",
		"--tlscacert", caFile,
		"--tlscert", certFile,
		"--tlskey", keyFile,
	)

	err := cmd.Start()
	if err != nil {
		fmt.Printf("nats-server failed to start: %v\n", err)
	}

	natsTimeout := 10 * time.Second
	natsPollingInterval := 20 * time.Millisecond
	Eventually(func() error {
		_, err := net.Dial("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
		return err
	}, natsTimeout, natsPollingInterval).Should(Succeed())

	fmt.Fprintf(GinkgoWriter, "nats-server running on port %d\n", port)
	return cmd
}
