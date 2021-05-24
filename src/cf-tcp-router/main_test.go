package main_test

import (
	"crypto/tls"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"code.cloudfoundry.org/routing-release/cf-tcp-router/testrunner"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	routingtestrunner "code.cloudfoundry.org/routing-release/routing-api/cmd/routing-api/testrunner"
	"code.cloudfoundry.org/routing-release/routing-api/models"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/ghttp"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Main", func() {
	var (
		routerGroupGuid string
	)

	getServerPort := func(serverURL string) int {
		u, err := url.Parse(serverURL)
		Expect(err).NotTo(HaveOccurred())

		_, portStr, err := net.SplitHostPort(u.Host)
		Expect(err).NotTo(HaveOccurred())

		port, err := strconv.Atoi(portStr)
		Expect(err).NotTo(HaveOccurred())
		return port
	}

	oAuthServer := func(logger lager.Logger, serverCert tls.Certificate) *ghttp.Server {
		server := ghttp.NewUnstartedServer()

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{serverCert},
		}
		server.HTTPTestServer.TLS = tlsConfig
		server.AllowUnhandledRequests = true
		server.HTTPTestServer.StartTLS()

		publicKey := "-----BEGIN PUBLIC KEY-----\\n" +
			"MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDHFr+KICms+tuT1OXJwhCUmR2d\\n" +
			"KVy7psa8xzElSyzqx7oJyfJ1JZyOzToj9T5SfTIq396agbHJWVfYphNahvZ/7uMX\\n" +
			"qHxf+ZH9BL1gk9Y6kCnbM5R60gfwjyW1/dQPjOzn9N394zd2FJoFHwdq9Qs0wBug\\n" +
			"spULZVNRxq7veq/fzwIDAQAB\\n" +
			"-----END PUBLIC KEY-----"

		data := fmt.Sprintf(`{"alg":"rsa", "value":"%s"}`, publicKey)
		server.RouteToHandler("GET", "/token_key",
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/token_key"),
				ghttp.RespondWith(http.StatusOK, data)),
		)

		server.RouteToHandler("POST", "/oauth/token",
			func(w http.ResponseWriter, req *http.Request) {
				jsonBytes := []byte(`{"access_token":"some-token", "expires_in":10}`)
				w.Write(jsonBytes)
			},
		)
		logger.Info("starting-oauth-server", lager.Data{"address": server.URL()})
		return server
	}

	routingApiServer := func(logger lager.Logger) ifrit.Process {
		server := routingtestrunner.New(routingAPIBinPath, routingAPIArgs)
		logger.Info("starting-routing-api-server")
		process := ginkgomon.Invoke(server)
		return process
	}

	verifyHaProxyConfigContent := func(haproxyFileName, expectedContent string, present bool) {
		Eventually(func() bool {
			data, err := ioutil.ReadFile(haproxyFileName)
			Expect(err).ShouldNot(HaveOccurred())
			return strings.Contains(string(data), expectedContent)
		}, 6, 1).Should(Equal(present))
	}

	var (
		oauthServer *ghttp.Server
		server      ifrit.Process
		logger      *lagertest.TestLogger
		session     *gexec.Session
		uaaServCert tls.Certificate
		uaaCAPath   string
	)

	BeforeEach(func() {
		uaaCACert, uaaCAPrivKey, err := createCA()
		Expect(err).ToNot(HaveOccurred())

		f, err := ioutil.TempFile("", "routing-api-uaa-ca")
		Expect(err).ToNot(HaveOccurred())

		uaaCAPath = f.Name()

		err = pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: uaaCACert.Raw})
		Expect(err).ToNot(HaveOccurred())

		err = f.Close()
		Expect(err).ToNot(HaveOccurred())

		uaaServCert, err = createCertificate(uaaCACert, uaaCAPrivKey, isCA)
		Expect(err).ToNot(HaveOccurred())

		logger = lagertest.NewTestLogger("test")
	})

	AfterEach(func() {
		logger.Info("shutting-down")
		session.Signal(os.Interrupt)
		Eventually(session.Exited, 5*time.Second).Should(BeClosed())

		err := os.Remove(uaaCAPath)
		Expect(err).NotTo(HaveOccurred())

		ginkgomon.Interrupt(server, "10s")
		if oauthServer != nil {
			oauthServer.Close()
		}
	})

	Context("when both oauth and routing api servers are up and running", func() {
		BeforeEach(func() {
			oauthServer = oAuthServer(logger, uaaServCert)
			server = routingApiServer(logger)
			routerGroupGuid = getRouterGroupGuid(routingApiClient)
			oauthServerPort := getServerPort(oauthServer.URL())
			configFile := generateTCPRouterConfigFile(oauthServerPort, uaaCAPath, false, 1020)
			tcpRouterArgs := testrunner.Args{
				BaseLoadBalancerConfigFilePath: haproxyBaseConfigFile,
				LoadBalancerConfigFilePath:     haproxyConfigFile,
				ConfigFilePath:                 configFile,
			}

			tcpRouteMapping := models.NewTcpRouteMapping(routerGroupGuid, 5222, "some-ip-1", 61000, 120)
			err := routingApiClient.UpsertTcpRouteMappings([]models.TcpRouteMapping{tcpRouteMapping})
			Expect(err).ToNot(HaveOccurred())

			tcpRouteMappings, err := routingApiClient.TcpRouteMappings()
			Expect(err).NotTo(HaveOccurred())
			Expect(contains(tcpRouteMappings, tcpRouteMapping)).To(BeTrue())

			allOutput := logger.Buffer()
			runner := testrunner.New(tcpRouterPath, tcpRouterArgs)
			session, err = gexec.Start(runner.Command, allOutput, allOutput)
			Expect(err).ToNot(HaveOccurred())
		})

		It("syncs with routing api", func() {
			Eventually(session.Out, 5*time.Second).Should(gbytes.Say("applied-fetched-routes-to-routing-table"))
			expectedConfigEntry := "\nfrontend frontend_5222\n  mode tcp\n  bind :5222\n"
			serverConfigEntry := "server server_some-ip-1_61000 some-ip-1:61000"
			verifyHaProxyConfigContent(haproxyConfigFile, expectedConfigEntry, true)
			verifyHaProxyConfigContent(haproxyConfigFile, serverConfigEntry, true)
		})

		It("starts an SSE connection to the server", func() {
			Eventually(session.Out, 5*time.Second).Should(gbytes.Say("Subscribing-to-routing-api-event-stream"))
			Eventually(session.Out, 5*time.Second).Should(gbytes.Say("Successfully-subscribed-to-routing-api-event-stream"))
			tcpRouteMapping := models.NewTcpRouteMapping(routerGroupGuid, 5222, "some-ip-2", 61000, 120)
			err := routingApiClient.UpsertTcpRouteMappings([]models.TcpRouteMapping{tcpRouteMapping})
			Expect(err).ToNot(HaveOccurred())
			Eventually(session.Out, 5*time.Second).Should(gbytes.Say("handle-event.finished"))
			expectedConfigEntry := "\nfrontend frontend_5222\n  mode tcp\n  bind :5222\n"
			verifyHaProxyConfigContent(haproxyConfigFile, expectedConfigEntry, true)
			oldServerConfigEntry := "server server_some-ip-1_61000 some-ip-1:61000"
			verifyHaProxyConfigContent(haproxyConfigFile, oldServerConfigEntry, true)
			newServerConfigEntry := "server server_some-ip-2_61000 some-ip-2:61000"
			verifyHaProxyConfigContent(haproxyConfigFile, newServerConfigEntry, true)
		})

		It("prunes stale routes", func() {
			Eventually(session.Out, 5*time.Second).Should(gbytes.Say("Subscribing-to-routing-api-event-stream"))
			Eventually(session.Out, 5*time.Second).Should(gbytes.Say("Successfully-subscribed-to-routing-api-event-stream"))
			tcpRouteMapping := models.NewTcpRouteMapping(routerGroupGuid, 5222, "some-ip-3", 61000, 6)
			err := routingApiClient.UpsertTcpRouteMappings([]models.TcpRouteMapping{tcpRouteMapping})
			Expect(err).ToNot(HaveOccurred())
			Eventually(session.Out, 5*time.Second).Should(gbytes.Say("handle-event.finished"))
			expectedConfigEntry := "\nfrontend frontend_5222\n  mode tcp\n  bind :5222\n"
			verifyHaProxyConfigContent(haproxyConfigFile, expectedConfigEntry, true)
			oldServerConfigEntry := "server server_some-ip-1_61000 some-ip-1:61000"
			verifyHaProxyConfigContent(haproxyConfigFile, oldServerConfigEntry, true)
			newServerConfigEntry := "server server_some-ip-3_61000 some-ip-3:61000"
			verifyHaProxyConfigContent(haproxyConfigFile, newServerConfigEntry, true)
			Eventually(session.Out, 10*time.Second, 1*time.Second).Should(gbytes.Say("prune-stale-routes.starting"))
			Eventually(session.Out, 10*time.Second, 1*time.Second).Should(gbytes.Say("prune-stale-routes.completed"))
			verifyHaProxyConfigContent(haproxyConfigFile, newServerConfigEntry, false)
		})

		It("Confirms when there are no conflicting ports", func() {
			Eventually(session.Out, 5*time.Second).Should(gbytes.Say("router-group-port-checker-success"))
		})
	})

	Context("when systemComponentPorts conflict", func() {
		BeforeEach(func() {
			oauthServer = oAuthServer(logger, uaaServCert)
			server = routingApiServer(logger)
			routerGroupGuid = getRouterGroupGuid(routingApiClient)
			oauthServerPort := getServerPort(oauthServer.URL())
			configFile := generateTCPRouterConfigFile(oauthServerPort, uaaCAPath, false, 1040)
			tcpRouterArgs := testrunner.Args{
				BaseLoadBalancerConfigFilePath: haproxyBaseConfigFile,
				LoadBalancerConfigFilePath:     haproxyConfigFile,
				ConfigFilePath:                 configFile,
				RoutingGroupCheckExit:          true,
			}

			var err error
			allOutput := logger.Buffer()
			runner := testrunner.New(tcpRouterPath, tcpRouterArgs)
			session, err = gexec.Start(runner.Command, allOutput, allOutput)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Confirms when routing groups don't allow conflicting ports and exits", func() {
			Eventually(session.Out, 5*time.Second).Should(gbytes.Say("The reserved ports for router group \\'default-tcp\\' contains the following reserved system component port\\(s\\): \\'1040\\'. Please update your router group accordingly."))
			Eventually(session.Exited).Should(BeClosed())
		})
	})

	Context("when systemComponentPorts conflict, but no fail flag is used", func() {
		BeforeEach(func() {
			oauthServer = oAuthServer(logger, uaaServCert)
			server = routingApiServer(logger)
			routerGroupGuid = getRouterGroupGuid(routingApiClient)
			oauthServerPort := getServerPort(oauthServer.URL())
			configFile := generateTCPRouterConfigFile(oauthServerPort, uaaCAPath, false, 1040)
			tcpRouterArgs := testrunner.Args{
				BaseLoadBalancerConfigFilePath: haproxyBaseConfigFile,
				LoadBalancerConfigFilePath:     haproxyConfigFile,
				ConfigFilePath:                 configFile,
			}

			var err error
			allOutput := logger.Buffer()
			runner := testrunner.New(tcpRouterPath, tcpRouterArgs)
			session, err = gexec.Start(runner.Command, allOutput, allOutput)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Confirms when routing groups don't allow conflicting ports, but doesn't exit", func() {
			Eventually(session.Out, 5*time.Second).Should(gbytes.Say("The reserved ports for router group \\'default-tcp\\' contains the following reserved system component port\\(s\\): \\'1040\\'. Please update your router group accordingly."))
			Eventually(session.Exited).ShouldNot(BeClosed())
		})
	})

	Context("Oauth server is down", func() {
		var (
			tcpRouterArgs   testrunner.Args
			configFile      string
			oauthServerPort int
		)
		BeforeEach(func() {
			server = routingApiServer(logger)
			oauthServerPort = 1111
		})

		JustBeforeEach(func() {
			allOutput := logger.Buffer()
			runner := testrunner.New(tcpRouterPath, tcpRouterArgs)
			var err error
			session, err = gexec.Start(runner.Command, allOutput, allOutput)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("routing api auth is enabled", func() {
			BeforeEach(func() {
				configFile = generateTCPRouterConfigFile(oauthServerPort, uaaCAPath, false)
				tcpRouterArgs = testrunner.Args{
					BaseLoadBalancerConfigFilePath: haproxyBaseConfigFile,
					LoadBalancerConfigFilePath:     haproxyConfigFile,
					ConfigFilePath:                 configFile,
				}
			})

			It("exits with error", func() {
				Eventually(session.Out, 5*time.Second).Should(gbytes.Say("failed-connecting-to-uaa"))
				Eventually(session.Exited).Should(BeClosed())
			})
		})

		Context("routing api auth is disabled", func() {
			BeforeEach(func() {
				configFile = generateTCPRouterConfigFile(oauthServerPort, uaaCAPath, true)
				tcpRouterArgs = testrunner.Args{
					BaseLoadBalancerConfigFilePath: haproxyBaseConfigFile,
					LoadBalancerConfigFilePath:     haproxyConfigFile,
					ConfigFilePath:                 configFile,
				}
			})

			It("does not call oauth server to get auth token and starts SSE connection with routing api", func() {
				Eventually(session.Out, 5*time.Second).Should(gbytes.Say("creating-noop-uaa-client"))
				Eventually(session.Out, 5*time.Second).Should(gbytes.Say("Successfully-subscribed-to-routing-api-event-stream"))
			})
		})
	})

	Context("Routing API server is down", func() {
		BeforeEach(func() {
			oauthServer = oAuthServer(logger, uaaServCert)
			oauthServerPort := getServerPort(oauthServer.URL())
			configFile := generateTCPRouterConfigFile(oauthServerPort, uaaCAPath, false)
			tcpRouterArgs := testrunner.Args{
				BaseLoadBalancerConfigFilePath: haproxyBaseConfigFile,
				LoadBalancerConfigFilePath:     haproxyConfigFile,
				ConfigFilePath:                 configFile,
			}
			allOutput := logger.Buffer()
			runner := testrunner.New(tcpRouterPath, tcpRouterArgs)
			var err error
			session, err = gexec.Start(runner.Command, allOutput, allOutput)
			Expect(err).ToNot(HaveOccurred())
		})

		It("keeps trying to connect and doesn't blow up", func() {
			Eventually(session.Out, 5*time.Second).Should(gbytes.Say("router-group-port-checker-error"))
			Eventually(session.Out, 5*time.Second).Should(gbytes.Say("Subscribing-to-routing-api-event-stream"))
			Consistently(session.Exited).ShouldNot(BeClosed())
			Consistently(session.Out, 5*time.Second).ShouldNot(gbytes.Say("Successfully-subscribed-to-routing-api-event-stream"))
			By("starting routing api server")
			server = routingApiServer(logger)
			routerGroupGuid = getRouterGroupGuid(routingApiClient)
			Eventually(session.Out, 5*time.Second).Should(gbytes.Say("Successfully-subscribed-to-routing-api-event-stream"))
			tcpRouteMapping := models.NewTcpRouteMapping(routerGroupGuid, 5222, "some-ip-3", 61000, 120)
			err := routingApiClient.UpsertTcpRouteMappings([]models.TcpRouteMapping{tcpRouteMapping})
			Expect(err).ToNot(HaveOccurred())
			Eventually(session.Out, 5*time.Second).Should(gbytes.Say("handle-event.finished"))
			expectedConfigEntry := "\nfrontend frontend_5222\n  mode tcp\n  bind :5222\n"
			verifyHaProxyConfigContent(haproxyConfigFile, expectedConfigEntry, true)
			newServerConfigEntry := "server server_some-ip-3_61000 some-ip-3:61000"
			verifyHaProxyConfigContent(haproxyConfigFile, newServerConfigEntry, true)
		})
	})

	Context("when haproxy is down", func() {
		BeforeEach(func() {
			oauthServer = oAuthServer(logger, uaaServCert)
			server = routingApiServer(logger)
			oauthServerPort := getServerPort(oauthServer.URL())
			configFile := generateTCPRouterConfigFile(oauthServerPort, uaaCAPath, false)
			tcpRouterArgs := testrunner.Args{
				BaseLoadBalancerConfigFilePath: haproxyBaseConfigFile,
				LoadBalancerConfigFilePath:     haproxyConfigFile,
				ConfigFilePath:                 configFile,
			}

			runner := testrunner.New(tcpRouterPath, tcpRouterArgs)

			var err error
			session, err = gexec.Start(runner.Command, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			setupLongRunningProcess()
		})

		It("exits", func() {
			Eventually(session.Out, 5*time.Second).Should(gbytes.Say("Subscribing-to-routing-api-event-stream"))

			killLongRunningProcess()

			Eventually(session, "5s").Should(gexec.Exit())
		})
	})
})

func contains(ms []models.TcpRouteMapping, tcpRouteMapping models.TcpRouteMapping) bool {
	for _, m := range ms {
		if m.Matches(tcpRouteMapping) {
			return true
		}
	}
	return false
}
