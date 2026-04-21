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

	Describe("MakeEndpoint with route_policy_scope and route_policy_sources", func() {
		var message *RegistryMessage
		var payload []byte

		JustBeforeEach(func() {
			message = new(RegistryMessage)
			err := json.Unmarshal(payload, message)
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("With route_policy_scope=any and no route_policy_sources", func() {
			BeforeEach(func() {
				payload = []byte(`{
					"app":"app1",
					"uris":["test.com"],
					"host":"1.2.3.4",
					"port":1234,
					"tags":{},
					"private_instance_id":"private_instance_id",
					"options": {
						"route_policy_scope": "any"
					}
				}`)
			})

			It("parses route_policy_scope correctly with empty sources", func() {
				endpoint, err := message.MakeEndpoint(false, "round-robin")
				Expect(err).NotTo(HaveOccurred())
				Expect(endpoint.RoutePolicyScope).To(Equal("any"))
				Expect(endpoint.RoutePolicies).To(BeEmpty())
			})
		})

		Describe("With route_policy_scope=org and route_policy_sources listing apps and spaces", func() {
			BeforeEach(func() {
				payload = []byte(`{
					"app":"app1",
					"uris":["test.com"],
					"host":"1.2.3.4",
					"port":1234,
					"tags":{},
					"private_instance_id":"private_instance_id",
					"options": {
						"route_policy_scope": "org",
						"route_policy_sources": "cf:app:app-guid-1,cf:space:space-guid-1,cf:org:org-guid-1"
					}
				}`)
			})

			It("parses route_policy_scope and route_policy_sources correctly", func() {
				endpoint, err := message.MakeEndpoint(false, "round-robin")
				Expect(err).NotTo(HaveOccurred())
				Expect(endpoint.RoutePolicyScope).To(Equal("org"))
				Expect(endpoint.RoutePolicies).To(ConsistOf(
					"cf:app:app-guid-1",
					"cf:space:space-guid-1",
					"cf:org:org-guid-1",
				))
			})
		})

		Describe("With route_policy_scope=space and cf:any rule", func() {
			BeforeEach(func() {
				payload = []byte(`{
					"app":"app1",
					"uris":["test.com"],
					"host":"1.2.3.4",
					"port":1234,
					"tags":{},
					"private_instance_id":"private_instance_id",
					"options": {
						"route_policy_scope": "space",
						"route_policy_sources": "cf:any"
					}
				}`)
			})

			It("parses cf:any rule correctly", func() {
				endpoint, err := message.MakeEndpoint(false, "round-robin")
				Expect(err).NotTo(HaveOccurred())
				Expect(endpoint.RoutePolicyScope).To(Equal("space"))
				Expect(endpoint.RoutePolicies).To(ConsistOf("cf:any"))
			})
		})

		Describe("With no route_policy_scope or route_policy_sources", func() {
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

			It("leaves RoutePolicyScope empty and RoutePolicies nil", func() {
				endpoint, err := message.MakeEndpoint(false, "round-robin")
				Expect(err).NotTo(HaveOccurred())
				Expect(endpoint.RoutePolicyScope).To(BeEmpty())
				Expect(endpoint.RoutePolicies).To(BeEmpty())
			})
		})
	})
})
