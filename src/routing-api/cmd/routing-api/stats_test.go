package main_test

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"time"

	"code.cloudfoundry.org/routing-release/routing-api/cmd/routing-api/testrunner"
	"code.cloudfoundry.org/routing-release/routing-api/models"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Routes API", func() {
	var (
		err               error
		route1            models.Route
		addr              *net.UDPAddr
		fakeStatsdServer  *net.UDPConn
		fakeStatsdChan    chan string
		routingAPIProcess ifrit.Process
		configFilePath    string
	)

	BeforeEach(func() {
		routingAPIConfig := getRoutingAPIConfig(defaultConfig)
		configFilePath = writeConfigToTempFile(routingAPIConfig)
		routingAPIRunner := testrunner.New(routingAPIBinPath, testrunner.Args{
			IP:         routingAPIIP,
			ConfigPath: configFilePath,
			DevMode:    true,
		})
		routingAPIProcess = ginkgomon.Invoke(routingAPIRunner)
		Eventually(routingAPIProcess.Ready(), "5s").Should(BeClosed())
		addr, err = net.ResolveUDPAddr("udp", fmt.Sprintf("localhost:%d", 8125+GinkgoParallelNode()))
		Expect(err).ToNot(HaveOccurred())

		fakeStatsdServer, err = net.ListenUDP("udp", addr)
		Expect(err).ToNot(HaveOccurred())

		err = fakeStatsdServer.SetReadDeadline(time.Now().Add(15 * time.Second))
		Expect(err).ToNot(HaveOccurred())

		fakeStatsdChan = make(chan string, 1)

		go func(statsChan chan string) {
			defer GinkgoRecover()
			defer close(statsChan)

			for {
				buffer := make([]byte, 1000)
				_, err := fakeStatsdServer.Read(buffer)
				if err != nil {
					return
				}
				scanner := bufio.NewScanner(bytes.NewBuffer(buffer))
				for scanner.Scan() {
					select {
					case statsChan <- scanner.Text():
					}
				}
			}
		}(fakeStatsdChan)

		time.Sleep(1000 * time.Millisecond)
	})

	AfterEach(func() {
		ginkgomon.Kill(routingAPIProcess)
		err := fakeStatsdServer.Close()
		Expect(err).ToNot(HaveOccurred())
		Eventually(fakeStatsdChan).Should(BeClosed())

		err = os.RemoveAll(configFilePath)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("Stats for event subscribers", func() {
		Context("Subscribe", func() {
			It("should increase subscriptions by 4", func() {

				eventStream1, err := client.SubscribeToEvents()
				Expect(err).NotTo(HaveOccurred())
				defer func() {
					err := eventStream1.Close()
					Expect(err).NotTo(HaveOccurred())
				}()

				eventStream2, err := client.SubscribeToEvents()
				Expect(err).NotTo(HaveOccurred())
				defer func() {
					err := eventStream2.Close()
					Expect(err).NotTo(HaveOccurred())
				}()

				eventStream3, err := client.SubscribeToEvents()
				Expect(err).NotTo(HaveOccurred())
				defer func() {
					err := eventStream3.Close()
					Expect(err).NotTo(HaveOccurred())
				}()

				eventStream4, err := client.SubscribeToEvents()
				Expect(err).NotTo(HaveOccurred())
				defer func() {
					err := eventStream4.Close()
					Expect(err).NotTo(HaveOccurred())
				}()

				Eventually(fakeStatsdChan).Should(Receive(Equal("routing_api.total_http_subscriptions:+1|g")))
				Eventually(fakeStatsdChan).Should(Receive(Equal("routing_api.total_http_subscriptions:+1|g")))
				Eventually(fakeStatsdChan).Should(Receive(Equal("routing_api.total_http_subscriptions:+1|g")))
				Eventually(fakeStatsdChan).Should(Receive(Equal("routing_api.total_http_subscriptions:+1|g")))
			})
		})
	})

	Describe("Stats for total routes", func() {

		BeforeEach(func() {
			route1 = models.NewRoute("a.b.c", 33, "1.1.1.1", "potato", "", 55)
		})

		Context("periodically receives total routes", func() {
			It("Gets statsd messages for existing routes", func() {
				//The first time is because we get the event of adding the self route
				Eventually(fakeStatsdChan).Should(Receive(Equal("routing_api.total_http_routes:1|g")))
				//Do it again to make sure it's not because of events
				Eventually(fakeStatsdChan).Should(Receive(Equal("routing_api.total_http_routes:1|g")))
			})
		})

		Context("when creating and updating a new route", func() {
			AfterEach(func() {
				err := client.DeleteRoutes([]models.Route{route1})
				Expect(err).ToNot(HaveOccurred())
			})

			It("Gets statsd messages for new routes", func() {
				err := client.UpsertRoutes([]models.Route{route1})
				Expect(err).ToNot(HaveOccurred())

				Eventually(fakeStatsdChan).Should(Receive(Equal("routing_api.total_http_routes:+1|g")))
			})
		})

		Context("when deleting a route", func() {
			It("gets statsd messages for deleted routes", func() {
				err := client.UpsertRoutes([]models.Route{route1})
				Expect(err).ToNot(HaveOccurred())

				err = client.DeleteRoutes([]models.Route{route1})
				Expect(err).ToNot(HaveOccurred())

				Eventually(fakeStatsdChan).Should(Receive(Equal("routing_api.total_http_routes:+1|g")))
				Eventually(fakeStatsdChan).Should(Receive(Equal("routing_api.total_http_routes:-1|g")))
			})
		})

		Context("when expiring a route", func() {
			It("gets statsd messages for expired routes", func() {
				routeExpire := models.NewRoute("z.a.k", 63, "42.42.42.42", "Tomato", "", 1)

				err := client.UpsertRoutes([]models.Route{routeExpire})
				Expect(err).ToNot(HaveOccurred())

				Eventually(fakeStatsdChan).Should(Receive(Equal("routing_api.total_http_routes:+1|g")))
				Eventually(fakeStatsdChan).Should(Receive(Equal("routing_api.total_http_routes:-1|g")))
			})
		})
	})
})
