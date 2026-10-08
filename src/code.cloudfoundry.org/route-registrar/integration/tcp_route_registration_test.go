package integration

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	tls_helpers "code.cloudfoundry.org/cf-routing-test-helpers/tls"
	"code.cloudfoundry.org/route-registrar/config"

	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/ghttp"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TCP Route Registration", func() {
	var (
		oauthServer                       *ghttp.Server
		routingAPIServer                  *ghttp.Server
		natsCmd                           *exec.Cmd
		rootConfig                        config.ConfigSchema
		oauthHandlers                     []http.HandlerFunc
		natsCAPath                        string
		mtlsNATSCertPath, mtlsNATSKeyPath string
	)

	BeforeEach(func() {
		routingAPICAFileName, routingAPICAPrivateKey := tls_helpers.GenerateCa()
		_, _, serverTLSConfig := tls_helpers.GenerateCertAndKey(routingAPICAFileName, routingAPICAPrivateKey)
		routingAPIClientCertPath, routingAPIClientPrivateKeyPath, _ := tls_helpers.GenerateCertAndKey(routingAPICAFileName, routingAPICAPrivateKey)

		routingAPIServer = ghttp.NewUnstartedServer()
		routingAPIServer.HTTPTestServer.TLS = &tls.Config{}
		routingAPIServer.HTTPTestServer.TLS.RootCAs = tls_helpers.CertPool(routingAPICAFileName)
		routingAPIServer.HTTPTestServer.TLS.ClientCAs = tls_helpers.CertPool(routingAPICAFileName)
		routingAPIServer.HTTPTestServer.TLS.ClientAuth = tls.RequireAndVerifyClientCert
		routingAPIServer.HTTPTestServer.TLS.CipherSuites = []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}
		routingAPIServer.HTTPTestServer.TLS.Certificates = []tls.Certificate{serverTLSConfig}

		routingAPIResponses := []http.HandlerFunc{
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/routing/v1/router_groups"),
				ghttp.RespondWith(200, `[{
					"guid": "router-group-guid",
					"name": "my-router-group",
					"type": "tcp",
					"reservable_ports": "1024-1025"
				}]`),
			),
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("POST", "/routing/v1/tcp_routes/create"),
				ghttp.VerifyJSON(`[{
					"router_group_guid":"router-group-guid",
					"backend_port":1234,
					"backend_tls_port":-1,
					"instance_id": "",
					"backend_ip":"127.0.0.1",
					"port":5678,
					"modification_tag":{
						"guid":"",
						"index":0
					},
					"ttl": 1,
					"isolation_segment":""
				}]`),
				ghttp.RespondWith(200, ""),
			),
		}
		routingAPIServer.AppendHandlers(routingAPIResponses...)
		routingAPIServer.SetAllowUnhandledRequests(true) // sometimes multiple creates happen

		oauthServer = ghttp.NewUnstartedServer()
		oauthHandlers = []http.HandlerFunc{
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("POST", "/oauth/token"),
				ghttp.RespondWith(200, `{
					"access_token": "some-access-token",
					"token_type": "bearer",
					"expires_in": 3600
				}`,
					http.Header{"Content-Type": []string{"application/json"}},
				),
			),
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("POST", "/oauth/token"),
				ghttp.RespondWith(200, `{
					"access_token": "some-access-token",
					"token_type": "bearer",
					"expires_in": 3600
				}`,
					http.Header{"Content-Type": []string{"application/json"}},
				),
			),
		}

		rootConfig = initConfig()
		rootConfig.RoutingAPI.ClientID = "my-client"
		rootConfig.RoutingAPI.ClientSecret = "my-secret"
		rootConfig.RoutingAPI.ClientCertificatePath = routingAPIClientCertPath
		rootConfig.RoutingAPI.ClientPrivateKeyPath = routingAPIClientPrivateKeyPath
		rootConfig.RoutingAPI.ServerCACertificatePath = routingAPICAFileName

		port := uint16(1234)
		externalPort := uint16(5678)
		routes := []config.RouteSchema{{
			Name:                 "my-route",
			Type:                 "tcp",
			Port:                 &port,
			ExternalPort:         &externalPort,
			URIs:                 []string{"my-host"},
			RouterGroup:          "my-router-group",
			RegistrationInterval: "100ns",
		}}
		rootConfig.Routes = routes

		natsHost := "127.0.0.1"
		// The server cert and client cert are the same
		natsCAPath, mtlsNATSCertPath, mtlsNATSKeyPath, _ = tls_helpers.GenerateCaAndMutualTlsCerts()
		natsCmd = startNatsTLS(natsHost, natsPort, natsCAPath, mtlsNATSCertPath, mtlsNATSKeyPath)

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
	})

	JustBeforeEach(func() {
		oauthServer.AppendHandlers(oauthHandlers...)
		oauthServer.Start()
		rootConfig.RoutingAPI.OAuthURL = oauthServer.URL()
	})

	AfterEach(func() {
		Expect(natsCmd.Process.Kill()).To(Succeed())
		Expect(os.Remove(mtlsNATSCertPath)).To(Succeed())
		Expect(os.Remove(mtlsNATSKeyPath)).To(Succeed())
		routingAPIServer.Close()
		oauthServer.Close()
	})

	Context("when provided a tcp route", func() {
		JustBeforeEach(func() {
			routingAPIServer.HTTPTestServer.Start()
			rootConfig.RoutingAPI.APIURL = routingAPIServer.URL()
			writeConfig(rootConfig)
		})

		var session *gexec.Session

		BeforeEach(func() {
			var err error
			session, err = registerRoute()
			Expect(err).ShouldNot(HaveOccurred())
		})

		AfterEach(func() {
			session.Kill()
		})

		It("registers it with the routing API", func() {
			Eventually(session.Out).Should(gbytes.Say("Initializing"))
			Eventually(session.Out).Should(gbytes.Say("creating routing API connection"))
			Eventually(session.Out).Should(gbytes.Say("Writing pid"))
			Eventually(session.Out).Should(gbytes.Say("Running"))
			Eventually(session.Out).Should(gbytes.Say("Mapped new router group"))
			Eventually(session.Out).Should(gbytes.Say("Upserted route"))
		})
		Context("when UAA errors intermittently occur", func() {
			BeforeEach(func() {
				oauthHandlers = []http.HandlerFunc{
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", "/oauth/token"),
						ghttp.RespondWith(500, `{}`, http.Header{"Content-Type": []string{"application/json"}}),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", "/oauth/token"),
						ghttp.RespondWith(200, `{
				"access_token": "some-access-token",
				"token_type": "bearer",
				"expires_in": 3600
				}`,
							http.Header{"Content-Type": []string{"application/json"}},
						),
					),
				}
			})
			It("Retries UAA token refreshes if problems were encountered", func() {
				Eventually(session.Out).Should(gbytes.Say("error-fetching-token"))
				Consistently(session.Out, 5*time.Second).ShouldNot(gbytes.Say("token-error"))
			})
		})

		Context("when UAA errors consistently ooccur", func() {
			BeforeEach(func() {
				oauthHandlers = []http.HandlerFunc{
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", "/oauth/token"),
						ghttp.RespondWith(500, `{}`, http.Header{"Content-Type": []string{"application/json"}}),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", "/oauth/token"),
						ghttp.RespondWith(500, `{}`, http.Header{"Content-Type": []string{"application/json"}}),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", "/oauth/token"),
						ghttp.RespondWith(500, `{}`, http.Header{"Content-Type": []string{"application/json"}}),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", "/oauth/token"),
						ghttp.RespondWith(500, `{}`, http.Header{"Content-Type": []string{"application/json"}}),
					),
				}
			})
			It("Gives up and returns a token error", func() {
				Eventually(session.Out, 5*time.Second).Should(gbytes.Say("token-error"))
				Eventually(session.Out, 5*time.Second).Should(gbytes.Say("error\":\"oauth2: cannot fetch token:"))
			})
		})
	})

	Context("when routing API uses TLS", func() {
		Context("when provided a tcp route", func() {
			JustBeforeEach(func() {
				routingAPIServer.HTTPTestServer.StartTLS()
				rootConfig.RoutingAPI.APIURL = routingAPIServer.URL()
				writeConfig(rootConfig)
			})

			var session *gexec.Session

			BeforeEach(func() {
				var err error
				session, err = registerRoute()
				Expect(err).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				session.Kill()
			})

			It("registers it with the routing API", func() {
				Eventually(session.Out).Should(gbytes.Say("Initializing"))
				Eventually(session.Out).Should(gbytes.Say("creating routing API connection"))
				Eventually(session.Out).Should(gbytes.Say("Writing pid"))
				Eventually(session.Out).Should(gbytes.Say("Running"))
				Eventually(session.Out).Should(gbytes.Say("Mapped new router group"))
				Eventually(session.Out).Should(gbytes.Say("Upserted route"))
				// Upserted Route content verified with expected body in the ghttp server setup
			})
		})
	})
})

func registerRoute() (*gexec.Session, error) {
	command := exec.Command(
		routeRegistrarBinPath,
		"-logLevel=debug",
		fmt.Sprintf("-pidfile=%s", pidFile),
		fmt.Sprintf("-configPath=%s", configFile),
	)

	return gexec.Start(command, GinkgoWriter, GinkgoWriter)
}
