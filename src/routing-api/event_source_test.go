package routing_api_test

import (
	"errors"

	"bytes"
	"encoding/json"
	"io/ioutil"

	"code.cloudfoundry.org/routing-release/routing-api"
	"code.cloudfoundry.org/routing-release/routing-api/fake_routing_api"
	"code.cloudfoundry.org/routing-release/routing-api/models"
	trace "code.cloudfoundry.org/trace-logger"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vito/go-sse/sse"
)

var _ = Describe("EventSource", func() {

	var fakeRawEventSource *fake_routing_api.FakeRawEventSource

	BeforeEach(func() {
		fakeRawEventSource = &fake_routing_api.FakeRawEventSource{}
	})

	Describe("Http events", func() {
		var eventSource routing_api.EventSource

		BeforeEach(func() {
			eventSource = routing_api.NewEventSource(fakeRawEventSource)
		})

		Describe("Next", func() {
			Context("When the event source returns an error", func() {
				It("returns the error", func() {
					fakeRawEventSource.NextReturns(sse.Event{}, errors.New("boom"))
					_, err := eventSource.Next()
					Expect(err.Error()).To(Equal("boom"))
				})
			})

			Context("When the event source successfully returns an event", func() {
				It("logs the event", func() {
					stdout := bytes.NewBuffer([]byte{})
					trace.SetStdout(stdout)
					trace.Logger = trace.NewLogger("true")
					rawEvent := sse.Event{
						ID:    "1",
						Name:  "Test",
						Data:  []byte(`{"route":"jim.com","port":8080,"ip":"1.1.1.1","ttl":60,"log_guid":"logs"}`),
						Retry: 1,
					}
					expectedJSON, _ := json.Marshal(rawEvent)

					fakeRawEventSource.NextReturns(rawEvent, nil)
					_, err := eventSource.Next()
					Expect(err).ToNot(HaveOccurred())

					log, err := ioutil.ReadAll(stdout)
					Expect(err).NotTo(HaveOccurred())
					Expect(log).To(ContainSubstring("EVENT: "))
					Expect(log).To(ContainSubstring(string(expectedJSON)))
				})

				Context("When the event is unmarshalled successfully", func() {
					It("returns the raw event", func() {
						rawEvent := sse.Event{
							ID:    "1",
							Name:  "Test",
							Data:  []byte(`{"route":"jim.com","port":8080,"ip":"1.1.1.1","ttl":60,"log_guid":"logs"}`),
							Retry: 1,
						}

						route := models.NewRoute("jim.com", 8080, "1.1.1.1", "logs", "", 60)
						expectedEvent := routing_api.Event{
							Route:  route,
							Action: "Test",
						}

						fakeRawEventSource.NextReturns(rawEvent, nil)
						event, err := eventSource.Next()
						Expect(err).ToNot(HaveOccurred())
						Expect(event).To(Equal(expectedEvent))
					})
				})

				Context("When the event is unmarshalled successfully", func() {
					It("returns the error", func() {
						rawEvent := sse.Event{
							ID:    "1",
							Name:  "Invalid",
							Data:  []byte("This isn't valid json"),
							Retry: 1,
						}

						fakeRawEventSource.NextReturns(rawEvent, nil)
						_, err := eventSource.Next()
						Expect(err).To(HaveOccurred())
					})
				})
			})
		})

		Describe("Close", func() {
			Context("when closing the raw event source succeeds", func() {
				It("closes the event source", func() {
					err := eventSource.Close()
					Expect(err).ToNot(HaveOccurred())
					Expect(fakeRawEventSource.CloseCallCount()).To(Equal(1))
				})
			})

			Context("when closing the raw event source fails", func() {
				It("returns the error", func() {
					expectedError := errors.New("close failed")
					fakeRawEventSource.CloseReturns(expectedError)
					err := eventSource.Close()
					Expect(fakeRawEventSource.CloseCallCount()).To(Equal(1))
					Expect(err).To(Equal(expectedError))
				})
			})
		})
	})

	Describe("Tcp events", func() {
		var tcpEventSource routing_api.TcpEventSource

		BeforeEach(func() {
			tcpEventSource = routing_api.NewTcpEventSource(fakeRawEventSource)
		})

		Describe("Next", func() {
			Context("When the event source returns an error", func() {
				It("returns the error", func() {
					fakeRawEventSource.NextReturns(sse.Event{}, errors.New("boom"))
					_, err := tcpEventSource.Next()
					Expect(err.Error()).To(Equal("boom"))
				})
			})

			Context("When the event source successfully returns an event", func() {
				It("logs the event", func() {
					stdout := bytes.NewBuffer([]byte{})
					trace.SetStdout(stdout)
					trace.Logger = trace.NewLogger("true")
					rawEvent := sse.Event{
						ID:    "1",
						Name:  "Test",
						Data:  []byte(`{"router_group_guid": "rguid1", "port":52000, "backend_port":60000,"backend_ip":"1.1.1.1"}`),
						Retry: 1,
					}
					expectedJSON, _ := json.Marshal(rawEvent)

					fakeRawEventSource.NextReturns(rawEvent, nil)
					_, err := tcpEventSource.Next()
					Expect(err).ToNot(HaveOccurred())

					log, err := ioutil.ReadAll(stdout)
					Expect(err).NotTo(HaveOccurred())
					Expect(log).To(ContainSubstring("EVENT: "))
					Expect(log).To(ContainSubstring(string(expectedJSON)))
				})

				Context("When the event is unmarshalled successfully", func() {
					It("returns the raw event", func() {
						rawEvent := sse.Event{
							ID:    "1",
							Name:  "Test",
							Data:  []byte(`{"router_group_guid": "rguid1", "port":52000, "backend_port":60000,"backend_ip":"1.1.1.1","modification_tag":{"guid":"my-guid","index":5}}`),
							Retry: 1,
						}

						modTag := models.ModificationTag{
							Guid:  "my-guid",
							Index: 5,
						}
						tcpMapping := models.NewTcpRouteMapping("rguid1", 52000, "1.1.1.1", 60000, 5)
						tcpMapping.ModificationTag = modTag
						tcpMapping.TTL = nil

						expectedEvent := routing_api.TcpEvent{
							TcpRouteMapping: tcpMapping,
							Action:          "Test",
						}

						fakeRawEventSource.NextReturns(rawEvent, nil)
						event, err := tcpEventSource.Next()
						Expect(err).ToNot(HaveOccurred())
						Expect(event).To(Equal(expectedEvent))
					})
				})

				Context("When the event has invalid json", func() {
					It("returns the error", func() {
						rawEvent := sse.Event{
							ID:    "1",
							Name:  "Invalid",
							Data:  []byte("This isn't valid json"),
							Retry: 1,
						}

						fakeRawEventSource.NextReturns(rawEvent, nil)
						_, err := tcpEventSource.Next()
						Expect(err).To(HaveOccurred())
					})
				})
			})
		})

		Describe("Close", func() {
			Context("when closing the raw event source succeeds", func() {
				It("closes the event source", func() {
					err := tcpEventSource.Close()
					Expect(err).ToNot(HaveOccurred())
					Expect(fakeRawEventSource.CloseCallCount()).To(Equal(1))
				})
			})

			Context("when closing the raw event source fails", func() {
				It("returns the error", func() {
					expectedError := errors.New("close failed")
					fakeRawEventSource.CloseReturns(expectedError)
					err := tcpEventSource.Close()
					Expect(fakeRawEventSource.CloseCallCount()).To(Equal(1))
					Expect(err).To(Equal(expectedError))
				})
			})
		})
	})
})
