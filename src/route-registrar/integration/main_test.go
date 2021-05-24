package integration

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strconv"
	"time"

	tls_helpers "code.cloudfoundry.org/cf-routing-test-helpers/tls"
	"code.cloudfoundry.org/routing-release/route-registrar/config"
	"code.cloudfoundry.org/routing-release/route-registrar/messagebus"
	"code.cloudfoundry.org/routing-release/routing-api/test_helpers"
	"code.cloudfoundry.org/tlsconfig"
	"github.com/nats-io/nats.go"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Main", func() {
	var (
		natsCmd       *exec.Cmd
		testSpyClient *nats.Conn
	)

	BeforeEach(func() {
		natsUsername := "nats"
		natsPassword := "nats"
		natsHost := "127.0.0.1"

		rootConfig := initConfig()
		writeConfig(rootConfig)

		natsCmd = exec.Command(
			"nats-server",
			"-p", strconv.Itoa(natsPort),
			"--user", natsUsername,
			"--pass", natsPassword,
		)

		err := natsCmd.Start()
		Expect(err).NotTo(HaveOccurred())

		natsAddress := fmt.Sprintf("127.0.0.1:%d", natsPort)

		Eventually(func() error {
			_, err := net.Dial("tcp", natsAddress)
			return err
		}).Should(Succeed())

		servers := []string{
			fmt.Sprintf(
				"nats://%s:%s@%s:%d",
				natsUsername,
				natsPassword,
				natsHost,
				natsPort,
			),
		}

		opts := nats.DefaultOptions
		opts.Servers = servers

		testSpyClient, err = opts.Connect()

		Expect(err).ShouldNot(HaveOccurred())
	})

	AfterEach(func() {
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

		pidFileContents, err := ioutil.ReadFile(pidFile)
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

		i12345 := 12345
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

	Context("When route registrar is configured to use mTLS to connect to NATS", func() {
		var (
			natsCAPath                        string
			mtlsNATSCertPath, mtlsNATSKeyPath string
			tlsTestSpyClient                  *nats.Conn
			tlsNATSCmd                        *exec.Cmd
		)

		BeforeEach(func() {
			natsHost := "127.0.0.1"
			natsTLSPort := test_helpers.NextAvailPort()

			// The server cert and client cert are the same
			natsCAPath, mtlsNATSCertPath, mtlsNATSKeyPath, _ = tls_helpers.GenerateCaAndMutualTlsCerts()

			tlsNATSCmd = startNatsTLS(natsHost, natsTLSPort, natsCAPath, mtlsNATSCertPath, mtlsNATSKeyPath)

			tlsServers := []string{
				fmt.Sprintf(
					"nats://%s:%d",
					natsHost,
					natsTLSPort,
				),
			}

			tlsOpts := nats.DefaultOptions
			tlsOpts.Servers = tlsServers

			spyClientTLSConfig, err := tlsconfig.Build(
				tlsconfig.WithInternalServiceDefaults(),
				tlsconfig.WithIdentityFromFile(mtlsNATSCertPath, mtlsNATSKeyPath),
			).Client(
				tlsconfig.WithAuthorityFromFile(natsCAPath),
			)
			Expect(err).NotTo(HaveOccurred())

			tlsOpts.TLSConfig = spyClientTLSConfig

			tlsTestSpyClient, err = tlsOpts.Connect()
			Expect(err).ToNot(HaveOccurred())

			Expect(err).ShouldNot(HaveOccurred())

			// Ensure nats server is listening before tests
			Eventually(func() string {
				connStatus := tlsTestSpyClient.Status()
				return fmt.Sprintf("%v", connStatus)
			}, 5*time.Second).Should(Equal("1"))

			Expect(err).ShouldNot(HaveOccurred())

			rootConfig := initConfig()
			rootConfig.MessageBusServers = []config.MessageBusServerSchema{
				{
					Host: fmt.Sprintf("%s:%d", natsHost, natsTLSPort),
				},
			}
			rootConfig.NATSmTLSConfig = config.ClientTLSConfigSchema{
				Enabled:  true,
				CertPath: mtlsNATSCertPath,
				KeyPath:  mtlsNATSKeyPath,
				CAPath:   natsCAPath,
			}
			writeConfig(rootConfig)
		})

		AfterEach(func() {
			tlsTestSpyClient.Close()
			Expect(os.Remove(mtlsNATSCertPath)).To(Succeed())
			Expect(os.Remove(mtlsNATSKeyPath)).To(Succeed())

			Expect(tlsNATSCmd.Process.Kill()).To(Succeed())
		})

		It("registers routes via NATS", func() {
			const (
				topic = "router.register"
			)

			registered := make(chan string)
			tlsTestSpyClient.Subscribe(topic, func(msg *nats.Msg) {
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

			i12345 := 12345
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
	})
})

func initConfig() config.ConfigSchema {
	aPort := 12345

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

func startNatsTLS(host string, port int, caFile, certFile, keyFile string) *exec.Cmd {
	fmt.Fprintf(GinkgoWriter, "Starting nats-server on port %d\n", port)

	cmd := exec.Command(
		"nats-server",
		"-p", strconv.Itoa(port),
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
		_, err := net.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
		return err
	}, natsTimeout, natsPollingInterval).Should(Succeed())

	fmt.Fprintf(GinkgoWriter, "nats-server running on port %d\n", port)
	return cmd
}
