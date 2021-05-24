package routingapi

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/routing-release/route-registrar/config"
	"code.cloudfoundry.org/routing-release/routing-api/fake_routing_api"
	"code.cloudfoundry.org/routing-release/routing-api/models"
	fakeuaa "code.cloudfoundry.org/uaa-go-client/fakes"
	"code.cloudfoundry.org/uaa-go-client/schema"
)

var _ = Describe("Routing API", func() {
	var (
		client    *fake_routing_api.FakeClient
		uaaClient *fakeuaa.FakeClient

		api    *RoutingAPI
		logger lager.Logger

		port         int
		externalPort int
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("routing api test")
		uaaClient = &fakeuaa.FakeClient{}
		uaaClient.FetchTokenReturns(&schema.Token{AccessToken: "my-token"}, nil)
		client = &fake_routing_api.FakeClient{}
		client.RouterGroupWithNameReturns(models.RouterGroup{Guid: "router-group-guid"}, nil)
		api = NewRoutingAPI(logger, uaaClient, client)

		port = 1234
		externalPort = 5678
	})

	It("Sets SNI hostname if ServerCertDomainSAN is present.", func() {
		tcpRouteMapping, err := api.makeTcpRouteMapping(config.Route{
			Port:                &port,
			ExternalPort:        &externalPort,
			RouterGroup:         "my-router-group",
			ServerCertDomainSAN: "sniHostname",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(tcpRouteMapping.SniHostname).ToNot(BeNil())
		Expect(*tcpRouteMapping.SniHostname).To(Equal("sniHostname"))
	})

	It("SNI hostname nil if ServerCertDomainSAN is not present.", func() {
		tcpRouteMapping, err := api.makeTcpRouteMapping(config.Route{
			Port:         &port,
			ExternalPort: &externalPort,
			RouterGroup:  "my-router-group",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(tcpRouteMapping.SniHostname).To(BeNil())
	})
})
