package main_test

import (
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"io/ioutil"
	"net/http"
	"os/exec"
	"time"

	"os"

	"code.cloudfoundry.org/routing-release/routing-api"
	"code.cloudfoundry.org/routing-release/routing-api/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
	. "github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/ghttp"
	"github.com/vito/go-sse/sse"
)

var _ = Describe("Main", func() {
	var (
		flags []string
	)

	var buildCommand = func(cmd string, flags []string, args []string) []string {
		command := []string{cmd}
		command = append(command, flags...)
		command = append(command, args...)
		return command
	}

	Context("Given reasonable arguments", func() {
		var (
			server     *ghttp.Server
			authServer *ghttp.Server
			token      string
			caLocation string
		)

		BeforeEach(func() {
			server = ghttp.NewServer()
			token = "some-token"

			authServer = ghttp.NewUnstartedServer()
			caCert, caPrivKey, err := createCA()
			Expect(err).ToNot(HaveOccurred())

			serverCert, err := createCertificate(caCert, caPrivKey, isServer)
			Expect(err).ToNot(HaveOccurred())

			tlsConfig := &tls.Config{
				Certificates: []tls.Certificate{serverCert},
			}
			authServer.AllowUnhandledRequests = true
			authServer.UnhandledRequestStatusCode = http.StatusOK
			authServer.HTTPTestServer.TLS = tlsConfig
			authServer.HTTPTestServer.StartTLS()

			f, err := ioutil.TempFile("", "routing-api-cli-ca")
			Expect(err).ToNot(HaveOccurred())

			caLocation = f.Name()

			err = pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw})
			Expect(err).ToNot(HaveOccurred())

			authServer.RouteToHandler("POST", "/oauth/token",
				func(w http.ResponseWriter, req *http.Request) {
					jsonBytes := []byte(`{"access_token":"some-token", "expires_in":10}`)
					w.Write(jsonBytes)
				})

			flags = []string{
				"-api", server.URL(),
				"-client-id", "some-name",
				"-client-secret", "some-secret",
				"-oauth-url", authServer.URL(),
				"--ca-certs", f.Name(),
			}
		})

		AfterEach(func() {
			authServer.Close()
			server.Close()
			err := os.Remove(caLocation)
			Expect(err).NotTo(HaveOccurred())
		})

		It("successfully requests a token", func() {
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("POST", "/routing/v1/routes"),
					ghttp.VerifyHeader(http.Header{
						"Authorization": []string{"bearer " + token},
					}),
					ghttp.RespondWithJSONEncoded(http.StatusCreated, nil),
				),
			)

			command := buildCommand("register", flags, []string{"[{}]"})
			session := routingAPICLI(command...)

			Eventually(session, "2s").Should(Exit(0))
			Expect(authServer.ReceivedRequests()).To(HaveLen(1))
		})

		It("registers a route to the routing api", func() {
			command := buildCommand("register", flags, []string{`[{"route":"zak.com","port":3,"ip":"4","ttl":1}]`})

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("POST", "/routing/v1/routes"),
					ghttp.VerifyJSONRepresenting([]map[string]interface{}{
						{
							"route":    "zak.com",
							"port":     3,
							"ip":       "4",
							"ttl":      1,
							"log_guid": "",
							"modification_tag": map[string]interface{}{
								"guid":  "",
								"index": 0,
							},
						},
					}),
					ghttp.RespondWithJSONEncoded(http.StatusCreated, nil),
				),
			)

			session := routingAPICLI(command...)

			Eventually(session, "2s").Should(Exit(0))
			Expect(server.ReceivedRequests()).To(HaveLen(1))
		})

		It("registers multiple routes to the routing api", func() {
			routes := `[{"route":"zak.com","port":0,"ip": "","ttl":5,"log_guid":"yo"},{"route":"jak.com","port":8,"ip":"11","ttl":0}]`
			command := buildCommand("register", flags, []string{routes})
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("POST", "/routing/v1/routes"),
					ghttp.VerifyJSONRepresenting([]map[string]interface{}{
						{
							"route":    "zak.com",
							"port":     0,
							"ip":       "",
							"ttl":      5,
							"log_guid": "yo",
							"modification_tag": map[string]interface{}{
								"guid":  "",
								"index": 0,
							},
						},
						{
							"route":    "jak.com",
							"port":     8,
							"ip":       "11",
							"ttl":      0,
							"log_guid": "",
							"modification_tag": map[string]interface{}{
								"guid":  "",
								"index": 0,
							},
						},
					}),
					ghttp.RespondWithJSONEncoded(http.StatusCreated, nil),
				),
			)

			session := routingAPICLI(command...)

			Eventually(session, "2s").Should(Exit(0))
			Expect(string(session.Out.Contents())).To(ContainSubstring("Successfully registered routes: " + routes + "\n"))
			Expect(server.ReceivedRequests()).To(HaveLen(1))
		})

		It("Unregisters a route to the routing api", func() {
			routes := `[{"route":"zak.com","ttl":5,"log_guid":"yo"}]`
			command := buildCommand("unregister", flags, []string{routes})

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("DELETE", "/routing/v1/routes"),
					ghttp.VerifyJSONRepresenting([]map[string]interface{}{
						{
							"route":    "zak.com",
							"port":     0,
							"ip":       "",
							"ttl":      5,
							"log_guid": "yo",
							"modification_tag": map[string]interface{}{
								"guid":  "",
								"index": 0,
							},
						},
					}),
					ghttp.RespondWithJSONEncoded(http.StatusNoContent, nil),
				),
			)

			session := routingAPICLI(command...)

			Eventually(session, "2s").Should(Exit(0))
			Expect(string(session.Out.Contents())).To(ContainSubstring("Successfully unregistered routes: " + routes))
			Expect(server.ReceivedRequests()).To(HaveLen(1))
		})

		Describe("Listing routes", func() {
			BeforeEach(func() {
				os.Unsetenv("RTR_TRACE")
			})
			It("Lists the routes without logging uaa messages", func() {
				routes := []models.Route{
					models.NewRoute("llama.example.com", 0, "", "yo", "", 5),
					models.NewRoute("example.com", 8, "11", "yo", "", 1),
				}
				command := buildCommand("list", flags, []string{})

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/routing/v1/routes"),
						ghttp.RespondWithJSONEncoded(http.StatusOK, routes),
					),
				)

				session := routingAPICLI(command...)

				expectedRoutes, err := json.Marshal(routes)
				Expect(err).ToNot(HaveOccurred())

				Eventually(session, "2s").Should(Exit(0))
				Expect(string(session.Out.Contents())).ToNot(ContainSubstring("uaa-client"))
				Expect(string(session.Out.Contents())).To(ContainSubstring(string(expectedRoutes) + "\n"))
				Expect(server.ReceivedRequests()).To(HaveLen(1))
			})

			Context("with RTR_TRACE=true", func() {
				BeforeEach(func() {
					os.Setenv("RTR_TRACE", "true")
				})
				It("Lists the routes and logs uaa messages", func() {
					routes := []models.Route{
						models.NewRoute("llama.example.com", 0, "", "yo", "", 5),
						models.NewRoute("example.com", 8, "11", "yo", "", 1),
					}
					command := buildCommand("list", flags, []string{})

					server.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("GET", "/routing/v1/routes"),
							ghttp.RespondWithJSONEncoded(http.StatusOK, routes),
						),
					)

					session := routingAPICLI(command...)

					expectedRoutes, err := json.Marshal(routes)
					Expect(err).ToNot(HaveOccurred())

					Eventually(session, "2s").Should(Exit(0))
					Expect(string(session.Out.Contents())).To(ContainSubstring("uaa-client"))
					Expect(string(session.Out.Contents())).To(ContainSubstring(string(expectedRoutes) + "\n"))
					Expect(server.ReceivedRequests()).To(HaveLen(1))
				})
			})
		})

		Context("events", func() {
			var (
				httpEvent          routing_api.Event
				tcpEvent           routing_api.TcpEvent
				httpEventString    []byte
				tcpEventString     []byte
				sseEventHandler    http.HandlerFunc
				sseEventTcpHandler http.HandlerFunc
				headers            http.Header
			)

			BeforeEach(func() {
				var err error

				headers = make(http.Header)
				headers.Set("Content-Type", "text/event-stream; charset=utf-8")

				httpEvent = routing_api.Event{
					Action: "Delete",
					Route:  models.NewRoute("z.a.k", 63, "42.42.42.42", "Tomato", "https://route-service-url.com", 1),
				}

				httpEventString, err = json.Marshal(httpEvent.Route)
				Expect(err).ToNot(HaveOccurred())

				sseEvent := sse.Event{
					Name: httpEvent.Action,
					Data: httpEventString,
				}

				sseEventHandler = ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/routing/v1/events"),
					ghttp.RespondWith(http.StatusOK, sseEvent.Encode(), headers),
				)

				tcpEvent = routing_api.TcpEvent{
					Action: "Upsert",
					TcpRouteMapping: models.NewTcpRouteMapping(
						"some-guid",
						1234,
						"some-ip",
						6789,
						0,
					),
				}

				tcpEventString, err = json.Marshal(tcpEvent.TcpRouteMapping)
				Expect(err).ToNot(HaveOccurred())

				sseEventTcp := sse.Event{
					Name: tcpEvent.Action,
					Data: tcpEventString,
				}
				sseEventTcpHandler = ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/routing/v1/tcp_routes/events"),
					ghttp.RespondWith(http.StatusOK, sseEventTcp.Encode(), headers),
				)
			})

			It("emits an error message on server termination", func() {
				command := buildCommand("events", flags, []string{})

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.RespondWith(http.StatusOK, ""),
					),
					ghttp.CombineHandlers(
						ghttp.RespondWith(http.StatusOK, ""),
					),
				)

				session := routingAPICLI(command...)

				Eventually(session, "2s").Should(Exit(0))
				Expect(server.ReceivedRequests()).To(HaveLen(2))
				Expect(string(session.Out.Contents())).To(ContainSubstring("Connection closed: "))
			})

			Context("when --http flag is provided", func() {
				var flagsWithHttp []string

				BeforeEach(func() {
					flagsWithHttp = append(flags, "--http")
				})

				It("subscribes to HTTP events", func() {
					command := buildCommand("events", flagsWithHttp, []string{})

					server.AppendHandlers(sseEventHandler)

					session := routingAPICLI(command...)

					Eventually(session, "2s").Should(Exit(0))
					Expect(server.ReceivedRequests()).To(HaveLen(1))
					Expect(string(session.Out.Contents())).To(ContainSubstring(string(httpEventString)))
					Expect(string(session.Out.Contents())).NotTo(ContainSubstring(string(tcpEventString)))
				})
			})

			Context("when --tcp flag is provided", func() {
				var flagsWithTcp []string

				BeforeEach(func() {

					server.AppendHandlers(sseEventTcpHandler)
					flagsWithTcp = append(flags, "--tcp")
				})

				It("subscribes to TCP events", func() {
					command := buildCommand("events", flagsWithTcp, []string{})

					session := routingAPICLI(command...)

					Eventually(session, "2s").Should(Exit(0))
					Expect(server.ReceivedRequests()).To(HaveLen(1))
					Expect(string(session.Out.Contents())).To(ContainSubstring(string(tcpEventString)))
					Expect(string(session.Out.Contents())).NotTo(ContainSubstring(string(httpEventString)))
				})
			})

			Context("when both --http and --tcp flags are provided", func() {
				var flagsWithAllProtocols []string

				BeforeEach(func() {
					server.RouteToHandler("GET", "/routing/v1/events", sseEventHandler)
					server.RouteToHandler("GET", "/routing/v1/tcp_routes/events", sseEventTcpHandler)

					flagsWithAllProtocols = append(flags, "--http", "--tcp")
				})

				It("subscribes to HTTP and TCP events", func() {
					command := buildCommand("events", flagsWithAllProtocols, []string{})

					session := routingAPICLI(command...)

					Eventually(session, "2s").Should(Exit(0))
					Expect(server.ReceivedRequests()).To(HaveLen(2))
					Expect(string(session.Out.Contents())).To(ContainSubstring(string(tcpEventString)))
					Expect(string(session.Out.Contents())).To(ContainSubstring(string(httpEventString)))
				})
			})

			Context("when no protocol specific flag is provided", func() {
				BeforeEach(func() {
					server.RouteToHandler("GET", "/routing/v1/events", sseEventHandler)
					server.RouteToHandler("GET", "/routing/v1/tcp_routes/events", sseEventTcpHandler)
				})

				It("subscribes to HTTP and TCP events", func() {
					command := buildCommand("events", flags, []string{})

					session := routingAPICLI(command...)

					Eventually(session, "2s").Should(Exit(0))
					Expect(server.ReceivedRequests()).To(HaveLen(2))
					Expect(string(session.Out.Contents())).To(ContainSubstring(string(tcpEventString)))
					Expect(string(session.Out.Contents())).To(ContainSubstring(string(httpEventString)))
				})
			})
		})

		Context("with --skip-tls-verification without a provided custom CA", func() {
			var (
				tlsServer *ghttp.Server
			)

			AfterEach(func() {
				tlsServer.Close()
			})

			BeforeEach(func() {
				tlsServer = ghttp.NewTLSServer()
				flags = []string{
					"-api", tlsServer.URL(),
					"-client-id", "some-name",
					"-client-secret", "some-secret",
					"-oauth-url", authServer.URL(),
					"--skip-tls-verification",
				}
				createHttpRoutesHandler := ghttp.CombineHandlers(
					ghttp.VerifyRequest("POST", "/routing/v1/routes"),
					ghttp.VerifyHeader(http.Header{
						"Authorization": []string{"bearer " + token},
					}),
					ghttp.RespondWithJSONEncoded(http.StatusCreated, nil),
				)

				headers := make(http.Header)
				headers.Set("Content-Type", "text/event-stream; charset=utf-8")

				httpEvent := routing_api.Event{
					Action: "Delete",
					Route:  models.NewRoute("z.a.k", 63, "42.42.42.42", "Tomato", "https://route-service-url.com", 1),
				}

				httpEventString, err := json.Marshal(httpEvent.Route)
				Expect(err).ToNot(HaveOccurred())

				sseEvent := sse.Event{
					Name: httpEvent.Action,
					Data: httpEventString,
				}

				sseEventHandler := ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/routing/v1/events"),
					ghttp.RespondWith(http.StatusOK, sseEvent.Encode(), headers),
				)

				tlsServer.RouteToHandler("POST", "/routing/v1/routes", createHttpRoutesHandler)
				tlsServer.RouteToHandler("GET", "/routing/v1/events", sseEventHandler)
			})

			It("successfully requests a token", func() {
				command := buildCommand("register", flags, []string{"[{}]"})
				session := routingAPICLI(command...)

				Eventually(session, "2s").Should(Exit(0))
				Expect(authServer.ReceivedRequests()).To(HaveLen(1))
			})

			It("successfully connects to routing api", func() {
				command := buildCommand("register", flags, []string{"[{}]"})
				session := routingAPICLI(command...)

				Eventually(session, "2s").Should(Exit(0))
				Expect(authServer.ReceivedRequests()).To(HaveLen(1))
				Expect(tlsServer.ReceivedRequests()).To(HaveLen(1))
			})

			It("successfully streams events from the routing api", func() {
				command := buildCommand("events", flags, []string{"--http"})
				session := routingAPICLI(command...)

				Eventually(session, "2s").Should(Exit(0))
				Expect(authServer.ReceivedRequests()).To(HaveLen(1))
				Expect(tlsServer.ReceivedRequests()).To(HaveLen(1))
			})
		})

		Context("environment variables", func() {
			Context("RTR_TRACE", func() {
				var session *Session
				BeforeEach(func() {
					routes := []models.Route{
						models.NewRoute("llama.example.com", 0, "", "yo", "", 5),
						models.NewRoute("example.com", 8, "11", "yo", "", 1),
					}
					server.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("GET", "/routing/v1/routes"),
							ghttp.RespondWithJSONEncoded(http.StatusOK, routes),
						),
					)
				})

				JustBeforeEach(func() {
					command := buildCommand("list", flags, []string{})
					session = routingAPICLI(command...)
					Eventually(session, "2s").Should(Exit(0))
				})

				Context("when RTR_TRACE is not set", func() {
					BeforeEach(func() {
						os.Unsetenv("RTR_TRACE")
					})

					It("should not trace the requests made/responses received", func() {
						Expect(string(session.Out.Contents())).NotTo(ContainSubstring("REQUEST"))
					})
				})

				Context("when RTR_TRACE is set to true", func() {
					BeforeEach(func() {
						os.Setenv("RTR_TRACE", "true")
					})

					It("should trace the requests made/responses received", func() {
						Expect(string(session.Out.Contents())).To(ContainSubstring("REQUEST"))
					})
				})

				Context("when RTR_TRACE is set to false", func() {
					BeforeEach(func() {
						os.Setenv("RTR_TRACE", "false")
					})

					It("should not trace the requests made/responses received", func() {
						Expect(string(session.Out.Contents())).NotTo(ContainSubstring("REQUEST"))
					})
				})

				Context("when RTR_TRACE is set to an invalid value", func() {
					BeforeEach(func() {
						os.Setenv("RTR_TRACE", "adsf")
					})

					It("should not trace the requests made/responses received", func() {
						Expect(string(session.Out.Contents())).NotTo(ContainSubstring("REQUEST"))
					})
				})
			})
		})
	})

	Context("Given unreasonable arguments", func() {
		BeforeEach(func() {
			flags = []string{
				"-api", "some-server-name",
				"-client-id", "some-name",
				"-client-secret", "some-secret",
				"-oauth-url", "invalid-url",
				"--skip-tls-verification",
			}
		})

		Context("when no API endpoint is specified", func() {
			BeforeEach(func() {
				flags = []string{
					"-client-id", "some-name",
					"-client-secret", "some-secret",
					"-oauth-url", "http://some.oauth.url",
				}
			})

			It("checks for the presence of api", func() {
				command := buildCommand("register", []string{}, []string{})
				session := routingAPICLI(command...)

				Eventually(session).Should(Exit(1))
				Eventually(session).Should(Say("Must provide an API endpoint for the routing-api component.\n"))
			})
		})

		Context("when no flags are given", func() {
			It("tells you everything you did wrong", func() {
				session := routingAPICLI("register")

				Eventually(session).Should(Exit(1))
				contents := session.Out.Contents()
				Expect(contents).To(ContainSubstring("Must provide an API endpoint for the routing-api component.\n"))
				Expect(contents).To(ContainSubstring("Must provide the id of an OAuth client.\n"))
				Expect(contents).To(ContainSubstring("Must provide an OAuth secret.\n"))
				Expect(contents).To(ContainSubstring("Must provide an URL to the OAuth client.\n"))
			})
		})

		It("checks for a valid command", func() {
			session := routingAPICLI("not-a-command")

			Eventually(session).Should(Exit(1))
			Eventually(session).Should(Say("Not a valid command: not-a-command"))
		})

		It("outputs help info for a valid command", func() {
			session := routingAPICLI("register")

			Eventually(session).Should(Exit(1))
			Eventually(session).Should(Say("register"))
		})

		It("outputs help info for a valid command", func() {
			session := routingAPICLI("events")

			Eventually(session).Should(Exit(1))
			Eventually(session).Should(Say("events"))
		})

		It("outputs help info for a valid command", func() {
			session := routingAPICLI("unregister")

			Eventually(session).Should(Exit(1))
			Eventually(session).Should(Say("unregister"))
		})

		Context("register", func() {
			It("checks for the presence of the route json", func() {
				command := buildCommand("register", flags, []string{})
				session := routingAPICLI(command...)

				Eventually(session).Should(Exit(1))
				Eventually(session).Should(Say("Must provide routes JSON."))
			})

			It("fails if the request has invalid json", func() {
				command := buildCommand("register", flags, []string{`[{"kind":"of","valid":"json}]`})
				session := routingAPICLI(command...)

				Eventually(session).Should(Exit(3))
				Eventually(session).Should(Say("unexpected end of JSON input"))
			})

			It("fails if there are unexpected arguments", func() {
				command := buildCommand("register", flags, []string{`[{"kind":"of","valid":"json}]`, "ice cream"})
				session := routingAPICLI(command...)

				Eventually(session).Should(Exit(1))
				Eventually(session).Should(Say("Unexpected arguments."))
			})

			It("shows the error if registration fails", func() {
				command := buildCommand("register", flags, []string{"[{}]"})
				session := routingAPICLI(command...)

				Eventually(session, 5*time.Second).Should(Exit(3))
				Eventually(session).Should(Say("route registration failed:"))
			})
		})

		Context("unregister", func() {
			It("checks for the presence of the route json", func() {
				command := buildCommand("unregister", flags, []string{})
				session := routingAPICLI(command...)

				Eventually(session).Should(Exit(1))
				Eventually(session).Should(Say("Must provide routes JSON."))
			})

			It("fails if the unregister request has invalid json", func() {
				command := buildCommand("unregister", flags, []string{`[{"kind":"of","valid":"json}]`})
				session := routingAPICLI(command...)

				Eventually(session).Should(Exit(3))
				Eventually(session).Should(Say("unexpected end of JSON input"))
			})

			It("fails if there are unexpected arguments", func() {
				command := buildCommand("unregister", flags, []string{`[{"kind":"of","valid":"json}]`, "ice cream"})
				session := routingAPICLI(command...)

				Eventually(session).Should(Exit(1))
				Eventually(session).Should(Say("Unexpected arguments."))
			})

			It("shows the error if unregistration fails", func() {
				command := buildCommand("unregister", flags, []string{"[{}]"})
				session := routingAPICLI(command...)

				Eventually(session, 5*time.Second).Should(Say("route unregistration failed:"))
				Eventually(session, 5*time.Second).Should(Exit(3))
			})
		})

		Context("events", func() {
			It("fails if there are unexpected arguments", func() {
				command := buildCommand("events", flags, []string{"ice cream"})
				session := routingAPICLI(command...)

				Eventually(session).Should(Exit(1))
				Eventually(session).Should(Say("Unexpected arguments."))
			})

			It("shows the error if streaming events fails", func() {
				command := buildCommand("events", flags, []string{})
				session := routingAPICLI(command...)

				Eventually(session, 5*time.Second).Should(Exit(3))
				Eventually(session).Should(Say("streaming events failed"))
			})
		})

		Context("list", func() {
			It("fails if there are unexpected arguments", func() {
				command := buildCommand("list", flags, []string{"ice cream"})
				session := routingAPICLI(command...)

				Eventually(session).Should(Exit(1))
				Eventually(session).Should(Say("Unexpected arguments."))
			})

			It("shows the error if listing routes fails", func() {
				command := buildCommand("list", flags, []string{})
				session := routingAPICLI(command...)

				Eventually(session, 5*time.Second).Should(Exit(3))
				Eventually(session).Should(Say("listing routes failed:"))
			})
		})
	})
})

func routingAPICLI(args ...string) *Session {
	session, err := Start(exec.Command(path, args...), GinkgoWriter, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred())

	return session
}

func verifyBody(expectedBody string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		Expect(err).ToNot(HaveOccurred())

		defer r.Body.Close()
		Expect(string(body)).To(Equal(expectedBody))
	}
}
