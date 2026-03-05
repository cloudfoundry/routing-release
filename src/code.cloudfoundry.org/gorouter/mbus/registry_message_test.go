package mbus_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "code.cloudfoundry.org/gorouter/mbus"
)

var _ = Describe("RegistryMessage", func() {
	Describe("ValidateMessage", func() {
		var message *RegistryMessage
		var payload []byte

		JustBeforeEach(func() {
			message = new(RegistryMessage)
			err := json.Unmarshal(payload, message)
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("With a payload with no route service url", func() {
			BeforeEach(func() {
				payload = []byte(`{"dea":"dea1","app":"app1","uris":["test.com"],"host":"1.2.3.4","port":1234,"tags":{},"private_instance_id":"private_instance_id"}`)
			})

			It("passes validation", func() {
				Expect(message.ValidateMessage()).To(BeTrue())
			})
		})

		Describe("With a payload with an empty route service url", func() {
			BeforeEach(func() {
				payload = []byte(`{"dea":"dea1","app":"app1","uris":["test.com"],"host":"1.2.3.4","port":1234,"tags":{},"route_service_url":"","private_instance_id":"private_instance_id"}`)
			})

			It("passes validation", func() {
				Expect(message.ValidateMessage()).To(BeTrue())
			})
		})

		Describe("With a payload with an https route service url", func() {
			BeforeEach(func() {
				payload = []byte(`{"dea":"dea1","app":"app1","uris":["test.com"],"host":"1.2.3.4","port":1234,"tags":{},"route_service_url":"https://www.my-route.me","private_instance_id":"private_instance_id"}`)
			})

			It("passes validation", func() {
				Expect(message.ValidateMessage()).To(BeTrue())
			})
		})

		Describe("With a payload with an http route service url", func() {
			BeforeEach(func() {
				payload = []byte(`{"dea":"dea1","app":"app1","uris":["test.com"],"host":"1.2.3.4","port":1234,"tags":{},"route_service_url":"http://www.my-insecure-route.com","private_instance_id":"private_instance_id"}`)
			})

			It("fails validation", func() {
				Expect(message.ValidateMessage()).To(BeFalse())
			})
		})
	})

	Describe("MakeEndpoint with AllowedSources", func() {
		var message *RegistryMessage
		var payload []byte

		JustBeforeEach(func() {
			message = new(RegistryMessage)
			err := json.Unmarshal(payload, message)
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("With allowed_sources at top level", func() {
			BeforeEach(func() {
				payload = []byte(`{
					"app":"app1",
					"uris":["test.com"],
					"host":"1.2.3.4",
					"port":1234,
					"tags":{},
					"private_instance_id":"private_instance_id",
					"allowed_sources": {
						"apps": ["app-guid-1", "app-guid-2"],
						"spaces": ["space-guid-1"],
						"orgs": ["org-guid-1"],
						"any": false
					}
				}`)
			})

			It("parses allowed_sources correctly", func() {
				endpoint, err := message.MakeEndpoint(false)
				Expect(err).NotTo(HaveOccurred())
				Expect(endpoint.AllowedSources).NotTo(BeNil())
				Expect(endpoint.AllowedSources.Apps).To(ConsistOf("app-guid-1", "app-guid-2"))
				Expect(endpoint.AllowedSources.Spaces).To(ConsistOf("space-guid-1"))
				Expect(endpoint.AllowedSources.Orgs).To(ConsistOf("org-guid-1"))
				Expect(endpoint.AllowedSources.Any).To(BeFalse())
			})
		})

		Describe("With allowed_sources nested in options (CAPI/Diego format)", func() {
			BeforeEach(func() {
				payload = []byte(`{
					"app":"app1",
					"uris":["test.com"],
					"host":"1.2.3.4",
					"port":1234,
					"tags":{},
					"private_instance_id":"private_instance_id",
					"options": {
						"loadbalancing": "round-robin",
						"allowed_sources": {
							"apps": ["nested-app-guid"],
							"any": true
						}
					}
				}`)
			})

			It("parses nested allowed_sources correctly", func() {
				endpoint, err := message.MakeEndpoint(false)
				Expect(err).NotTo(HaveOccurred())
				Expect(endpoint.AllowedSources).NotTo(BeNil())
				Expect(endpoint.AllowedSources.Apps).To(ConsistOf("nested-app-guid"))
				Expect(endpoint.AllowedSources.Any).To(BeTrue())
			})
		})

		Describe("With allowed_sources at both top-level and nested", func() {
			BeforeEach(func() {
				payload = []byte(`{
					"app":"app1",
					"uris":["test.com"],
					"host":"1.2.3.4",
					"port":1234,
					"tags":{},
					"private_instance_id":"private_instance_id",
					"allowed_sources": {
						"apps": ["top-level-app"]
					},
					"options": {
						"allowed_sources": {
							"apps": ["nested-app"]
						}
					}
				}`)
			})

			It("uses top-level allowed_sources (takes precedence)", func() {
				endpoint, err := message.MakeEndpoint(false)
				Expect(err).NotTo(HaveOccurred())
				Expect(endpoint.AllowedSources).NotTo(BeNil())
				Expect(endpoint.AllowedSources.Apps).To(ConsistOf("top-level-app"))
			})
		})

		Describe("With no allowed_sources", func() {
			BeforeEach(func() {
				payload = []byte(`{
					"app":"app1",
					"uris":["test.com"],
					"host":"1.2.3.4",
					"port":1234,
					"tags":{},
					"private_instance_id":"private_instance_id"
				}`)
			})

			It("returns nil for allowed_sources", func() {
				endpoint, err := message.MakeEndpoint(false)
				Expect(err).NotTo(HaveOccurred())
				Expect(endpoint.AllowedSources).To(BeNil())
			})
		})
	})
})
