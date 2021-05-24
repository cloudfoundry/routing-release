package handlers_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"

	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/routing-release/routing-api"
	"code.cloudfoundry.org/routing-release/routing-api/db"
	fake_db "code.cloudfoundry.org/routing-release/routing-api/db/fakes"
	"code.cloudfoundry.org/routing-release/routing-api/handlers"
	"code.cloudfoundry.org/routing-release/routing-api/metrics"
	"code.cloudfoundry.org/routing-release/routing-api/models"
	fake_client "code.cloudfoundry.org/uaa-go-client/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/rata"
)

const (
	DefaultRouterGroupGuid      = "bad25cff-9332-48a6-8603-b619858e7992"
	DefaultHTTPRouterGroupGuid  = "sad25cff-9332-48a6-8603-b619858e7992"
	DefaultOtherRouterGroupGuid = "mad25cff-9332-48a6-8603-b619858e7992"
	DefaultRouterGroupName      = "default-tcp"
	DefaultRouterGroupType      = "tcp"
)

var _ = Describe("RouterGroupsHandler", func() {

	var (
		routerGroupHandler *handlers.RouterGroupsHandler
		request            *http.Request
		responseRecorder   *httptest.ResponseRecorder
		fakeClient         *fake_client.FakeClient
		fakeDb             *fake_db.FakeDB
		logger             *lagertest.TestLogger
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test-router-group")
		fakeClient = &fake_client.FakeClient{}
		fakeDb = &fake_db.FakeDB{}
		routerGroupHandler = handlers.NewRouteGroupsHandler(fakeClient, logger, fakeDb)
		responseRecorder = httptest.NewRecorder()

		fakeRouterGroups := []models.RouterGroup{
			{
				Guid:            DefaultRouterGroupGuid,
				Name:            DefaultRouterGroupName,
				Type:            DefaultRouterGroupType,
				ReservablePorts: "1024-65535",
			},
			{
				Guid: "some-guid",
				Name: "http-group",
				Type: "http",
			},
		}
		fakeDb.ReadRouterGroupsReturns(fakeRouterGroups, nil)
		fakeDb.ReadRouterGroupByNameStub = func(name string) (models.RouterGroup, error) {
			if name == "http-group" {
				return fakeRouterGroups[1], nil
			}
			return models.RouterGroup{}, nil
		}

	})

	Describe("ListRouterGroups", func() {
		It("responds with 200 OK and returns details for all the router groups", func() {
			var err error
			request, err = http.NewRequest("GET", routing_api.ListRouterGroups, nil)
			Expect(err).NotTo(HaveOccurred())
			routerGroupHandler.ListRouterGroups(responseRecorder, request)
			Expect(responseRecorder.Code).To(Equal(http.StatusOK))
			payload := responseRecorder.Body.String()
			Expect(payload).To(MatchJSON(`[
			{
				"guid": "bad25cff-9332-48a6-8603-b619858e7992",
				"name": "default-tcp",
				"type": "tcp",
				"reservable_ports": "1024-65535"
			},
			{
				"guid": "some-guid",
				"name": "http-group",
				"type": "http",
				"reservable_ports": ""
			}]`))
		})

		Context("when router name is passed in as a query param", func() {

			It("should return router group for that name", func() {
				request = handlers.NewTestRequest("")
				requestURL, err := url.Parse("http://foobar.com/routing/v1/router_groups?name=http-group")
				request.URL = requestURL
				Expect(err).NotTo(HaveOccurred())

				routerGroupHandler.ListRouterGroups(responseRecorder, request)
				Expect(responseRecorder.Code).To(Equal(http.StatusOK))
				payload := responseRecorder.Body.String()
				Expect(payload).To(MatchJSON(`[
				{
					"guid": "some-guid",
					"name": "http-group",
					"type": "http",
					"reservable_ports": ""
				}]`))
			})

			It("should return 404 if that router group does not exist", func() {
				request = handlers.NewTestRequest("")
				requestURL, err := url.Parse("http://foobar.com/routing/v1/router_groups?name=does-not-exist")
				request.URL = requestURL
				Expect(err).NotTo(HaveOccurred())

				routerGroupHandler.ListRouterGroups(responseRecorder, request)
				Expect(responseRecorder.Code).To(Equal(http.StatusNotFound))

				payload := responseRecorder.Body.String()
				Expect(payload).To(MatchJSON(`{
				  "name": "ResourceNotFoundError",
					"message": "Router Group 'does-not-exist' not found"
				}`))
			})
		})

		It("checks for routing.router_groups.read scope", func() {
			var err error
			request, err = http.NewRequest("GET", routing_api.ListRouterGroups, nil)
			Expect(err).NotTo(HaveOccurred())
			routerGroupHandler.ListRouterGroups(responseRecorder, request)
			_, permission := fakeClient.DecodeTokenArgsForCall(0)
			Expect(permission).To(ConsistOf(handlers.RouterGroupsReadScope))
		})

		Context("when the db fails to save router group", func() {
			BeforeEach(func() {
				fakeDb.ReadRouterGroupsReturns([]models.RouterGroup{}, errors.New("db communication failed"))
			})

			It("returns a DB communication error", func() {
				var err error
				request, err = http.NewRequest("GET", routing_api.ListRouterGroups, nil)
				Expect(err).NotTo(HaveOccurred())
				routerGroupHandler.ListRouterGroups(responseRecorder, request)
				Expect(fakeDb.ReadRouterGroupsCallCount()).To(Equal(1))
				Expect(responseRecorder.Code).To(Equal(http.StatusServiceUnavailable))
				payload := responseRecorder.Body.String()
				Expect(payload).To(MatchJSON(`{
					"name": "DBCommunicationError",
					"message": "db communication failed"
				}`))
			})
		})

		Context("when authorization token is invalid", func() {
			var (
				currentCount int64
			)
			BeforeEach(func() {
				currentCount = metrics.GetTokenErrors()
				fakeClient.DecodeTokenReturns(errors.New("kaboom"))
			})

			It("returns Unauthorized error", func() {
				var err error
				request, err = http.NewRequest("GET", routing_api.ListRouterGroups, nil)
				Expect(err).NotTo(HaveOccurred())
				routerGroupHandler.ListRouterGroups(responseRecorder, request)
				Expect(responseRecorder.Code).To(Equal(http.StatusUnauthorized))
				Expect(metrics.GetTokenErrors()).To(Equal(currentCount + 1))
			})
		})

	})

	Describe("UpdateRouterGroup", func() {
		var (
			existingTCPRouterGroup   models.RouterGroup
			existingHTTPRouterGroup  models.RouterGroup
			existingOtherRouterGroup models.RouterGroup
			handler                  http.Handler
			body                     io.Reader
		)

		BeforeEach(func() {
			var err error
			existingTCPRouterGroup = models.RouterGroup{
				Guid:            DefaultRouterGroupGuid,
				Name:            DefaultRouterGroupName,
				Type:            DefaultRouterGroupType,
				ReservablePorts: "1024-65535",
			}

			existingHTTPRouterGroup = models.RouterGroup{
				Guid:            DefaultHTTPRouterGroupGuid,
				Name:            "default-http",
				Type:            "http",
				ReservablePorts: "",
			}

			existingOtherRouterGroup = models.RouterGroup{
				Guid:            DefaultOtherRouterGroupGuid,
				Name:            "default-other",
				Type:            "other",
				ReservablePorts: "9876",
			}

			fakeDb.ReadRouterGroupStub = func(guid string) (models.RouterGroup, error) {
				if guid == DefaultHTTPRouterGroupGuid {
					return existingHTTPRouterGroup, nil
				}
				if guid == DefaultOtherRouterGroupGuid {
					return existingOtherRouterGroup, nil
				}
				return existingTCPRouterGroup, nil
			}

			routes := rata.Routes{
				routing_api.RoutesMap[routing_api.UpdateRouterGroup],
			}
			handler, err = rata.NewRouter(routes, rata.Handlers{
				routing_api.UpdateRouterGroup: http.HandlerFunc(routerGroupHandler.UpdateRouterGroup),
			})
			Expect(err).NotTo(HaveOccurred())
			queryGroup := models.RouterGroup{
				ReservablePorts: "8000",
			}
			bodyBytes, err := json.Marshal(queryGroup)
			Expect(err).ToNot(HaveOccurred())
			body = bytes.NewReader(bodyBytes)
		})

		It("saves the router group", func() {
			var err error
			request, err = http.NewRequest(
				"PUT",
				fmt.Sprintf("/routing/v1/router_groups/%s", DefaultRouterGroupGuid),
				body,
			)
			Expect(err).NotTo(HaveOccurred())

			handler.ServeHTTP(responseRecorder, request)

			Expect(fakeDb.ReadRouterGroupCallCount()).To(Equal(1))
			guid := fakeDb.ReadRouterGroupArgsForCall(0)
			Expect(guid).To(Equal(DefaultRouterGroupGuid))

			Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(1))
			savedGroup := fakeDb.SaveRouterGroupArgsForCall(0)
			updatedGroup := models.RouterGroup{
				Guid:            DefaultRouterGroupGuid,
				Name:            DefaultRouterGroupName,
				Type:            DefaultRouterGroupType,
				ReservablePorts: "8000",
			}
			Expect(savedGroup).To(Equal(updatedGroup))

			Expect(responseRecorder.Code).To(Equal(http.StatusOK))
			payload := responseRecorder.Body.String()
			Expect(payload).To(MatchJSON(`
			{
			"guid": "bad25cff-9332-48a6-8603-b619858e7992",
			"name": "default-tcp",
			"type": "tcp",
			"reservable_ports": "8000"
			}`))
		})

		It("adds X-Cf-Warnings header", func() {
			var err error
			request, err = http.NewRequest(
				"PUT",
				fmt.Sprintf("/routing/v1/router_groups/%s", DefaultRouterGroupGuid),
				body,
			)
			Expect(err).NotTo(HaveOccurred())

			handler.ServeHTTP(responseRecorder, request)
			warning := responseRecorder.HeaderMap.Get("X-Cf-Warnings")
			Expect(url.QueryUnescape(warning)).To(ContainSubstring("routes becoming inaccessible"))
		})

		Context("when reservable port field is invalid", func() {
			BeforeEach(func() {
				queryGroup := models.RouterGroup{
					ReservablePorts: "fadfadfasdf",
				}
				bodyBytes, err := json.Marshal(queryGroup)
				Expect(err).ToNot(HaveOccurred())
				body = bytes.NewReader(bodyBytes)
			})

			It("does not save the router group", func() {
				var err error
				request, err = http.NewRequest(
					"PUT",
					fmt.Sprintf("/routing/v1/router_groups/%s", DefaultRouterGroupGuid),
					body,
				)
				Expect(err).NotTo(HaveOccurred())

				handler.ServeHTTP(responseRecorder, request)

				Expect(fakeDb.ReadRouterGroupCallCount()).To(Equal(1))
				guid := fakeDb.ReadRouterGroupArgsForCall(0)
				Expect(guid).To(Equal(DefaultRouterGroupGuid))

				Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(0))
			})

			It("returns a 400 Bad Request", func() {
				var err error
				request, err = http.NewRequest(
					"PUT",
					fmt.Sprintf("/routing/v1/router_groups/%s", DefaultRouterGroupGuid),
					body,
				)
				Expect(err).NotTo(HaveOccurred())

				handler.ServeHTTP(responseRecorder, request)

				Expect(responseRecorder.Code).To(Equal(http.StatusBadRequest))
				payload := responseRecorder.Body.String()
				Expect(payload).To(MatchJSON(`
				{
					"name": "ProcessRequestError",
					"message": "Cannot process request: Port must be between 1024 and 65535"
				}`))
			})
		})

		Context("when adding reservable ports to type: http", func() {
			BeforeEach(func() {
				queryGroup := models.RouterGroup{
					ReservablePorts: "8001",
				}

				bodyBytes, err := json.Marshal(queryGroup)
				Expect(err).ToNot(HaveOccurred())
				body = bytes.NewReader(bodyBytes)
			})

			It("does not save the router group", func() {
				var err error
				request, err = http.NewRequest(
					"PUT",
					fmt.Sprintf("/routing/v1/router_groups/%s", DefaultHTTPRouterGroupGuid),
					body,
				)
				Expect(err).NotTo(HaveOccurred())

				handler.ServeHTTP(responseRecorder, request)

				Expect(fakeDb.ReadRouterGroupCallCount()).To(Equal(1))
				guid := fakeDb.ReadRouterGroupArgsForCall(0)
				Expect(guid).To(Equal(DefaultHTTPRouterGroupGuid))

				Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(0))
			})

			It("returns a 400 Bad Request", func() {
				var err error
				request, err = http.NewRequest(
					"PUT",
					fmt.Sprintf("/routing/v1/router_groups/%s", DefaultHTTPRouterGroupGuid),
					body,
				)
				Expect(err).NotTo(HaveOccurred())

				handler.ServeHTTP(responseRecorder, request)

				Expect(responseRecorder.Code).To(Equal(http.StatusBadRequest))
				payload := responseRecorder.Body.String()
				Expect(payload).To(MatchJSON(`
				{
					"name": "ProcessRequestError",
					"message": "Cannot process request: Reservable ports are not supported for router groups of type http"
				}`))
			})
		})

		Context("when changing non-http, non-tcp router groups", func() {
			It("saves the router group when changing the ports", func() {
				queryGroup := models.RouterGroup{
					ReservablePorts: "8001",
				}

				bodyBytes, err := json.Marshal(queryGroup)
				Expect(err).ToNot(HaveOccurred())
				body = bytes.NewReader(bodyBytes)

				request, err = http.NewRequest(
					"PUT",
					fmt.Sprintf("/routing/v1/router_groups/%s", DefaultOtherRouterGroupGuid),
					body,
				)
				Expect(err).NotTo(HaveOccurred())

				handler.ServeHTTP(responseRecorder, request)

				Expect(fakeDb.ReadRouterGroupCallCount()).To(Equal(1))
				guid := fakeDb.ReadRouterGroupArgsForCall(0)
				Expect(guid).To(Equal(DefaultOtherRouterGroupGuid))

				Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(1))
				savedGroup := fakeDb.SaveRouterGroupArgsForCall(0)
				updatedGroup := models.RouterGroup{
					Guid:            DefaultOtherRouterGroupGuid,
					Name:            "default-other",
					Type:            "other",
					ReservablePorts: "8001",
				}
				Expect(savedGroup).To(Equal(updatedGroup))

				Expect(responseRecorder.Code).To(Equal(http.StatusOK))
				payload := responseRecorder.Body.String()
				Expect(payload).To(MatchJSON(`
			{
			"guid": "mad25cff-9332-48a6-8603-b619858e7992",
			"name": "default-other",
			"type": "other",
			"reservable_ports": "8001"
			}`))
			})

			It("saves the router group when removing the ports", func() {
				queryGroup := models.RouterGroup{
					ReservablePorts: "",
				}

				bodyBytes, err := json.Marshal(queryGroup)
				Expect(err).ToNot(HaveOccurred())
				body = bytes.NewReader(bodyBytes)

				request, err = http.NewRequest(
					"PUT",
					fmt.Sprintf("/routing/v1/router_groups/%s", DefaultOtherRouterGroupGuid),
					body,
				)
				Expect(err).NotTo(HaveOccurred())

				handler.ServeHTTP(responseRecorder, request)

				Expect(fakeDb.ReadRouterGroupCallCount()).To(Equal(1))
				guid := fakeDb.ReadRouterGroupArgsForCall(0)
				Expect(guid).To(Equal(DefaultOtherRouterGroupGuid))

				Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(1))
				savedGroup := fakeDb.SaveRouterGroupArgsForCall(0)
				updatedGroup := models.RouterGroup{
					Guid:            DefaultOtherRouterGroupGuid,
					Name:            "default-other",
					Type:            "other",
					ReservablePorts: "",
				}
				Expect(savedGroup).To(Equal(updatedGroup))

				Expect(responseRecorder.Code).To(Equal(http.StatusOK))
				payload := responseRecorder.Body.String()
				Expect(payload).To(MatchJSON(`
			{
			"guid": "mad25cff-9332-48a6-8603-b619858e7992",
			"name": "default-other",
			"type": "other",
			"reservable_ports": ""
			}`))
			})

		})

		Context("when reservable port field is the empty string for a TCP router group", func() {
			It("does not save the router group", func() {
				var err error

				queryGroup := models.RouterGroup{}
				bodyBytes, err := json.Marshal(queryGroup)
				Expect(err).ToNot(HaveOccurred())
				body := bytes.NewReader(bodyBytes)
				request, err = http.NewRequest(
					"PUT",
					fmt.Sprintf("/routing/v1/router_groups/%s", DefaultRouterGroupGuid),
					body,
				)
				Expect(err).NotTo(HaveOccurred())
				handler.ServeHTTP(responseRecorder, request)

				Expect(fakeDb.ReadRouterGroupCallCount()).To(Equal(1))
				guid := fakeDb.ReadRouterGroupArgsForCall(0)
				Expect(guid).To(Equal(DefaultRouterGroupGuid))

				Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(0))

				Expect(responseRecorder.Code).To(Equal(http.StatusBadRequest))
				payload := responseRecorder.Body.String()
				Expect(payload).To(MatchJSON(`
				{
					"name": "ProcessRequestError",
					"message": "Cannot process request: Missing reservable_ports in router group: default-tcp"
				}`))
			})
		})

		Context("when reservable port field is not changed", func() {
			It("does not save the router group", func() {
				var err error

				queryGroup := models.RouterGroup{
					ReservablePorts: "1024-65535",
				}
				bodyBytes, err := json.Marshal(queryGroup)
				Expect(err).ToNot(HaveOccurred())
				body := bytes.NewReader(bodyBytes)
				request, err = http.NewRequest(
					"PUT",
					fmt.Sprintf("/routing/v1/router_groups/%s", DefaultRouterGroupGuid),
					body,
				)
				Expect(err).NotTo(HaveOccurred())
				handler.ServeHTTP(responseRecorder, request)

				Expect(fakeDb.ReadRouterGroupCallCount()).To(Equal(1))
				guid := fakeDb.ReadRouterGroupArgsForCall(0)
				Expect(guid).To(Equal(DefaultRouterGroupGuid))

				Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(0))

				Expect(responseRecorder.Code).To(Equal(http.StatusOK))
				payload := responseRecorder.Body.String()
				Expect(payload).To(MatchJSON(`
				{
				"guid": "bad25cff-9332-48a6-8603-b619858e7992",
				"name": "default-tcp",
				"type": "tcp",
				"reservable_ports": "1024-65535"
				}`))
			})
		})

		It("checks for routing.router_groups.write scope", func() {
			var err error
			updatedGroup := models.RouterGroup{
				ReservablePorts: "8000",
			}
			bodyBytes, err := json.Marshal(updatedGroup)
			Expect(err).ToNot(HaveOccurred())
			body := bytes.NewReader(bodyBytes)
			request, err = http.NewRequest(
				"PUT",
				fmt.Sprintf("/routing/v1/router_groups/%s", DefaultRouterGroupGuid),
				body,
			)
			Expect(err).NotTo(HaveOccurred())
			handler.ServeHTTP(responseRecorder, request)
			_, permission := fakeClient.DecodeTokenArgsForCall(0)
			Expect(permission).To(ConsistOf(handlers.RouterGroupsWriteScope))
		})

		Context("when the router group does not exist", func() {
			BeforeEach(func() {
				fakeDb.ReadRouterGroupReturns(models.RouterGroup{}, nil)
			})

			It("does not save the router group and returns a not found status", func() {
				var err error

				bodyBytes := []byte("{}")
				body := bytes.NewReader(bodyBytes)
				request, err = http.NewRequest(
					"PUT",
					"/routing/v1/router_groups/not-exist",
					body,
				)
				Expect(err).NotTo(HaveOccurred())
				handler.ServeHTTP(responseRecorder, request)
				Expect(fakeDb.ReadRouterGroupCallCount()).To(Equal(1))
				Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(0))
				Expect(responseRecorder.Code).To(Equal(http.StatusNotFound))
				payload := responseRecorder.Body.String()
				Expect(payload).To(MatchJSON(`{
					"name": "ResourceNotFoundError",
					"message": "Router Group 'not-exist' does not exist"
				}`))
			})
		})

		Context("when the request body is invalid", func() {
			It("does not save the router group and returns a bad request response", func() {
				var err error

				bodyBytes := []byte("invalid json")
				body := bytes.NewReader(bodyBytes)
				request, err = http.NewRequest(
					"PUT",
					fmt.Sprintf("/routing/v1/router_groups/%s", DefaultRouterGroupGuid),
					body,
				)
				Expect(err).NotTo(HaveOccurred())
				handler.ServeHTTP(responseRecorder, request)
				Expect(fakeDb.ReadRouterGroupCallCount()).To(Equal(0))
				Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(0))
				Expect(responseRecorder.Code).To(Equal(http.StatusBadRequest))
			})
		})

		Context("when the db fails to read router group", func() {
			BeforeEach(func() {
				fakeDb.ReadRouterGroupReturns(models.RouterGroup{}, errors.New("db communication failed"))
			})

			It("returns a DB communication error", func() {
				var err error

				updatedGroup := models.RouterGroup{
					ReservablePorts: "8000",
				}
				bodyBytes, err := json.Marshal(updatedGroup)
				Expect(err).ToNot(HaveOccurred())
				body := bytes.NewReader(bodyBytes)
				request, err = http.NewRequest(
					"PUT",
					fmt.Sprintf("/routing/v1/router_groups/%s", DefaultRouterGroupGuid),
					body,
				)
				Expect(err).NotTo(HaveOccurred())
				handler.ServeHTTP(responseRecorder, request)
				Expect(fakeDb.ReadRouterGroupCallCount()).To(Equal(1))
				Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(0))
				Expect(responseRecorder.Code).To(Equal(http.StatusServiceUnavailable))
				payload := responseRecorder.Body.String()
				Expect(payload).To(MatchJSON(`{
					"name": "DBCommunicationError",
					"message": "db communication failed"
				}`))
			})
		})

		Context("when the db fails to save router group", func() {
			BeforeEach(func() {
				fakeDb.SaveRouterGroupReturns(errors.New("db communication failed"))
			})

			It("returns a DB communication error", func() {
				var err error

				updatedGroup := models.RouterGroup{
					ReservablePorts: "8000",
				}
				bodyBytes, err := json.Marshal(updatedGroup)
				Expect(err).ToNot(HaveOccurred())
				body := bytes.NewReader(bodyBytes)
				request, err = http.NewRequest(
					"PUT",
					fmt.Sprintf("/routing/v1/router_groups/%s", DefaultRouterGroupGuid),
					body,
				)
				Expect(err).NotTo(HaveOccurred())
				handler.ServeHTTP(responseRecorder, request)
				Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(1))
				Expect(responseRecorder.Code).To(Equal(http.StatusServiceUnavailable))
				payload := responseRecorder.Body.String()
				Expect(payload).To(MatchJSON(`{
					"name": "DBCommunicationError",
					"message": "db communication failed"
				}`))
			})
		})

		Context("when authorization token is invalid", func() {
			var (
				currentCount int64
			)
			BeforeEach(func() {
				currentCount = metrics.GetTokenErrors()
				fakeClient.DecodeTokenReturns(errors.New("kaboom"))
			})

			It("returns Unauthorized error", func() {
				var err error

				updatedGroup := models.RouterGroup{
					ReservablePorts: "8000",
				}
				bodyBytes, err := json.Marshal(updatedGroup)
				Expect(err).ToNot(HaveOccurred())
				body := bytes.NewReader(bodyBytes)
				request, err = http.NewRequest(
					"PUT",
					fmt.Sprintf("/routing/v1/router_groups/%s", DefaultRouterGroupGuid),
					body,
				)
				Expect(err).NotTo(HaveOccurred())
				handler.ServeHTTP(responseRecorder, request)
				Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(0))
				Expect(responseRecorder.Code).To(Equal(http.StatusUnauthorized))
				Expect(metrics.GetTokenErrors()).To(Equal(currentCount + 1))
			})
		})
	})

	Describe("DeleteRouterGroup", func() {
		var (
			handler http.Handler
		)

		BeforeEach(func() {
			var err error
			routes := rata.Routes{
				routing_api.RoutesMap[routing_api.DeleteRouterGroup],
			}
			handler, err = rata.NewRouter(routes, rata.Handlers{
				routing_api.DeleteRouterGroup: http.HandlerFunc(routerGroupHandler.DeleteRouterGroup),
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("deletes the router group", func() {
			var err error
			request, err = http.NewRequest(
				"DELETE",
				fmt.Sprintf("/routing/v1/router_groups/%s", DefaultRouterGroupGuid),
				nil,
			)
			Expect(err).NotTo(HaveOccurred())

			handler.ServeHTTP(responseRecorder, request)

			Expect(fakeDb.DeleteRouterGroupCallCount()).To(Equal(1))
			guid := fakeDb.DeleteRouterGroupArgsForCall(0)
			Expect(guid).To(Equal(DefaultRouterGroupGuid))

			Expect(responseRecorder.Code).To(Equal(http.StatusNoContent))
			payload := responseRecorder.Body.String()
			Expect(payload).To(BeEmpty())

			Expect(responseRecorder.Header().Get("Content-Length")).To(Equal("0"))
		})

		Context("when the router group does not exist", func() {
			BeforeEach(func() {
				fakeDb.DeleteRouterGroupReturns(db.DeleteRouterGroupError)
			})

			It("returns a not found status", func() {
				var err error

				request, err = http.NewRequest(
					"DELETE",
					"/routing/v1/router_groups/not-exist",
					nil,
				)
				Expect(err).NotTo(HaveOccurred())
				handler.ServeHTTP(responseRecorder, request)
				Expect(fakeDb.DeleteRouterGroupCallCount()).To(Equal(1))
				Expect(responseRecorder.Code).To(Equal(http.StatusNotFound))
				payload := responseRecorder.Body.String()
				Expect(payload).To(BeEmpty())
			})
		})

		Context("when the db fails to delete router group", func() {
			BeforeEach(func() {
				fakeDb.DeleteRouterGroupReturns(errors.New("db communication failed"))
			})

			It("returns a DB communication error", func() {
				var err error

				request, err = http.NewRequest(
					"DELETE",
					fmt.Sprintf("/routing/v1/router_groups/%s", DefaultRouterGroupGuid),
					nil,
				)
				Expect(err).NotTo(HaveOccurred())
				handler.ServeHTTP(responseRecorder, request)
				Expect(fakeDb.DeleteRouterGroupCallCount()).To(Equal(1))
				Expect(responseRecorder.Code).To(Equal(http.StatusServiceUnavailable))
				payload := responseRecorder.Body.String()
				Expect(payload).To(MatchJSON(`{
					"name": "DBCommunicationError",
					"message": "db communication failed"
				}`))
			})
		})

		Context("when authorization token is invalid", func() {
			var (
				currentCount int64
			)
			BeforeEach(func() {
				currentCount = metrics.GetTokenErrors()
				fakeClient.DecodeTokenReturns(errors.New("kaboom"))
			})

			It("returns Unauthorized error", func() {
				var err error

				request, err = http.NewRequest(
					"DELETE",
					fmt.Sprintf("/routing/v1/router_groups/%s", DefaultRouterGroupGuid),
					nil,
				)
				Expect(err).NotTo(HaveOccurred())
				handler.ServeHTTP(responseRecorder, request)
				Expect(fakeDb.DeleteRouterGroupCallCount()).To(Equal(0))
				Expect(responseRecorder.Code).To(Equal(http.StatusUnauthorized))
				Expect(metrics.GetTokenErrors()).To(Equal(currentCount + 1))
			})
		})
	})

	Describe("CreateRouterGroup", func() {
		Describe("HTTP Router groups", func() {
			Context("when the request body is invalid", func() {
				Context("when reservable_ports is set", func() {
					It("does not save the router group and returns a bad request response", func() {
						var err error

						bodyBytes := []byte(`{"name":"test-group","type":"http","reservable_ports":"1024"}`)
						body := bytes.NewReader(bodyBytes)
						request, err := http.NewRequest(
							"POST",
							routing_api.CreateRouterGroup,
							body,
						)
						Expect(err).NotTo(HaveOccurred())

						routerGroupHandler.CreateRouterGroup(responseRecorder, request)
						Expect(err).NotTo(HaveOccurred())
						Expect(responseRecorder.Code).To(Equal(http.StatusBadRequest))
						Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(0))
					})
				})
			})

			Context("when the request body is valid", func() {
				It("saves the router group", func() {
					var err error
					bodyBytes := []byte(`{"name":"test-group","type":"http"}`)
					body := bytes.NewReader(bodyBytes)
					request, err = http.NewRequest(
						"POST",
						routing_api.CreateRouterGroup,
						body,
					)
					Expect(err).NotTo(HaveOccurred())

					routerGroupHandler.CreateRouterGroup(responseRecorder, request)

					Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(1))
					savedGroup := fakeDb.SaveRouterGroupArgsForCall(0)
					Expect(err).NotTo(HaveOccurred())
					Expect(savedGroup.Guid).NotTo(BeEmpty())
					Expect(savedGroup.Name).To(Equal("test-group"))
					Expect(savedGroup.Type).To(Equal(models.RouterGroupType("http")))
					Expect(responseRecorder.Code).To(Equal(http.StatusCreated))
					payload := responseRecorder.Body.String()
					jsonPayload := fmt.Sprintf("\n{\n\"guid\": \"%s\",\n\"name\": \"test-group\",\n\"type\": \"http\",\n\"reservable_ports\":\"\"\n}", savedGroup.Guid)
					Expect(payload).To(MatchJSON(jsonPayload))
				})
				It("checks for routing.router_groups.write scope", func() {
					var err error
					bodyBytes := []byte(`{"name":"test-group","type":"http"}`)
					body := bytes.NewReader(bodyBytes)
					request, err = http.NewRequest(
						"POST",
						routing_api.CreateRouterGroup,
						body,
					)
					Expect(err).NotTo(HaveOccurred())
					routerGroupHandler.CreateRouterGroup(responseRecorder, request)
					_, permission := fakeClient.DecodeTokenArgsForCall(0)
					Expect(permission).To(ConsistOf(handlers.RouterGroupsWriteScope))
				})
				Context("when the scope is routing.router_groups.write", func() {
					It("returns 200 when re-creating router group with same attributes", func() {
						createRouterGroup := func() (error, *httptest.ResponseRecorder) {
							var err error
							bodyBytes := []byte(`{"name":"test-group","type":"http"}`)
							body := bytes.NewReader(bodyBytes)
							request, err = http.NewRequest(
								"POST",
								routing_api.CreateRouterGroup,
								body,
							)
							responseRecorder = httptest.NewRecorder()
							routerGroupHandler.CreateRouterGroup(responseRecorder, request)
							return err, responseRecorder
						}

						err, resp := createRouterGroup()
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.Code).To(Equal(http.StatusCreated))

						savedGroup := fakeDb.SaveRouterGroupArgsForCall(0)
						fakeRouterGroups := []models.RouterGroup{savedGroup}
						fakeDb.ReadRouterGroupsReturns(fakeRouterGroups, nil)

						err, resp = createRouterGroup()
						Expect(err).NotTo(HaveOccurred())
						Expect(resp.Code).To(Equal(http.StatusOK))
						payload := resp.Body.String()
						jsonPayload := fmt.Sprintf("\n{\n\"guid\": \"%s\",\n\"name\": \"test-group\",\n\"type\": \"http\",\n\"reservable_ports\":\"\"\n}", savedGroup.Guid)
						Expect(payload).To(MatchJSON(jsonPayload))
					})
				})
				Context("when the scope is NOT routing.router_groups.write", func() {
					It("returns unauthorized when creating router group with same attributes", func() {
						createRouterGroup := func() (error, int) {
							var err error
							bodyBytes := []byte(`{"name":"test-group","type":"http"}`)
							body := bytes.NewReader(bodyBytes)
							request, err = http.NewRequest(
								"POST",
								routing_api.CreateRouterGroup,
								body,
							)
							responseRecorder = httptest.NewRecorder()
							routerGroupHandler.CreateRouterGroup(responseRecorder, request)
							return err, responseRecorder.Code
						}

						fakeClient.DecodeTokenReturns(errors.New("non admin token"))

						err, statusCode := createRouterGroup()
						Expect(err).ToNot(HaveOccurred())
						Expect(statusCode).To(Equal(http.StatusUnauthorized))
					})
				})

				Context("when extra fields (i.e. guid) are provided", func() {
					It("doesn't do anything with the extra fields", func() {
						var err error
						bodyBytes := []byte(`{"guid":"some-other-guid","name":"test-group","type":"http","banana":"fake-banana"}`)
						body := bytes.NewReader(bodyBytes)
						request, err = http.NewRequest(
							"POST",
							routing_api.CreateRouterGroup,
							body,
						)
						Expect(err).NotTo(HaveOccurred())

						routerGroupHandler.CreateRouterGroup(responseRecorder, request)

						Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(1))
						savedGroup := fakeDb.SaveRouterGroupArgsForCall(0)
						Expect(err).NotTo(HaveOccurred())
						Expect(savedGroup.Guid).NotTo(Equal("some-other-guid"))
						Expect(savedGroup.Name).To(Equal("test-group"))
						Expect(savedGroup.Type).To(Equal(models.RouterGroupType("http")))
						Expect(responseRecorder.Code).To(Equal(http.StatusCreated))
						payload := responseRecorder.Body.String()
						jsonPayload := fmt.Sprintf("\n{\n\"guid\": \"%s\",\n\"name\": \"test-group\",\n\"type\": \"http\",\n\"reservable_ports\":\"\"\n}", savedGroup.Guid)
						Expect(payload).To(MatchJSON(jsonPayload))
					})
				})
			})
		})

		Describe("Router Group", func() {
			Context("when the db fails to save router group", func() {
				BeforeEach(func() {
					fakeDb.SaveRouterGroupReturns(errors.New("db communication failed"))
				})

				It("returns a DB communication error", func() {
					var err error

					bodyBytes := `{"name":"test-group","type":"tcp","reservable_ports":"1025"}`
					body := bytes.NewReader([]byte(bodyBytes))
					request, err := http.NewRequest(
						"POST",
						routing_api.CreateRouterGroup,
						body,
					)

					Expect(err).NotTo(HaveOccurred())
					routerGroupHandler.CreateRouterGroup(responseRecorder, request)

					Expect(responseRecorder.Code).To(Equal(http.StatusServiceUnavailable))
					Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(1))
					payload := responseRecorder.Body.String()
					Expect(payload).To(MatchJSON(`{
					"name": "DBCommunicationError",
					"message": "db communication failed"
				}`))
				})
			})
			It("does not save the router group and returns a bad request response", func() {
				var err error

				bodyBytes := []byte("invalid json")
				body := bytes.NewReader(bodyBytes)
				request, err := http.NewRequest(
					"POST",
					routing_api.CreateRouterGroup,
					body,
				)
				Expect(err).NotTo(HaveOccurred())
				routerGroupHandler.CreateRouterGroup(responseRecorder, request)
				Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(0))
				Expect(responseRecorder.Code).To(Equal(http.StatusBadRequest))
			})

			It("checks for routing.router_groups.write scope", func() {
				var err error
				bodyBytes := []byte(`{"name":"test-group","type":"tcp","reservable_ports":"1025"}`)
				body := bytes.NewReader(bodyBytes)
				request, err = http.NewRequest(
					"POST",
					routing_api.CreateRouterGroup,
					body,
				)
				Expect(err).NotTo(HaveOccurred())
				routerGroupHandler.CreateRouterGroup(responseRecorder, request)
				_, permission := fakeClient.DecodeTokenArgsForCall(0)
				Expect(permission).To(ConsistOf(handlers.RouterGroupsWriteScope))
			})

			Context("when authorization token is invalid", func() {
				var (
					currentCount int64
				)
				BeforeEach(func() {
					currentCount = metrics.GetTokenErrors()
					fakeClient.DecodeTokenReturns(errors.New("kaboom"))
				})
				It("returns Unauthorized error", func() {
					var err error

					bodyBytes := []byte(`{"name":"test-group","type":"tcp","reservable_ports":"1025"}`)
					body := bytes.NewReader(bodyBytes)
					request, err = http.NewRequest(
						"POST",
						routing_api.CreateRouterGroup,
						body,
					)
					Expect(err).NotTo(HaveOccurred())
					routerGroupHandler.CreateRouterGroup(responseRecorder, request)
					Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(0))
					Expect(responseRecorder.Code).To(Equal(http.StatusUnauthorized))
					Expect(metrics.GetTokenErrors()).To(Equal(currentCount + 1))
				})
			})
		})

		Describe("TCP Router Group", func() {
			Context("when the request body is invalid", func() {
				Context("when reservable_ports is not set", func() {
					It("does not save the router group and returns a bad request response", func() {
						var err error

						bodyBytes := []byte(`{"name":"test-group","type":"tcp"}`)
						body := bytes.NewReader(bodyBytes)
						request, err := http.NewRequest(
							"POST",
							routing_api.CreateRouterGroup,
							body,
						)
						Expect(err).NotTo(HaveOccurred())

						routerGroupHandler.CreateRouterGroup(responseRecorder, request)
						Expect(err).NotTo(HaveOccurred())
						Expect(responseRecorder.Code).To(Equal(http.StatusBadRequest))
						responseBody, err := ioutil.ReadAll(responseRecorder.Body)
						Expect(err).NotTo(HaveOccurred())
						Expect(responseBody).To(ContainSubstring("Missing reservable_ports in router group: test-group"))
						Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(0))
					})
				})

				Context("when reservable_ports is invalid", func() {
					It("does not save the router group and returns a bad request response", func() {
						var err error

						bodyBytes := []byte(`{"name":"test-group","type":"tcp", "reservable_ports":"1000"}`)
						body := bytes.NewReader(bodyBytes)
						request, err := http.NewRequest(
							"POST",
							routing_api.CreateRouterGroup,
							body,
						)
						Expect(err).NotTo(HaveOccurred())

						routerGroupHandler.CreateRouterGroup(responseRecorder, request)
						Expect(err).NotTo(HaveOccurred())
						Expect(responseRecorder.Code).To(Equal(http.StatusBadRequest))
						Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(0))
					})
				})
			})

			Context("when the request body is valid", func() {
				It("saves the router group", func() {
					var err error
					bodyBytes := []byte(`{"name":"test-group","type":"tcp", "reservable_ports":"2000-3000"}`)
					body := bytes.NewReader(bodyBytes)
					request, err = http.NewRequest(
						"POST",
						routing_api.CreateRouterGroup,
						body,
					)
					Expect(err).NotTo(HaveOccurred())

					routerGroupHandler.CreateRouterGroup(responseRecorder, request)

					Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(1))
					savedGroup := fakeDb.SaveRouterGroupArgsForCall(0)
					Expect(err).NotTo(HaveOccurred())
					Expect(savedGroup.Guid).NotTo(BeEmpty())
					Expect(savedGroup.Name).To(Equal("test-group"))
					Expect(savedGroup.Type).To(Equal(models.RouterGroupType("tcp")))
					Expect(responseRecorder.Code).To(Equal(http.StatusCreated))
					payload := responseRecorder.Body.String()
					jsonPayload := fmt.Sprintf("\n{\n\"guid\": \"%s\",\n\"name\": \"test-group\",\n\"type\": \"tcp\",\n\"reservable_ports\":\"2000-3000\"\n}", savedGroup.Guid)
					Expect(payload).To(MatchJSON(jsonPayload))
				})

				Context("when extra fields (i.e. guid) are provided", func() {
					It("doesn't do anything with the extra fields", func() {
						var err error
						bodyBytes := []byte(`{"guid":"some-other-guid","name":"test-group","type":"tcp","banana":"fake-banana","reservable_ports":"1035"}`)
						body := bytes.NewReader(bodyBytes)
						request, err = http.NewRequest(
							"POST",
							routing_api.CreateRouterGroup,
							body,
						)
						Expect(err).NotTo(HaveOccurred())

						routerGroupHandler.CreateRouterGroup(responseRecorder, request)

						Expect(fakeDb.SaveRouterGroupCallCount()).To(Equal(1))
						savedGroup := fakeDb.SaveRouterGroupArgsForCall(0)
						Expect(err).NotTo(HaveOccurred())
						Expect(savedGroup.Guid).NotTo(Equal("some-other-guid"))
						Expect(savedGroup.Name).To(Equal("test-group"))
						Expect(savedGroup.Type).To(Equal(models.RouterGroupType("tcp")))
						Expect(savedGroup.ReservablePorts).To(Equal(models.ReservablePorts("1035")))
						Expect(responseRecorder.Code).To(Equal(http.StatusCreated))
						payload := responseRecorder.Body.String()
						jsonPayload := fmt.Sprintf("\n{\n\"guid\": \"%s\",\n\"name\": \"test-group\",\n\"type\": \"tcp\",\n\"reservable_ports\":\"1035\"\n}", savedGroup.Guid)
						Expect(payload).To(MatchJSON(jsonPayload))
					})
				})
			})
		})
	})
})
