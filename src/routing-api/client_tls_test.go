package routing_api_test

import (
	"bytes"
	"encoding/json"
	"net/http"

	"code.cloudfoundry.org/routing-release/routing-api"
	"code.cloudfoundry.org/routing-release/routing-api/models"
	"code.cloudfoundry.org/trace-logger"
	"github.com/vito/go-sse/sse"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

var _ = Describe("Client", func() {
	const (
		ROUTES_API_URL = "/routing/v1/routes"
		EVENTS_SSE_URL = "/routing/v1/events"
	)
	var server *ghttp.Server
	var client routing_api.Client
	var stdout *bytes.Buffer

	BeforeEach(func() {
		stdout = bytes.NewBuffer([]byte{})
		trace.SetStdout(stdout)
		trace.Logger = trace.NewLogger("true")
	})

	BeforeEach(func() {
		server = ghttp.NewTLSServer()
		data, _ := json.Marshal([]models.Route{})
		server.RouteToHandler("GET", ROUTES_API_URL,
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", ROUTES_API_URL),
				ghttp.RespondWith(http.StatusOK, data),
			),
		)

		event := sse.Event{
			ID:   "1",
			Name: "Upsert",
			Data: data,
		}

		headers := make(http.Header)
		headers.Set("Content-Type", "text/event-stream; charset=utf-8")

		server.RouteToHandler("GET", EVENTS_SSE_URL,
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", EVENTS_SSE_URL),
				ghttp.RespondWith(http.StatusOK, event.Encode(), headers),
			),
		)
	})

	AfterEach(func() {
		server.Close()
	})

	Context("without skip SSL validation", func() {
		BeforeEach(func() {
			client = routing_api.NewClient(server.URL(), false)
		})

		It("fails to connect to the Routing API", func() {
			_, err := client.Routes()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("x509: certificate signed by unknown authority"))
		})

		It("fails to stream events from the Routing API", func() {
			_, err := client.SubscribeToEventsWithMaxRetries(1)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("x509: certificate signed by unknown authority"))
		})
	})

	Context("with skip SSL validation", func() {
		BeforeEach(func() {
			client = routing_api.NewClient(server.URL(), true)
		})

		It("successfully connect to the Routing API", func() {
			_, err := client.Routes()
			Expect(err).ToNot(HaveOccurred())
		})

		It("streams events from the Routing API", func() {
			_, err := client.SubscribeToEventsWithMaxRetries(1)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
