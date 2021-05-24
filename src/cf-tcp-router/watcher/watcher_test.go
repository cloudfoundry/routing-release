package watcher_test

import (
	"errors"
	"os"
	"time"

	fake_routing_table "code.cloudfoundry.org/routing-release/cf-tcp-router/routing_table/fakes"
	"code.cloudfoundry.org/routing-release/cf-tcp-router/watcher"
	"code.cloudfoundry.org/routing-release/routing-api"
	"code.cloudfoundry.org/routing-release/routing-api/fake_routing_api"
	"code.cloudfoundry.org/routing-release/routing-api/models"
	testUaaClient "code.cloudfoundry.org/uaa-go-client/fakes"
	"code.cloudfoundry.org/uaa-go-client/schema"
	"github.com/tedsuo/ifrit"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Watcher", func() {
	const (
		routerGroupGuid = "rtrGrp0001"
	)
	var (
		eventSource      *fake_routing_api.FakeTcpEventSource
		routingApiClient *fake_routing_api.FakeClient
		uaaClient        *testUaaClient.FakeClient
		testWatcher      *watcher.Watcher
		process          ifrit.Process
		syncChannel      chan struct{}
		updater          *fake_routing_table.FakeUpdater
	)

	BeforeEach(func() {
		eventSource = new(fake_routing_api.FakeTcpEventSource)
		routingApiClient = new(fake_routing_api.FakeClient)
		updater = new(fake_routing_table.FakeUpdater)
		uaaClient = &testUaaClient.FakeClient{}
		token := &schema.Token{
			AccessToken: "access_token",
			ExpiresIn:   5,
		}
		uaaClient.FetchTokenReturns(token, nil)

		routingApiClient.SubscribeToTcpEventsReturns(eventSource, nil)
		syncChannel = make(chan struct{})
		testWatcher = watcher.New(routingApiClient, updater, uaaClient, 1, syncChannel, logger)
	})

	JustBeforeEach(func() {
		process = ifrit.Invoke(testWatcher)
	})

	AfterEach(func() {
		process.Signal(os.Interrupt)
		Eventually(process.Wait()).Should(Receive())
		Eventually(logger).Should(gbytes.Say("test.watcher.stopping"))
	})

	Context("handle UpsertEvent", func() {
		var (
			tcpEvent routing_api.TcpEvent
		)

		BeforeEach(func() {
			tcpEvent = routing_api.TcpEvent{
				TcpRouteMapping: models.NewTcpRouteMapping(
					routerGroupGuid,
					61000,
					"some-ip-1",
					5222,
					0,
				),
				Action: "Upsert",
			}
			eventSource.NextReturns(tcpEvent, nil)
		})

		It("calls updater HandleEvent", func() {
			Eventually(updater.HandleEventCallCount).Should(BeNumerically(">=", 1))
			upsertEvent := updater.HandleEventArgsForCall(0)
			Expect(upsertEvent).Should(Equal(tcpEvent))
		})
	})

	Context("handle DeleteEvent", func() {
		var (
			tcpEvent routing_api.TcpEvent
		)

		BeforeEach(func() {
			tcpEvent = routing_api.TcpEvent{
				TcpRouteMapping: models.NewTcpRouteMapping(
					routerGroupGuid,
					61000,
					"some-ip-1",
					5222,
					0,
				),
				Action: "Delete",
			}
			eventSource.NextReturns(tcpEvent, nil)
		})

		It("calls updater HandleEvent", func() {
			Eventually(updater.HandleEventCallCount).Should(BeNumerically(">=", 1))
			deleteEvent := updater.HandleEventArgsForCall(0)
			Expect(deleteEvent).Should(Equal(tcpEvent))
		})
	})

	Context("handle Sync Event", func() {
		JustBeforeEach(func() {
			syncChannel <- struct{}{}
		})

		It("calls updater Sync", func() {
			Eventually(updater.SyncCallCount).Should(Equal(1))
		})
	})

	Context("when eventSource returns error", func() {
		BeforeEach(func() {
			eventSource.NextReturns(routing_api.TcpEvent{}, errors.New("buzinga.."))
		})

		It("resubscribes to SSE from routing api", func() {
			Eventually(routingApiClient.SubscribeToTcpEventsCallCount).Should(BeNumerically(">=", 2))
			Eventually(logger).Should(gbytes.Say("failed-to-get-next-routing-api-event"))
		})

		It("closes the current eventSource", func() {
			Eventually(eventSource.CloseCallCount).Should(BeNumerically(">=", 1))
		})
	})

	Context("when subscribe to events fails", func() {
		var (
			routingApiErrChannel chan error
		)
		BeforeEach(func() {
			routingApiErrChannel = make(chan error)

			routingApiClient.SubscribeToTcpEventsStub = func() (routing_api.TcpEventSource, error) {
				select {
				case err := <-routingApiErrChannel:
					if err != nil {
						return nil, err
					}
				}
				return eventSource, nil
			}

			testWatcher = watcher.New(routingApiClient, updater, uaaClient, 1, syncChannel, logger)
		})

		Context("with error other than unauthorized", func() {
			It("uses the cached token and retries to subscribe", func() {
				Eventually(uaaClient.FetchTokenCallCount, 5*time.Second, 1*time.Second).Should(Equal(1))
				Expect(uaaClient.FetchTokenArgsForCall(0)).To(BeFalse())
				routingApiErrChannel <- errors.New("kaboom")
				close(routingApiErrChannel)
				Eventually(routingApiClient.SubscribeToTcpEventsCallCount, 5*time.Second, 1*time.Second).Should(Equal(2))
				Eventually(logger).Should(gbytes.Say("failed-subscribing-to-routing-api-event-stream"))
				Eventually(uaaClient.FetchTokenCallCount, 5*time.Second, 1*time.Second).Should(Equal(2))
				Expect(uaaClient.FetchTokenArgsForCall(1)).To(BeFalse())
			})
		})

		Context("with unauthorized error", func() {
			It("fetches a new token and retries to subscribe", func() {
				Eventually(uaaClient.FetchTokenCallCount, 5*time.Second, 1*time.Second).Should(Equal(1))
				Expect(uaaClient.FetchTokenArgsForCall(0)).To(BeFalse())
				routingApiErrChannel <- errors.New("unauthorized")
				Eventually(routingApiClient.SubscribeToTcpEventsCallCount, 5*time.Second, 1*time.Second).Should(Equal(2))
				Eventually(logger).Should(gbytes.Say("failed-subscribing-to-routing-api-event-stream"))
				Eventually(uaaClient.FetchTokenCallCount, 5*time.Second, 1*time.Second).Should(Equal(2))
				Expect(uaaClient.FetchTokenArgsForCall(1)).To(BeTrue())

				By("resumes to use cache token for subsequent errors")
				routingApiErrChannel <- errors.New("kaboom")
				close(routingApiErrChannel)
				Eventually(routingApiClient.SubscribeToTcpEventsCallCount, 5*time.Second, 1*time.Second).Should(Equal(3))
				Eventually(logger).Should(gbytes.Say("failed-subscribing-to-routing-api-event-stream"))
				Eventually(uaaClient.FetchTokenCallCount, 5*time.Second, 1*time.Second).Should(Equal(3))
				Expect(uaaClient.FetchTokenArgsForCall(2)).To(BeFalse())
			})
		})
	})

	Context("when the token fetcher returns an error", func() {
		BeforeEach(func() {
			uaaClient.FetchTokenReturns(nil, errors.New("token fetcher error"))
		})

		It("returns an error", func() {
			Eventually(logger).Should(gbytes.Say("error-fetching-token"))
			Eventually(uaaClient.FetchTokenCallCount, 5*time.Second, 1*time.Second).Should(BeNumerically(">", 2))
		})
	})

})
