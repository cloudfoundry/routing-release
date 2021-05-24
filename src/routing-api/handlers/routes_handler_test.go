package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"

	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/routing-release/routing-api"
	"code.cloudfoundry.org/routing-release/routing-api/db"
	fake_db "code.cloudfoundry.org/routing-release/routing-api/db/fakes"
	"code.cloudfoundry.org/routing-release/routing-api/handlers"
	fake_validator "code.cloudfoundry.org/routing-release/routing-api/handlers/fakes"
	"code.cloudfoundry.org/routing-release/routing-api/metrics"
	"code.cloudfoundry.org/routing-release/routing-api/models"
	fake_client "code.cloudfoundry.org/uaa-go-client/fakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("RoutesHandler", func() {
	var (
		routesHandler    *handlers.RoutesHandler
		request          *http.Request
		responseRecorder *httptest.ResponseRecorder
		database         *fake_db.FakeDB
		logger           *lagertest.TestLogger
		validator        *fake_validator.FakeRouteValidator
		fakeClient       *fake_client.FakeClient
		defaultTTL       int
	)

	BeforeEach(func() {
		database = &fake_db.FakeDB{}
		validator = &fake_validator.FakeRouteValidator{}
		fakeClient = &fake_client.FakeClient{}
		logger = lagertest.NewTestLogger("routing-api-test")
		defaultTTL = 50
		routesHandler = handlers.NewRoutesHandler(fakeClient, defaultTTL, validator, database, logger)
		responseRecorder = httptest.NewRecorder()
	})

	Describe(".List", func() {
		It("response with a 200 OK", func() {
			request = handlers.NewTestRequest("")

			routesHandler.List(responseRecorder, request)

			Expect(responseRecorder.Code).To(Equal(http.StatusOK))
		})

		It("checks for routing.routes.read scope", func() {
			request = handlers.NewTestRequest("")

			routesHandler.List(responseRecorder, request)
			_, permission := fakeClient.DecodeTokenArgsForCall(0)
			Expect(permission).To(ConsistOf(handlers.RoutingRoutesReadScope))
		})

		Context("when the UAA token is not valid", func() {
			var (
				currentCount int64
			)
			BeforeEach(func() {
				currentCount = metrics.GetTokenErrors()
				fakeClient.DecodeTokenReturns(errors.New("Not valid"))
			})

			It("returns an Unauthorized status code", func() {
				request = handlers.NewTestRequest("")
				routesHandler.List(responseRecorder, request)
				Expect(responseRecorder.Code).To(Equal(http.StatusUnauthorized))
				Expect(metrics.GetTokenErrors()).To(Equal(currentCount + 1))
			})
		})

		Context("when token does not have necessary permissions", func() {
			BeforeEach(func() {
				fakeClient.DecodeTokenReturns(errors.New("Token does not have 'routing.router_groups.read' scope"))
			})
			It("returns an UnauthorizedError", func() {
				type authorization struct {
					Name, Message string
				}
				request = handlers.NewTestRequest("")
				routesHandler.List(responseRecorder, request)
				Expect(responseRecorder.Code).To(Equal(http.StatusUnauthorized))
				// convert body from json and get message part alone
				dec := json.NewDecoder(strings.NewReader(responseRecorder.Body.String()))
				var responseBody authorization
				err := dec.Decode(&responseBody)
				Expect(err).ToNot(HaveOccurred())
				Expect(responseBody.Message).To(Equal("You are not authorized to perform the requested action"))
			})
		})

		Context("when the database is empty", func() {
			var (
				routes []models.Route
			)

			BeforeEach(func() {
				routes = []models.Route{}

				database.ReadRoutesReturns(routes, nil)
			})

			It("returns an empty set", func() {
				request = handlers.NewTestRequest("")

				routesHandler.List(responseRecorder, request)

				Expect(responseRecorder.Body.String()).To(MatchJSON("[]"))
			})
		})

		Context("when the database has one route", func() {
			var (
				routes []models.Route
			)

			BeforeEach(func() {
				route := models.NewRoute("post_here", 7000, "1.2.3.4", "log", "rsurl", 60)
				routes = []models.Route{route}

				database.ReadRoutesReturns(routes, nil)
			})

			It("returns a single route", func() {
				request = handlers.NewTestRequest("")

				routesHandler.List(responseRecorder, request)

				Expect(responseRecorder.Body.String()).To(MatchJSON(`[
							{
								"route": "post_here",
								"port": 7000,
								"ip": "1.2.3.4",
								"ttl": 60,
								"log_guid": "log",
								"route_service_url": "rsurl",
								"modification_tag": {
									"guid": "",
									"index": 0
							  }
							}
						]`))
			})
		})

		Context("when the database has many routes", func() {
			var (
				routes []models.Route
			)

			BeforeEach(func() {
				route1 := models.NewRoute("post_here", 7000, "1.2.3.4", "", "", 0)
				route2 := models.NewRoute("post_there", 2000, "1.2.3.5", "Something", "", 23)
				routes = []models.Route{route1, route2}

				database.ReadRoutesReturns(routes, nil)
			})

			It("returns many routes", func() {
				request = handlers.NewTestRequest("")

				routesHandler.List(responseRecorder, request)

				Expect(responseRecorder.Body.String()).To(MatchJSON(`[
							{
								"route": "post_here",
								"port": 7000,
								"ip": "1.2.3.4",
								"ttl": 0,
								"log_guid": "",
								"modification_tag": {
									"guid": "",
									"index": 0
							  }

							},
							{
								"route": "post_there",
								"port": 2000,
								"ip": "1.2.3.5",
								"ttl": 23,
								"log_guid": "Something",
								"modification_tag": {
									"guid": "",
									"index": 0
							  }

							}
						]`))
			})
		})

		Context("when the database errors out", func() {
			BeforeEach(func() {
				database.ReadRoutesReturns(nil, errors.New("some bad thing happened"))
			})

			It("returns a 500 Internal Server Error", func() {
				request = handlers.NewTestRequest("")

				routesHandler.List(responseRecorder, request)

				Expect(responseRecorder.Code).To(Equal(http.StatusServiceUnavailable))
			})
		})
	})

	Describe(".DeleteRoute", func() {
		var (
			routes []models.Route
		)

		BeforeEach(func() {
			route := models.NewRoute("post_here", 7000, "1.2.3.4", "", "", 60)
			routes = []models.Route{route}
		})

		It("checks for routing.routes.write scope", func() {
			request = handlers.NewTestRequest(routes)

			routesHandler.Delete(responseRecorder, request)

			_, permission := fakeClient.DecodeTokenArgsForCall(0)
			Expect(permission).To(ConsistOf(handlers.RoutingRoutesWriteScope))
		})

		Context("when all inputs are present and correct", func() {
			It("returns a status not found when deleting a route", func() {
				request = handlers.NewTestRequest(routes)

				routesHandler.Delete(responseRecorder, request)

				Expect(responseRecorder.Code).To(Equal(http.StatusNoContent))
				Expect(database.DeleteRouteCallCount()).To(Equal(1))
				Expect(database.DeleteRouteArgsForCall(0)).To(Equal(routes[0]))
			})

			It("accepts an array of routes in the body", func() {
				routes = append(routes, routes[0])
				routes[1].IP = "5.4.3.2"

				request = handlers.NewTestRequest(routes)
				routesHandler.Delete(responseRecorder, request)

				Expect(responseRecorder.Code).To(Equal(http.StatusNoContent))
				Expect(database.DeleteRouteCallCount()).To(Equal(2))
				Expect(database.DeleteRouteArgsForCall(0)).To(Equal(routes[0]))
				Expect(database.DeleteRouteArgsForCall(1)).To(Equal(routes[1]))
			})

			It("logs the routes deletion", func() {
				request = handlers.NewTestRequest(routes)
				routesHandler.Delete(responseRecorder, request)

				data := map[string]interface{}{
					"ip":       "1.2.3.4",
					"log_guid": "",
					"port":     float64(7000),
					"route":    "post_here",
					"ttl":      float64(60),
					"modification_tag": map[string]interface{}{
						"guid":  "",
						"index": float64(0),
					},
				}
				log_data := map[string][]interface{}{"route_deletion": []interface{}{data}}

				Expect(logger.Logs()[0].Message).To(ContainSubstring("request"))
				Expect(logger.Logs()[0].Data["route_deletion"]).To(Equal(log_data["route_deletion"]))
			})

			Context("when the database deletion fails", func() {
				It("returns a 204 if the key was not found", func() {
					database.DeleteRouteReturns(db.DBError{Type: db.KeyNotFound, Message: "The specified route could not be found."})

					request = handlers.NewTestRequest(routes)
					routesHandler.Delete(responseRecorder, request)

					Expect(responseRecorder.Code).To(Equal(http.StatusNoContent))
				})

				It("responds with a server error", func() {
					database.DeleteRouteReturns(errors.New("stuff broke"))

					request = handlers.NewTestRequest(routes)
					routesHandler.Delete(responseRecorder, request)

					Expect(responseRecorder.Code).To(Equal(http.StatusServiceUnavailable))
					Expect(responseRecorder.Body.String()).To(ContainSubstring("stuff broke"))
				})
			})
		})

		Context("when there are errors with the input", func() {
			It("returns a bad request if it cannot parse the arguments", func() {
				request = handlers.NewTestRequest("bad args")

				routesHandler.Delete(responseRecorder, request)

				Expect(responseRecorder.Code).To(Equal(http.StatusBadRequest))
				Expect(responseRecorder.Body.String()).To(ContainSubstring("Cannot process request"))
			})
		})

		Context("when the UAA token is not valid", func() {
			var (
				currentCount int64
			)
			BeforeEach(func() {
				currentCount = metrics.GetTokenErrors()
				fakeClient.DecodeTokenReturns(errors.New("Not valid"))
			})

			It("returns an Unauthorized status code", func() {
				request = handlers.NewTestRequest(routes)
				routesHandler.Delete(responseRecorder, request)

				Expect(responseRecorder.Code).To(Equal(http.StatusUnauthorized))
				Expect(metrics.GetTokenErrors()).To(Equal(currentCount + 1))
			})
		})
	})

	Describe(".Upsert", func() {
		Context("POST", func() {
			var (
				route  models.Route
				routes []models.Route
			)

			BeforeEach(func() {
				route = models.NewRoute("post_here", 7000, "1.2.3.4", "logGuid", "rs.com", 40)
				routes = []models.Route{route}
			})

			It("checks for routing.routes.write scope", func() {
				request = handlers.NewTestRequest(routes)

				routesHandler.Upsert(responseRecorder, request)

				_, permission := fakeClient.DecodeTokenArgsForCall(0)
				Expect(permission).To(ConsistOf(handlers.RoutingRoutesWriteScope))
			})

			Context("when TTL is not set", func() {
				BeforeEach(func() {
					route.TTL = nil
				})

				It("sets the default TTL", func() {
					request = handlers.NewTestRequest([]models.Route{route})
					routesHandler.Upsert(responseRecorder, request)
					Expect(responseRecorder.Code).To(Equal(http.StatusCreated))
					Expect(database.SaveRouteCallCount()).To(Equal(1))
					Expect(*database.SaveRouteArgsForCall(0).TTL).To(Equal(defaultTTL))
				})
			})

			Context("when all inputs are present and correct", func() {
				It("returns an http status created", func() {
					request = handlers.NewTestRequest(routes)
					routesHandler.Upsert(responseRecorder, request)

					Expect(responseRecorder.Code).To(Equal(http.StatusCreated))
				})

				It("accepts a list of routes in the body", func() {
					route.IP = "5.4.3.2"
					routes = append(routes, route)

					request = handlers.NewTestRequest(routes)
					routesHandler.Upsert(responseRecorder, request)

					Expect(responseRecorder.Code).To(Equal(http.StatusCreated))
					Expect(database.SaveRouteCallCount()).To(Equal(2))
					Expect(database.SaveRouteArgsForCall(0)).To(Equal(routes[0]))
					Expect(database.SaveRouteArgsForCall(1)).To(Equal(routes[1]))
				})

				It("logs the route declaration", func() {
					request = handlers.NewTestRequest(routes)
					routesHandler.Upsert(responseRecorder, request)

					data := map[string]interface{}{
						"ip":                "1.2.3.4",
						"log_guid":          "logGuid",
						"port":              float64(7000),
						"route":             "post_here",
						"ttl":               float64(40),
						"route_service_url": "rs.com",
						"modification_tag": map[string]interface{}{
							"guid":  "",
							"index": float64(0),
						},
					}
					log_data := map[string][]interface{}{"route_creation": []interface{}{data}}

					Expect(logger.Logs()[0].Message).To(ContainSubstring("request"))
					Expect(logger.Logs()[0].Data["route_creation"]).To(Equal(log_data["route_creation"]))
				})

				It("does not require route_service_url on the request", func() {
					route.RouteServiceUrl = "rs.com"
					request = handlers.NewTestRequest([]models.Route{route})
					routesHandler.Upsert(responseRecorder, request)

					Expect(responseRecorder.Code).To(Equal(http.StatusCreated))
				})

				It("does not require log guid on the request", func() {
					route.LogGuid = ""

					request = handlers.NewTestRequest([]models.Route{route})
					routesHandler.Upsert(responseRecorder, request)

					Expect(responseRecorder.Code).To(Equal(http.StatusCreated))
				})

				Context("when database fails to save", func() {
					BeforeEach(func() {
						database.SaveRouteReturns(errors.New("stuff broke"))
					})

					It("responds with a server error", func() {
						request = handlers.NewTestRequest([]models.Route{route})
						routesHandler.Upsert(responseRecorder, request)

						Expect(responseRecorder.Code).To(Equal(http.StatusServiceUnavailable))
						Expect(responseRecorder.Body.String()).To(ContainSubstring("stuff broke"))
					})
				})
			})

			Context("when there are errors with the input", func() {
				BeforeEach(func() {
					validator.ValidateCreateReturns(&routing_api.Error{Type: "a type", Message: "error message"})
				})

				It("blows up when a port does not fit into a uint16", func() {
					json := `[{"route":"my-route.com","ip":"1.2.3.4", "port":65537}]`
					request = handlers.NewTestRequest(json)
					routesHandler.Upsert(responseRecorder, request)

					Expect(responseRecorder.Code).To(Equal(http.StatusBadRequest))
					Expect(responseRecorder.Body.String()).To(ContainSubstring("cannot unmarshal number 65537"))
				})

				It("does not write to the key-value store backend", func() {
					request = handlers.NewTestRequest([]models.Route{route})
					routesHandler.Upsert(responseRecorder, request)

					Expect(database.SaveRouteCallCount()).To(Equal(0))
				})

				It("logs the error", func() {
					request = handlers.NewTestRequest([]models.Route{route})
					routesHandler.Upsert(responseRecorder, request)

					Expect(logger.Logs()[1].Message).To(ContainSubstring("error"))
					Expect(logger.Logs()[1].Data["error"]).To(Equal("error message"))
				})
			})

			Context("when the UAA token is not valid", func() {
				var (
					currentCount int64
				)
				BeforeEach(func() {
					currentCount = metrics.GetTokenErrors()
					fakeClient.DecodeTokenReturns(errors.New("Not valid"))
				})

				It("returns an Unauthorized status code", func() {
					request = handlers.NewTestRequest([]models.Route{route})
					routesHandler.Upsert(responseRecorder, request)

					Expect(responseRecorder.Code).To(Equal(http.StatusUnauthorized))
					Expect(metrics.GetTokenErrors()).To(Equal(currentCount + 1))
				})
			})
		})
	})
})
