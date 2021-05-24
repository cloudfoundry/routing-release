package routingapi_test

import (
	"errors"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/routing-release/route-registrar/config"
	"code.cloudfoundry.org/routing-release/route-registrar/routingapi"
	"code.cloudfoundry.org/routing-release/routing-api/fake_routing_api"
	"code.cloudfoundry.org/routing-release/routing-api/models"

	"code.cloudfoundry.org/lager/lagertest"
	fakeuaa "code.cloudfoundry.org/uaa-go-client/fakes"
	"code.cloudfoundry.org/uaa-go-client/schema"
)

var _ = Describe("Routing API", func() {
	var (
		client    *fake_routing_api.FakeClient
		uaaClient *fakeuaa.FakeClient

		api    *routingapi.RoutingAPI
		logger lager.Logger

		port         int
		externalPort int
		ttl          int
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("routing api test")
		uaaClient = &fakeuaa.FakeClient{}
		uaaClient.FetchTokenReturns(&schema.Token{AccessToken: "my-token"}, nil)
		client = &fake_routing_api.FakeClient{}
		api = routingapi.NewRoutingAPI(logger, uaaClient, client)

		port = 1234
		externalPort = 5678
		ttl = 100
	})

	Describe("RegisterRoute", func() {
		BeforeEach(func() {
			client.RouterGroupWithNameReturns(models.RouterGroup{Guid: "router-group-guid"}, nil)
		})

		Context("when given a valid route", func() {
			It("registers the route", func() {
				err := api.RegisterRoute(config.Route{
					Name:                 "test-route",
					Port:                 &port,
					ExternalPort:         &externalPort,
					Host:                 "myhost",
					RegistrationInterval: 100 * time.Second,
					RouterGroup:          "my-router-group",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(uaaClient.FetchTokenCallCount()).To(Equal(1))
				Expect(uaaClient.FetchTokenArgsForCall(0)).To(BeFalse())

				Expect(client.SetTokenCallCount()).To(Equal(1))
				Expect(client.SetTokenArgsForCall(0)).To(Equal("my-token"))

				Expect(client.RouterGroupWithNameCallCount()).To(Equal(1))
				Expect(client.RouterGroupWithNameArgsForCall(0)).To(Equal("my-router-group"))

				routeMapping := models.TcpRouteMapping{TcpMappingEntity: models.TcpMappingEntity{
					RouterGroupGuid: "router-group-guid",
					HostPort:        1234,
					ExternalPort:    5678,
					HostIP:          "myhost",
					TTL:             &ttl,
				}}
				Expect(client.UpsertTcpRouteMappingsCallCount()).To(Equal(1))
				Expect(client.UpsertTcpRouteMappingsArgsForCall(0)).To(Equal([]models.TcpRouteMapping{routeMapping}))
			})
		})

		Context("when the route mapping fails to register", func() {
			BeforeEach(func() {
				client.UpsertTcpRouteMappingsReturns(errors.New("registration error"))
			})

			It("returns an error", func() {
				err := api.RegisterRoute(config.Route{
					Name:                 "test-route",
					Port:                 &port,
					ExternalPort:         &externalPort,
					Host:                 "myhost",
					RegistrationInterval: 100 * time.Second,
					RouterGroup:          "my-router-group",
				})

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("registration error"))
			})
		})
	})

	Describe("UnregisterRoute", func() {
		BeforeEach(func() {
			client.RouterGroupWithNameReturns(models.RouterGroup{Guid: "router-group-guid"}, nil)
		})

		Context("when given a valid route", func() {
			It("unregisters the route", func() {
				err := api.UnregisterRoute(config.Route{
					Name:                 "test-route",
					Port:                 &port,
					ExternalPort:         &externalPort,
					Host:                 "myhost",
					RegistrationInterval: 100 * time.Second,
					RouterGroup:          "my-router-group",
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(uaaClient.FetchTokenCallCount()).To(Equal(1))
				Expect(uaaClient.FetchTokenArgsForCall(0)).To(BeFalse())

				Expect(client.SetTokenCallCount()).To(Equal(1))
				Expect(client.SetTokenArgsForCall(0)).To(Equal("my-token"))

				Expect(client.RouterGroupWithNameCallCount()).To(Equal(1))
				Expect(client.RouterGroupWithNameArgsForCall(0)).To(Equal("my-router-group"))

				routeMapping := models.TcpRouteMapping{TcpMappingEntity: models.TcpMappingEntity{
					RouterGroupGuid: "router-group-guid",
					HostPort:        1234,
					ExternalPort:    5678,
					HostIP:          "myhost",
					TTL:             &ttl,
				}}

				Expect(client.DeleteTcpRouteMappingsCallCount()).To(Equal(1))
				Expect(client.DeleteTcpRouteMappingsArgsForCall(0)).To(Equal([]models.TcpRouteMapping{routeMapping}))
			})
		})

		Context("when the route mapping fails to unregister", func() {
			BeforeEach(func() {
				client.DeleteTcpRouteMappingsReturns(errors.New("unregistration error"))
			})
			It("returns an error", func() {
				err := api.UnregisterRoute(config.Route{
					Name:                 "test-route",
					Port:                 &port,
					ExternalPort:         &externalPort,
					Host:                 "myhost",
					RegistrationInterval: 100 * time.Second,
					RouterGroup:          "my-router-group",
				})

				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("unregistration error"))
			})
		})
	})

	Context("when an error occurs", func() {
		Context("when a UAA token cannot be fetched", func() {
			BeforeEach(func() {
				uaaClient.FetchTokenReturns(&schema.Token{}, errors.New("my fetch error"))
			})

			It("returns an error", func() {
				err := api.RegisterRoute(config.Route{})
				Expect(uaaClient.FetchTokenCallCount()).To(Equal(1))
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("my fetch error"))
			})
		})

		Context("when the router group name fails to return", func() {
			BeforeEach(func() {
				client.RouterGroupWithNameReturns(models.RouterGroup{}, errors.New("my router group failed"))
			})

			It("returns an error", func() {
				err := api.RegisterRoute(config.Route{
					Name:                 "test-route",
					Port:                 &port,
					ExternalPort:         &externalPort,
					Host:                 "myhost",
					RegistrationInterval: 100 * time.Second,
					RouterGroup:          "my-router-group",
				})
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError("my router group failed"))
			})
		})
	})
})
