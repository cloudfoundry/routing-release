package routing_api_test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"code.cloudfoundry.org/routing-release/routing-api"
	"code.cloudfoundry.org/routing-release/routing-api/models"
	trace "code.cloudfoundry.org/trace-logger"
	"github.com/onsi/gomega/ghttp"
	"github.com/vito/go-sse/sse"
)

const (
	DefaultRouterGroupGuid = "bad25cff-9332-48a6-8603-b619858e7992"
	DefaultRouterGroupName = "default-tcp"
	DefaultRouterGroupType = "tcp"
)

var _ = Describe("Client", func() {
	const (
		ROUTES_API_URL                    = "/routing/v1/routes"
		TCP_CREATE_ROUTE_MAPPINGS_API_URL = "/routing/v1/tcp_routes/create"
		TCP_DELETE_ROUTE_MAPPINGS_API_URL = "/routing/v1/tcp_routes/delete"
		TCP_ROUTES_API_URL                = "/routing/v1/tcp_routes"
		ROUTER_GROUPS_API_URL             = "/routing/v1/router_groups"
		EVENTS_SSE_URL                    = "/routing/v1/events"
		TCP_EVENTS_SSE_URL                = "/routing/v1/tcp_routes/events"
	)

	var server *ghttp.Server
	var client routing_api.Client
	var route1 models.Route
	var route2 models.Route
	var stdout *bytes.Buffer

	BeforeEach(func() {
		stdout = bytes.NewBuffer([]byte{})
		trace.SetStdout(stdout)
		trace.Logger = trace.NewLogger("true")
	})

	BeforeEach(func() {
		route1 = models.NewRoute("a.b.c", 33, "1.1.1.1", "potato", "", 55)
		route2 = models.NewRoute("d.e.f", 35, "2.2.2.2", "banana", "", 66)
		server = ghttp.NewServer()
		client = routing_api.NewClient(server.URL(), false)
	})

	AfterEach(func() {
		server.Close()
	})

	Context("UpsertRoutes", func() {
		var err error
		JustBeforeEach(func() {
			err = client.UpsertRoutes([]models.Route{route1, route2})
		})

		Context("when the server returns a valid response", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.VerifyRequest("POST", ROUTES_API_URL),
				)
			})

			It("sends an Upsert request to the server", func() {
				Expect(server.ReceivedRequests()).Should(HaveLen(1))
			})

			It("does not receive an error", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("logs the request and response", func() {
				expectedBody, _ := json.Marshal([]models.Route{route1, route2})

				r, err := ioutil.ReadAll(stdout)
				log := string(r)
				Expect(err).NotTo(HaveOccurred())
				Expect(log).To(ContainSubstring("REQUEST: "))
				Expect(log).To(ContainSubstring("POST " + ROUTES_API_URL + " HTTP/1.1"))
				Expect(log).To(ContainSubstring(string(expectedBody)))

				Expect(log).To(ContainSubstring("RESPONSE: "))
				Expect(log).To(ContainSubstring("HTTP/1.1 200 OK"))
			})
		})

		Context("When the server returns an error", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", ROUTES_API_URL),
						ghttp.RespondWith(http.StatusBadRequest, "random error message"),
					),
				)
			})

			It("receives an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("random error message"))
			})

			It("logs the request and response", func() {
				expectedBody, _ := json.Marshal([]models.Route{route1, route2})

				r, err := ioutil.ReadAll(stdout)
				log := string(r)
				Expect(err).NotTo(HaveOccurred())
				Expect(log).To(ContainSubstring("REQUEST: "))
				Expect(log).To(ContainSubstring("POST " + ROUTES_API_URL + " HTTP/1.1"))
				Expect(log).To(ContainSubstring(string(expectedBody)))

				Expect(log).To(ContainSubstring("RESPONSE: "))
				Expect(log).To(ContainSubstring("HTTP/1.1 400 Bad Request"))
			})

			Context("when the error is valid JSON but not a routing api error", func() {
				BeforeEach(func() {
					server.SetHandler(0,
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("POST", ROUTES_API_URL),
							ghttp.RespondWith(http.StatusNotFound, `{"code": 100000}`),
						),
					)
				})

				It("returns a useful error message", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(Equal(`{"code": 100000}`))
				})
			})
		})
	})

	Context("UpsertTcpRouteMappings", func() {

		var (
			err              error
			tcpRouteMapping1 models.TcpRouteMapping
			tcpRouteMapping2 models.TcpRouteMapping
		)
		BeforeEach(func() {
			tcpRouteMapping1 = models.NewTcpRouteMapping("router-group-guid-001", 52000, "1.2.3.4", 60000, 60)
			tcpRouteMapping2 = models.NewTcpRouteMapping("router-group-guid-001", 52001, "1.2.3.5", 60001, 60)
		})

		JustBeforeEach(func() {
			err = client.UpsertTcpRouteMappings([]models.TcpRouteMapping{tcpRouteMapping1, tcpRouteMapping2})
		})

		Context("when the server returns a valid response", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.VerifyRequest("POST", TCP_CREATE_ROUTE_MAPPINGS_API_URL),
				)
			})

			It("sends an Upsert request to the server", func() {
				Expect(server.ReceivedRequests()).Should(HaveLen(1))
			})

			It("does not receive an error", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("logs the request and response", func() {
				expectedBody, _ := json.Marshal([]models.TcpRouteMapping{tcpRouteMapping1, tcpRouteMapping2})

				r, err := ioutil.ReadAll(stdout)
				log := string(r)
				Expect(err).NotTo(HaveOccurred())
				Expect(log).To(ContainSubstring("REQUEST: "))
				Expect(log).To(ContainSubstring("POST " + TCP_CREATE_ROUTE_MAPPINGS_API_URL + " HTTP/1.1"))
				Expect(log).To(ContainSubstring(string(expectedBody)))

				Expect(log).To(ContainSubstring("RESPONSE: "))
				Expect(log).To(ContainSubstring("HTTP/1.1 200 OK"))
			})
		})

		Context("when the server returns an error", func() {
			Context("other than unauthorized", func() {
				BeforeEach(func() {
					server.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("POST", TCP_CREATE_ROUTE_MAPPINGS_API_URL),
							ghttp.RespondWith(http.StatusBadRequest, nil),
						),
					)
				})

				It("receives an error", func() {
					Expect(err).To(HaveOccurred())
				})

				It("logs the request and response", func() {
					expectedBody, _ := json.Marshal([]models.TcpRouteMapping{tcpRouteMapping1, tcpRouteMapping2})

					r, err := ioutil.ReadAll(stdout)
					log := string(r)
					Expect(err).NotTo(HaveOccurred())
					Expect(log).To(ContainSubstring("REQUEST: "))
					Expect(log).To(ContainSubstring("POST " + TCP_CREATE_ROUTE_MAPPINGS_API_URL + " HTTP/1.1"))
					Expect(log).To(ContainSubstring(string(expectedBody)))

					Expect(log).To(ContainSubstring("RESPONSE: "))
					Expect(log).To(ContainSubstring("HTTP/1.1 400 Bad Request"))
				})
			})

			Context("unauthorized", func() {
				BeforeEach(func() {
					server.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("POST", TCP_CREATE_ROUTE_MAPPINGS_API_URL),
							ghttp.RespondWith(http.StatusUnauthorized, nil),
						),
					)
				})

				It("receives an unauthorized error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).Should(Equal("unauthorized"))
				})

				It("logs the request and response", func() {
					expectedBody, _ := json.Marshal([]models.TcpRouteMapping{tcpRouteMapping1, tcpRouteMapping2})

					r, err := ioutil.ReadAll(stdout)
					log := string(r)
					Expect(err).NotTo(HaveOccurred())
					Expect(log).To(ContainSubstring("REQUEST: "))
					Expect(log).To(ContainSubstring("POST " + TCP_CREATE_ROUTE_MAPPINGS_API_URL + " HTTP/1.1"))
					Expect(log).To(ContainSubstring(string(expectedBody)))

					Expect(log).To(ContainSubstring("RESPONSE: "))
					Expect(log).To(ContainSubstring("HTTP/1.1 401 Unauthorized"))
				})
			})
		})
	})

	Context("DeleteRoutes", func() {
		var err error
		JustBeforeEach(func() {
			err = client.DeleteRoutes([]models.Route{route1, route2})
		})

		Context("when the server returns a valid response", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("DELETE", ROUTES_API_URL),
						ghttp.VerifyJSONRepresenting([]models.Route{route1, route2}),
					),
				)
			})

			It("sends a Delete request to the server", func() {
				Expect(server.ReceivedRequests()).Should(HaveLen(1))
			})

			It("does not receive an error", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("logs the request and response", func() {
				expectedBody, _ := json.Marshal([]models.Route{route1, route2})

				r, err := ioutil.ReadAll(stdout)
				log := string(r)
				Expect(err).NotTo(HaveOccurred())
				Expect(log).To(ContainSubstring("REQUEST: "))
				Expect(log).To(ContainSubstring("DELETE " + ROUTES_API_URL + " HTTP/1.1"))
				Expect(log).To(ContainSubstring(string(expectedBody)))

				Expect(log).To(ContainSubstring("RESPONSE: "))
				Expect(log).To(ContainSubstring("HTTP/1.1 200 OK"))
			})
		})

		Context("When the server returns an error", func() {
			Context("other than unauthorized", func() {
				BeforeEach(func() {
					server.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("DELETE", ROUTES_API_URL),
							ghttp.RespondWith(http.StatusBadRequest, nil),
						),
					)
				})

				It("receives an error", func() {
					Expect(err).To(HaveOccurred())
				})

				It("logs the request and response", func() {
					expectedBody, _ := json.Marshal([]models.Route{route1, route2})

					r, err := ioutil.ReadAll(stdout)
					log := string(r)
					Expect(err).NotTo(HaveOccurred())
					Expect(log).To(ContainSubstring("REQUEST: "))
					Expect(log).To(ContainSubstring("DELETE " + ROUTES_API_URL + " HTTP/1.1"))
					Expect(log).To(ContainSubstring(string(expectedBody)))

					Expect(log).To(ContainSubstring("RESPONSE: "))
					Expect(log).To(ContainSubstring("HTTP/1.1 400 Bad Request"))
				})
			})

			Context("unauthorized", func() {
				BeforeEach(func() {
					server.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.VerifyRequest("DELETE", ROUTES_API_URL),
							ghttp.RespondWith(http.StatusUnauthorized, nil),
						),
					)
				})

				It("receives an unauthorized error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).Should(Equal("unauthorized"))
				})

				It("logs the request and response", func() {
					expectedBody, _ := json.Marshal([]models.Route{route1, route2})

					r, err := ioutil.ReadAll(stdout)
					log := string(r)
					Expect(err).NotTo(HaveOccurred())
					Expect(log).To(ContainSubstring("REQUEST: "))
					Expect(log).To(ContainSubstring("DELETE " + ROUTES_API_URL + " HTTP/1.1"))
					Expect(log).To(ContainSubstring(string(expectedBody)))

					Expect(log).To(ContainSubstring("RESPONSE: "))
					Expect(log).To(ContainSubstring("HTTP/1.1 401 Unauthorized"))
				})
			})
		})
	})

	Context("DeleteTcpRouteMappings", func() {
		var (
			err              error
			tcpRouteMapping1 models.TcpRouteMapping
			tcpRouteMapping2 models.TcpRouteMapping
		)
		BeforeEach(func() {
			tcpRouteMapping1 = models.NewTcpRouteMapping("router-group-guid-001", 52000, "1.2.3.4", 60000, 60)
			tcpRouteMapping2 = models.NewTcpRouteMapping("router-group-guid-001", 52001, "1.2.3.5", 60001, 60)
		})
		JustBeforeEach(func() {
			err = client.DeleteTcpRouteMappings([]models.TcpRouteMapping{tcpRouteMapping1, tcpRouteMapping2})
		})

		Context("when the server returns a valid response", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", TCP_DELETE_ROUTE_MAPPINGS_API_URL),
						func(w http.ResponseWriter, req *http.Request) {
							w.WriteHeader(http.StatusNoContent)
						},
					),
				)
			})

			It("sends a Delete request to the server", func() {
				Expect(server.ReceivedRequests()).Should(HaveLen(1))
			})

			It("does not receive an error", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("logs the request and response", func() {
				expectedBody, _ := json.Marshal([]models.TcpRouteMapping{tcpRouteMapping1, tcpRouteMapping2})

				r, err := ioutil.ReadAll(stdout)
				log := string(r)
				Expect(err).NotTo(HaveOccurred())
				Expect(log).To(ContainSubstring("REQUEST: "))
				Expect(log).To(ContainSubstring("POST " + TCP_DELETE_ROUTE_MAPPINGS_API_URL + " HTTP/1.1"))
				Expect(log).To(ContainSubstring(string(expectedBody)))

				Expect(log).To(ContainSubstring("RESPONSE: "))
				Expect(log).To(ContainSubstring("HTTP/1.1 204 No Content"))
			})
		})

		Context("When the server returns an error", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("POST", TCP_DELETE_ROUTE_MAPPINGS_API_URL),
						ghttp.RespondWith(http.StatusBadRequest, nil),
					),
				)
			})

			It("receives an error", func() {
				Expect(err).To(HaveOccurred())
			})

			It("logs the request and response", func() {
				expectedBody, _ := json.Marshal([]models.TcpRouteMapping{tcpRouteMapping1, tcpRouteMapping2})

				r, err := ioutil.ReadAll(stdout)
				log := string(r)
				Expect(err).NotTo(HaveOccurred())
				Expect(log).To(ContainSubstring("REQUEST: "))
				Expect(log).To(ContainSubstring("POST " + TCP_DELETE_ROUTE_MAPPINGS_API_URL + " HTTP/1.1"))
				Expect(log).To(ContainSubstring(string(expectedBody)))

				Expect(log).To(ContainSubstring("RESPONSE: "))
				Expect(log).To(ContainSubstring("HTTP/1.1 400 Bad Request"))
			})
		})
	})

	Context("Routes", func() {
		var routes []models.Route
		var err error
		var data []byte

		Context("when the server returns a valid response", func() {
			BeforeEach(func() {
				data, _ = json.Marshal([]models.Route{route1, route2})

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTES_API_URL),
						ghttp.RespondWith(http.StatusOK, data),
					),
				)
			})

			It("Sends a ListRoutes request to the server", func() {
				routes, err = client.Routes()
				Expect(err).NotTo(HaveOccurred())
				Expect(server.ReceivedRequests()).Should(HaveLen(1))
			})

			It("gets a list of routes from the server", func() {
				routes, err = client.Routes()
				Expect(err).NotTo(HaveOccurred())
				Expect(routes).To(Equal([]models.Route{route1, route2}))
			})

			It("does not send a body in the request", func() {
				server.SetHandler(0,
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTES_API_URL),
						ghttp.VerifyBody([]byte{}),
						ghttp.RespondWith(http.StatusOK, data),
					),
				)
				routes, err = client.Routes()
				Expect(err).NotTo(HaveOccurred())
			})

			It("logs the request and response", func() {
				routes, err = client.Routes()
				Expect(err).NotTo(HaveOccurred())
				expectedBody, _ := json.Marshal([]models.Route{route1, route2})

				r, err := ioutil.ReadAll(stdout)
				log := string(r)
				Expect(err).NotTo(HaveOccurred())
				Expect(log).To(ContainSubstring("REQUEST: "))
				Expect(log).To(ContainSubstring("GET " + ROUTES_API_URL + " HTTP/1.1"))

				Expect(log).To(ContainSubstring("RESPONSE: "))
				Expect(log).To(ContainSubstring("HTTP/1.1 200 OK"))
				Expect(log).To(ContainSubstring(string(expectedBody)))
			})
		})

		Context("When the server returns an error", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTES_API_URL),
						ghttp.RespondWith(http.StatusBadRequest, nil),
					),
				)
			})

			It("returns an error", func() {
				routes, err = client.Routes()
				Expect(err).To(HaveOccurred())
				Expect(routes).To(BeEmpty())
			})

			It("logs the request and response", func() {
				routes, err = client.Routes()
				Expect(err).To(HaveOccurred())
				expectedBody, _ := json.Marshal([]models.Route{route1, route2})

				r, err := ioutil.ReadAll(stdout)
				log := string(r)
				Expect(err).NotTo(HaveOccurred())
				Expect(log).To(ContainSubstring("REQUEST: "))
				Expect(log).To(ContainSubstring("GET " + ROUTES_API_URL + " HTTP/1.1"))

				Expect(log).To(ContainSubstring("RESPONSE: "))
				Expect(log).To(ContainSubstring("HTTP/1.1 400 Bad Request"))
				Expect(log).NotTo(ContainSubstring(string(expectedBody)))
			})
		})
	})

	Context("TcpRouteMappings", func() {

		var (
			err              error
			tcpRouteMapping1 models.TcpRouteMapping
			tcpRouteMapping2 models.TcpRouteMapping
			routes           []models.TcpRouteMapping
			data             []byte
		)
		BeforeEach(func() {
			tcpRouteMapping1 = models.NewTcpRouteMapping("router-group-guid-001", 52000, "1.2.3.4", 60000, 60)
			tcpRouteMapping2 = models.NewTcpRouteMapping("router-group-guid-001", 52001, "1.2.3.5", 60001, 60)
		})

		Context("when the server returns a valid response", func() {
			BeforeEach(func() {
				data, _ = json.Marshal([]models.TcpRouteMapping{tcpRouteMapping1, tcpRouteMapping2})

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", TCP_ROUTES_API_URL),
						ghttp.RespondWith(http.StatusOK, data),
					),
				)
			})

			It("sends a ListRoutes request to the server", func() {
				routes, err = client.TcpRouteMappings()
				Expect(err).NotTo(HaveOccurred())
				Expect(server.ReceivedRequests()).Should(HaveLen(1))
				Expect(routes).To(Equal([]models.TcpRouteMapping{tcpRouteMapping1, tcpRouteMapping2}))
			})

			It("can filter routes from the server", func() {
				tcpRouteMapping1.IsolationSegment = "is1"

				data, _ = json.Marshal([]models.TcpRouteMapping{tcpRouteMapping1})
				server.SetHandler(0,
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", TCP_ROUTES_API_URL, "isolation_segment=is1"),
						ghttp.RespondWith(http.StatusOK, data),
					),
				)
				routes, err = client.FilteredTcpRouteMappings([]string{"is1"})
				Expect(err).NotTo(HaveOccurred())
				Expect(server.ReceivedRequests()).Should(HaveLen(1))
				Expect(routes).To(Equal([]models.TcpRouteMapping{tcpRouteMapping1}))
			})

			It("does not send a body in the request", func() {
				server.SetHandler(0,
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", TCP_ROUTES_API_URL),
						ghttp.VerifyBody([]byte{}),
						ghttp.RespondWith(http.StatusOK, data),
					),
				)
				routes, err = client.TcpRouteMappings()
				Expect(err).NotTo(HaveOccurred())
			})

			It("logs the request and response", func() {
				routes, err = client.TcpRouteMappings()
				Expect(err).NotTo(HaveOccurred())
				expectedBody, _ := json.Marshal([]models.TcpRouteMapping{tcpRouteMapping1, tcpRouteMapping2})

				r, err := ioutil.ReadAll(stdout)
				log := string(r)
				Expect(err).NotTo(HaveOccurred())
				Expect(log).To(ContainSubstring("REQUEST: "))
				Expect(log).To(ContainSubstring("GET " + TCP_ROUTES_API_URL + " HTTP/1.1"))

				Expect(log).To(ContainSubstring("RESPONSE: "))
				Expect(log).To(ContainSubstring("HTTP/1.1 200 OK"))
				Expect(log).To(ContainSubstring(string(expectedBody)))
			})
		})

		Context("When the server returns an error", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", TCP_ROUTES_API_URL),
						ghttp.RespondWith(http.StatusBadRequest, nil),
					),
				)
			})

			It("returns an error", func() {
				routes, err = client.TcpRouteMappings()
				Expect(err).To(HaveOccurred())
				Expect(routes).To(BeEmpty())
			})

			It("logs the request and response", func() {
				routes, err = client.TcpRouteMappings()
				Expect(err).To(HaveOccurred())
				expectedBody, _ := json.Marshal([]models.TcpRouteMapping{tcpRouteMapping1, tcpRouteMapping2})

				r, err := ioutil.ReadAll(stdout)
				log := string(r)
				Expect(err).NotTo(HaveOccurred())
				Expect(log).To(ContainSubstring("REQUEST: "))
				Expect(log).To(ContainSubstring("GET " + TCP_ROUTES_API_URL + " HTTP/1.1"))

				Expect(log).To(ContainSubstring("RESPONSE: "))
				Expect(log).To(ContainSubstring("HTTP/1.1 400 Bad Request"))
				Expect(log).NotTo(ContainSubstring(string(expectedBody)))
			})
		})
	})

	Context("RouterGroups", func() {
		var (
			routerGroups []models.RouterGroup
			err          error
			routerGroup1 models.RouterGroup
			data         []byte
		)

		BeforeEach(func() {
			routerGroup1 = models.RouterGroup{
				Guid:            DefaultRouterGroupGuid,
				Name:            DefaultRouterGroupName,
				Type:            DefaultRouterGroupType,
				ReservablePorts: "1024-65535",
			}
		})

		Context("when the server returns a valid response", func() {
			BeforeEach(func() {
				data, _ = json.Marshal([]models.RouterGroup{routerGroup1})

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL),
						ghttp.RespondWith(http.StatusOK, data),
					),
				)
			})

			It("Sends a ListRouterGroups request to the server", func() {
				routerGroups, err = client.RouterGroups()
				Expect(err).NotTo(HaveOccurred())
				Expect(server.ReceivedRequests()).Should(HaveLen(1))
			})

			It("gets a list of router groups from the server", func() {
				routerGroups, err = client.RouterGroups()
				Expect(err).NotTo(HaveOccurred())
				Expect(routerGroups).To(Equal([]models.RouterGroup{routerGroup1}))
			})

			It("does not send a body in the request", func() {
				server.SetHandler(0,
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL),
						ghttp.VerifyBody([]byte{}),
						ghttp.RespondWith(http.StatusOK, data),
					),
				)
				routerGroups, err = client.RouterGroups()
				Expect(err).NotTo(HaveOccurred())
			})

			It("logs the request and response", func() {
				routerGroups, err = client.RouterGroups()
				Expect(err).NotTo(HaveOccurred())
				expectedBody, _ := json.Marshal([]models.RouterGroup{routerGroup1})

				r, err := ioutil.ReadAll(stdout)
				log := string(r)
				Expect(err).NotTo(HaveOccurred())
				Expect(log).To(ContainSubstring("REQUEST: "))
				Expect(log).To(ContainSubstring("GET " + ROUTER_GROUPS_API_URL + " HTTP/1.1"))

				Expect(log).To(ContainSubstring("RESPONSE: "))
				Expect(log).To(ContainSubstring("HTTP/1.1 200 OK"))
				Expect(log).To(ContainSubstring(string(expectedBody)))
			})
		})

		Context("When the server returns an error", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL),
						ghttp.RespondWith(http.StatusInternalServerError, nil),
					),
				)
			})

			It("returns an error", func() {
				routerGroups, err = client.RouterGroups()
				Expect(err).To(HaveOccurred())
				Expect(routerGroups).To(BeEmpty())
			})

			It("logs the request and response", func() {
				routerGroups, err = client.RouterGroups()
				Expect(err).To(HaveOccurred())
				expectedBody, _ := json.Marshal([]models.RouterGroup{routerGroup1})

				r, err := ioutil.ReadAll(stdout)
				log := string(r)
				Expect(err).NotTo(HaveOccurred())
				Expect(log).To(ContainSubstring("REQUEST: "))
				Expect(log).To(ContainSubstring("GET " + ROUTER_GROUPS_API_URL + " HTTP/1.1"))

				Expect(log).To(ContainSubstring("RESPONSE: "))
				Expect(log).To(ContainSubstring("HTTP/1.1 500 Internal Server Error"))
				Expect(log).NotTo(ContainSubstring(string(expectedBody)))
			})
		})
	})

	Context("RouterGroupWithName", func() {
		var (
			routerGroup1 models.RouterGroup
			data         []byte
		)

		BeforeEach(func() {
			routerGroup1 = models.RouterGroup{
				Guid:            DefaultRouterGroupGuid,
				Name:            "pineapple",
				Type:            DefaultRouterGroupType,
				ReservablePorts: "1024-65535",
			}
		})

		Context("when the server returns a valid response", func() {
			BeforeEach(func() {
				data, _ = json.Marshal([]models.RouterGroup{routerGroup1})

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL, "name=pineapple"),
						ghttp.RespondWith(http.StatusOK, data),
					),
				)
			})

			It("Sends a ListRouterGroups request to the server with the name query", func() {
				_, err := client.RouterGroupWithName("pineapple")
				Expect(err).NotTo(HaveOccurred())
				Expect(server.ReceivedRequests()).Should(HaveLen(1))
			})

			It("gets the router group with the name from the server", func() {
				routerGroup, err := client.RouterGroupWithName("pineapple")
				Expect(err).NotTo(HaveOccurred())
				Expect(routerGroup).To(Equal(routerGroup1))
			})

			It("logs the request and response", func() {
				_, err := client.RouterGroupWithName("pineapple")
				Expect(err).NotTo(HaveOccurred())
				expectedBody, _ := json.Marshal(routerGroup1)

				r, err := ioutil.ReadAll(stdout)
				log := string(r)
				Expect(err).NotTo(HaveOccurred())
				Expect(log).To(ContainSubstring("REQUEST: "))
				Expect(log).To(ContainSubstring("GET " + ROUTER_GROUPS_API_URL + "?name=pineapple HTTP/1.1"))

				Expect(log).To(ContainSubstring("RESPONSE: "))
				Expect(log).To(ContainSubstring("HTTP/1.1 200 OK"))
				Expect(log).To(ContainSubstring(string(expectedBody)))
			})
		})

		Context("When the server returns an error", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL, "name=pineapple"),
						ghttp.RespondWith(http.StatusInternalServerError, nil),
					),
				)
			})

			It("returns an error", func() {
				routerGroup, err := client.RouterGroupWithName("pineapple")
				Expect(err).To(HaveOccurred())
				Expect(routerGroup).To(Equal(models.RouterGroup{}))
			})

			It("logs the request and response", func() {
				_, err := client.RouterGroupWithName("pineapple")
				Expect(err).To(HaveOccurred())
				expectedBody, _ := json.Marshal(routerGroup1)

				r, err := ioutil.ReadAll(stdout)
				log := string(r)
				Expect(err).NotTo(HaveOccurred())
				Expect(log).To(ContainSubstring("REQUEST: "))
				Expect(log).To(ContainSubstring("GET " + ROUTER_GROUPS_API_URL + "?name=pineapple HTTP/1.1"))

				Expect(log).To(ContainSubstring("RESPONSE: "))
				Expect(log).To(ContainSubstring("HTTP/1.1 500 Internal Server Error"))
				Expect(log).NotTo(ContainSubstring(string(expectedBody)))
			})
		})
	})

	Context("UpdateRouterGroup", func() {
		var (
			err          error
			routerGroup1 models.RouterGroup
		)

		BeforeEach(func() {
			routerGroup1 = models.RouterGroup{
				Guid:            DefaultRouterGroupGuid,
				Name:            DefaultRouterGroupName,
				Type:            DefaultRouterGroupType,
				ReservablePorts: "4000-5000",
			}
		})

		Context("when the server returns a valid response", func() {
			BeforeEach(func() {
				data, _ := json.Marshal([]models.RouterGroup{routerGroup1})

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("PUT", fmt.Sprintf("%s/%s", ROUTER_GROUPS_API_URL, routerGroup1.Guid)),
						ghttp.RespondWith(http.StatusOK, data),
					),
				)
			})

			It("Sends a UpdateRouterGroup request to the server", func() {
				err = client.UpdateRouterGroup(routerGroup1)
				Expect(server.ReceivedRequests()).Should(HaveLen(1))
			})
		})

		Context("When the server returns an error", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("PUT", fmt.Sprintf("%s/%s", ROUTER_GROUPS_API_URL, routerGroup1.Guid)),
						ghttp.RespondWith(http.StatusInternalServerError, nil),
					),
				)
			})

			It("returns an error", func() {
				err = client.UpdateRouterGroup(routerGroup1)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("DeleteRouterGroup", func() {
		var (
			err          error
			routerGroup1 models.RouterGroup
		)

		BeforeEach(func() {
			routerGroup1 = models.RouterGroup{
				Guid:            DefaultRouterGroupGuid,
				Name:            DefaultRouterGroupName,
				Type:            DefaultRouterGroupType,
				ReservablePorts: "4000-5000",
			}
		})

		Context("when the server returns a valid response", func() {
			BeforeEach(func() {
				data, _ := json.Marshal([]models.RouterGroup{routerGroup1})

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("DELETE", fmt.Sprintf("%s/%s", ROUTER_GROUPS_API_URL, routerGroup1.Guid)),
						ghttp.RespondWith(http.StatusNoContent, data),
					),
				)
			})

			It("Sends a DeleteRouterGroup request to the server", func() {
				err = client.DeleteRouterGroup(routerGroup1)
				Expect(server.ReceivedRequests()).Should(HaveLen(1))
			})
		})

		Context("When the server returns an error", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("DELETE", fmt.Sprintf("%s/%s", ROUTER_GROUPS_API_URL, routerGroup1.Guid)),
						ghttp.RespondWith(http.StatusInternalServerError, nil),
					),
				)
			})

			It("returns an error", func() {
				err = client.DeleteRouterGroup(routerGroup1)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("CreateRouterGroup", func() {

		var (
			err          error
			routerGroup1 models.RouterGroup
			routerGroup2 models.RouterGroup
		)

		BeforeEach(func() {
			routerGroup1 = models.RouterGroup{
				Name:            DefaultRouterGroupName,
				Type:            DefaultRouterGroupType,
				ReservablePorts: "4000-5000",
			}
			routerGroup2 = models.RouterGroup{
				Name: DefaultRouterGroupName,
				Type: "http",
			}
		})

		Context("when the server returns a valid response", func() {
			BeforeEach(func() {
				data, _ := json.Marshal(routerGroup1)

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyBody(data),
						ghttp.VerifyRequest("POST", ROUTER_GROUPS_API_URL),
						ghttp.RespondWith(http.StatusCreated, data),
					),
				)
			})

			It("Sends a UpdateRouterGroup request to the server", func() {
				err = client.CreateRouterGroup(routerGroup1)
				Expect(server.ReceivedRequests()).Should(HaveLen(1))
			})
		})

		Context("When the server returns an error", func() {
			BeforeEach(func() {
				data, _ := json.Marshal(routerGroup2)
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyBody(data),
						ghttp.VerifyRequest("POST", ROUTER_GROUPS_API_URL),
						ghttp.RespondWith(http.StatusInternalServerError, nil),
					),
				)
			})

			It("returns an error", func() {
				err = client.CreateRouterGroup(routerGroup2)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("SubscribeToEvents", func() {
		var eventSource routing_api.EventSource
		var err error
		var event sse.Event

		BeforeEach(func() {
			data, _ := json.Marshal(route1)
			event = sse.Event{
				ID:   "1",
				Name: "Upsert",
				Data: data,
			}

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", EVENTS_SSE_URL),
					ghttp.VerifyHeader(http.Header{
						"Authorization": []string{"bearer"},
					}),
					func(w http.ResponseWriter, req *http.Request) {
						defer GinkgoRecover()
						writeErr := event.Write(w)
						Expect(writeErr).ToNot(HaveOccurred())
					},
				),
			)
		})

		JustBeforeEach(func() {
			eventSource, err = client.SubscribeToEvents()
		})

		It("Starts an SSE connection to the server", func() {
			Expect(server.ReceivedRequests()).Should(HaveLen(1))
		})

		It("Streams events from the server", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(eventSource).ToNot(BeNil())

			ev, err := eventSource.Next()
			Expect(err).NotTo(HaveOccurred())

			Expect(ev.Route).To(Equal(route1))
			Expect(ev.Action).To(Equal("Upsert"))
		})

		It("logs the request", func() {
			r, err := ioutil.ReadAll(stdout)
			log := string(r)
			Expect(err).NotTo(HaveOccurred())
			Expect(log).To(ContainSubstring("REQUEST: "))
			Expect(log).To(ContainSubstring("GET " + EVENTS_SSE_URL + " HTTP/1.1"))
		})

		Context("When the server responds with BadResponseError", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", EVENTS_SSE_URL),
						ghttp.RespondWith(http.StatusUnauthorized, nil),
					),
				)
			})

			JustBeforeEach(func() {
				eventSource, err = client.SubscribeToEvents()
			})

			It("propagates the error to the client", func() {
				Expect(err).To(HaveOccurred())
				Expect(eventSource).To(BeNil())
				Expect(err.Error()).To(ContainSubstring("unauthorized"))
			})
		})

		Context("When the server responds with an error", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", EVENTS_SSE_URL),
						ghttp.RespondWith(http.StatusBadRequest, nil),
					),
				)
			})

			JustBeforeEach(func() {
				eventSource, err = client.SubscribeToEvents()
			})

			It("propagates the error to the client", func() {
				Expect(err).To(HaveOccurred())
				Expect(eventSource).To(BeNil())
			})
		})
	})

	Context("SubscribeToEventsWithMaxRetries", func() {
		var attemptChan chan struct{}

		BeforeEach(func() {
			attemptChan = make(chan struct{}, 3)
			handler := ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", EVENTS_SSE_URL),
				ghttp.VerifyHeader(http.Header{
					"Authorization": []string{"bearer"},
				}),
				func(w http.ResponseWriter, req *http.Request) {
					attemptChan <- struct{}{}
					server.CloseClientConnections()
				},
			)
			server.AppendHandlers(handler, handler, handler)
		})

		It("returns error", func() {
			_, err := client.SubscribeToEventsWithMaxRetries(2)
			Expect(err).To(HaveOccurred())
			Expect(attemptChan).To(Receive())
			Expect(attemptChan).To(Receive())
			Expect(attemptChan).To(Receive())
		})
	})

	Context("SubscribeToTcpEvents", func() {
		var (
			tcpEventSource routing_api.TcpEventSource
			err            error
			event          sse.Event
			tcpRoute1      models.TcpRouteMapping
		)

		BeforeEach(func() {
			tcpRoute1 = models.NewTcpRouteMapping("rguid1", 52000, "1.1.1.1", 60000, 60)

			data, _ := json.Marshal(tcpRoute1)
			event = sse.Event{
				ID:   "1",
				Name: "Upsert",
				Data: data,
			}

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", TCP_EVENTS_SSE_URL),
					ghttp.VerifyHeader(http.Header{
						"Authorization": []string{"bearer"},
					}),
					func(w http.ResponseWriter, req *http.Request) {
						defer GinkgoRecover()
						writeErr := event.Write(w)
						Expect(writeErr).ToNot(HaveOccurred())
					},
				),
			)
		})

		JustBeforeEach(func() {
			tcpEventSource, err = client.SubscribeToTcpEvents()
		})

		It("Starts an SSE connection to the server", func() {
			Expect(server.ReceivedRequests()).Should(HaveLen(1))
		})

		It("Streams events from the server", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(tcpEventSource).ToNot(BeNil())

			ev, err := tcpEventSource.Next()
			Expect(err).NotTo(HaveOccurred())

			Expect(ev.TcpRouteMapping).To(Equal(tcpRoute1))
			Expect(ev.Action).To(Equal("Upsert"))
		})

		It("logs the request", func() {
			r, err := ioutil.ReadAll(stdout)
			log := string(r)
			Expect(err).NotTo(HaveOccurred())
			Expect(log).To(ContainSubstring("REQUEST: "))
			Expect(log).To(ContainSubstring("GET " + TCP_EVENTS_SSE_URL + " HTTP/1.1"))
		})

		Context("When the server responds with an error", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", TCP_EVENTS_SSE_URL),
						ghttp.RespondWith(http.StatusBadRequest, nil),
					),
				)
			})

			JustBeforeEach(func() {
				tcpEventSource, err = client.SubscribeToTcpEvents()
			})

			It("propagates the error to the client", func() {
				Expect(err).To(HaveOccurred())
				Expect(tcpEventSource).To(BeNil())
			})
		})
	})

	Context("SubscribeToTcpEventsWithMaxRetries", func() {
		var attemptChan chan struct{}

		BeforeEach(func() {
			attemptChan = make(chan struct{}, 3)
			handler := ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", TCP_EVENTS_SSE_URL),
				ghttp.VerifyHeader(http.Header{
					"Authorization": []string{"bearer"},
				}),
				func(w http.ResponseWriter, req *http.Request) {
					attemptChan <- struct{}{}
					server.CloseClientConnections()
				},
			)
			server.AppendHandlers(handler, handler, handler)
		})

		It("returns error", func() {
			_, err := client.SubscribeToTcpEventsWithMaxRetries(2)
			Expect(err).To(HaveOccurred())
			Expect(attemptChan).To(Receive())
			Expect(attemptChan).To(Receive())
			Expect(attemptChan).To(Receive())
		})
	})

	Context("ReservePort", func() {
		When("no router groups already exist", func() {
			BeforeEach(func() {
				routerGroup := models.RouterGroup{
					Name:            "new-router-group",
					Type:            "tcp",
					ReservablePorts: "2000",
				}

				listData, _ := json.Marshal([]models.RouterGroup{})
				postData, _ := json.Marshal(routerGroup)

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL),
						ghttp.RespondWith(http.StatusOK, listData),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL, "name=new-router-group"),
						ghttp.RespondWith(http.StatusNotFound, []byte(`error: router group not found`)),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyBody(postData),
						ghttp.VerifyRequest("POST", ROUTER_GROUPS_API_URL),
						ghttp.RespondWith(http.StatusCreated, postData),
					),
				)
			})

			It("creates a new router group within the port range", func() {
				port, err := client.ReservePort("new-router-group", "2000-3000")
				Expect(err).ToNot(HaveOccurred())

				Expect(port).To(Equal(2000))
			})
		})

		When("another router group already exists", func() {
			BeforeEach(func() {
				existingRouterGroup := models.RouterGroup{
					Name:            "existing-router-group",
					Type:            "tcp",
					ReservablePorts: "2000",
				}

				newRouterGroup := models.RouterGroup{
					Name:            "new-router-group",
					Type:            "tcp",
					ReservablePorts: "2001",
				}

				listData, _ := json.Marshal([]models.RouterGroup{existingRouterGroup})
				postData, _ := json.Marshal(newRouterGroup)

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL),
						ghttp.RespondWith(http.StatusOK, listData),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL, "name=new-router-group"),
						ghttp.RespondWith(http.StatusNotFound, []byte(`error: router group not found`)),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyBody(postData),
						ghttp.VerifyRequest("POST", ROUTER_GROUPS_API_URL),
						ghttp.RespondWith(http.StatusCreated, postData),
					),
				)
			})

			It("creates a new router group with the next available port in the range", func() {
				port, err := client.ReservePort("new-router-group", "2000-3000")
				Expect(err).ToNot(HaveOccurred())

				Expect(port).To(Equal(2001))
			})
		})

		When("the router group already exists with a port outside of the requested port range", func() {
			BeforeEach(func() {
				existingRouterGroup := models.RouterGroup{
					Name:            "existing-router-group",
					Type:            "tcp",
					ReservablePorts: "2002",
					Guid:            "some-guid",
				}

				newRouterGroup := models.RouterGroup{
					Name:            "existing-router-group",
					Type:            "tcp",
					ReservablePorts: "3000",
					Guid:            "some-guid",
				}

				listData, _ := json.Marshal([]models.RouterGroup{existingRouterGroup})
				putData, _ := json.Marshal(newRouterGroup)

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL),
						ghttp.RespondWith(http.StatusOK, listData),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL, "name=existing-router-group"),
						ghttp.RespondWith(http.StatusOK, listData),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyBody(putData),
						ghttp.VerifyRequest("PUT", fmt.Sprintf("%s/%s", ROUTER_GROUPS_API_URL, existingRouterGroup.Guid)),

						ghttp.RespondWith(http.StatusCreated, putData),
					),
				)
			})

			It("updates the new router group with the next available port in the range", func() {
				port, err := client.ReservePort("existing-router-group", "3000-4000")
				Expect(err).ToNot(HaveOccurred())

				Expect(port).To(Equal(3000))
			})

		})

		When("updating the existing router group fails", func() {
			BeforeEach(func() {
				existingRouterGroup := models.RouterGroup{
					Name:            "existing-router-group",
					Type:            "tcp",
					ReservablePorts: "2002",
					Guid:            "some-guid",
				}

				newRouterGroup := models.RouterGroup{
					Name:            "existing-router-group",
					Type:            "tcp",
					ReservablePorts: "3000",
					Guid:            "some-guid",
				}

				listData, _ := json.Marshal([]models.RouterGroup{existingRouterGroup})
				putData, _ := json.Marshal(newRouterGroup)

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL),
						ghttp.RespondWith(http.StatusOK, listData),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL, "name=existing-router-group"),
						ghttp.RespondWith(http.StatusOK, listData),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyBody(putData),
						ghttp.VerifyRequest("PUT", fmt.Sprintf("%s/%s", ROUTER_GROUPS_API_URL, existingRouterGroup.Guid)),

						ghttp.RespondWith(http.StatusInternalServerError, []byte(`error: failed to update router group`)),
					),
				)
			})

			It("returns an error", func() {
				_, err := client.ReservePort("existing-router-group", "3000-4000")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to update router group"))
			})
		})

		When("another router group with multiple ports already exists", func() {
			BeforeEach(func() {
				existingRouterGroup := models.RouterGroup{
					Name:            "existing-router-group",
					Type:            "tcp",
					ReservablePorts: "2000-2010",
				}

				newRouterGroup := models.RouterGroup{
					Name:            "new-router-group",
					Type:            "tcp",
					ReservablePorts: "2011",
				}

				listData, _ := json.Marshal([]models.RouterGroup{existingRouterGroup})
				postData, _ := json.Marshal(newRouterGroup)

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL),
						ghttp.RespondWith(http.StatusOK, listData),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL, "name=new-router-group"),
						ghttp.RespondWith(http.StatusNotFound, []byte(`error: router group not found`)),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyBody(postData),
						ghttp.VerifyRequest("POST", ROUTER_GROUPS_API_URL),
						ghttp.RespondWith(http.StatusCreated, postData),
					),
				)
			})

			It("creates a new router group with the next available port in the range", func() {
				port, err := client.ReservePort("new-router-group", "2000-3000")
				Expect(err).ToNot(HaveOccurred())

				Expect(port).To(Equal(2011))
			})
		})

		When("another router group with multiple ports and ranges already exists", func() {
			BeforeEach(func() {
				existingRouterGroup := models.RouterGroup{
					Name:            "some-other-router-group",
					Type:            "tcp",
					ReservablePorts: "2000-2010,2010-2011,2050-2051",
				}

				newRouterGroup := models.RouterGroup{
					Name:            "new-router-group",
					Type:            "tcp",
					ReservablePorts: "2012",
				}

				listData, _ := json.Marshal([]models.RouterGroup{existingRouterGroup})
				postData, _ := json.Marshal(newRouterGroup)

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL),
						ghttp.RespondWith(http.StatusOK, listData),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL, "name=new-router-group"),
						ghttp.RespondWith(http.StatusNotFound, []byte(`error: router group not found`)),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyBody(postData),
						ghttp.VerifyRequest("POST", ROUTER_GROUPS_API_URL),
						ghttp.RespondWith(http.StatusCreated, postData),
					),
				)
			})

			It("creates a new router group with the next available port in the range", func() {
				port, err := client.ReservePort("new-router-group", "2000-3000")
				Expect(err).ToNot(HaveOccurred())

				Expect(port).To(Equal(2012))
			})
		})

		When("all router groups are taken", func() {
			BeforeEach(func() {
				existingRouterGroup := models.RouterGroup{
					Name:            "existing-router-group",
					Type:            "tcp",
					ReservablePorts: "2000-3000",
				}

				listData, _ := json.Marshal([]models.RouterGroup{existingRouterGroup})

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL),
						ghttp.RespondWith(http.StatusOK, listData),
					),
				)
			})

			It("returns a port range exhausted error", func() {
				_, err := client.ReservePort("new-router-group", "2000-3000")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("There are no free ports in range: [2000-3000]"))
			})
		})

		When("fetching router groups fails", func() {
			BeforeEach(func() {
				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL),
						ghttp.RespondWith(http.StatusInternalServerError, nil),
					),
				)
			})

			It("returns an error", func() {
				_, err := client.ReservePort("my-router-group", "2000")
				Expect(err).To(HaveOccurred())
			})
		})

		When("creating the router group fails", func() {
			BeforeEach(func() {
				newRouterGroup := models.RouterGroup{
					Name:            "new-router-group",
					Type:            "tcp",
					ReservablePorts: "2000",
				}

				listData, _ := json.Marshal([]models.RouterGroup{})
				postData, _ := json.Marshal(newRouterGroup)

				server.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL),
						ghttp.RespondWith(http.StatusOK, listData),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", ROUTER_GROUPS_API_URL, "name=new-router-group"),
						ghttp.RespondWith(http.StatusNotFound, []byte(`error: router group not found`)),
					),
					ghttp.CombineHandlers(
						ghttp.VerifyBody(postData),
						ghttp.VerifyRequest("POST", ROUTER_GROUPS_API_URL),
						ghttp.RespondWith(http.StatusInternalServerError, nil),
					),
				)
			})

			It("returns an error", func() {
				_, err := client.ReservePort("new-router-group", "2000")
				Expect(err).To(HaveOccurred())
			})
		})

		When("the port range is an invalid range", func() {
			It("returns an error", func() {
				_, err := client.ReservePort("new-router-group", "2000-3000-4000")
				Expect(err).To(HaveOccurred())
			})
		})

		When("multiple port ranges are requested", func(){
			It("returns an error", func() {
				_, err := client.ReservePort("new-router-group", "2000-3000,4000-5000")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("multiple port ranges are not supported"))

			})
		})

		When("a single port is requested", func() {
			It("returns an error", func() {
				_, err := client.ReservePort("new-router-group", "2000")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("single port is not supported"))
			})

		})
	})
})
