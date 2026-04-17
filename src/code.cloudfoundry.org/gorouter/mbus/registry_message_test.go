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

	Describe("MakeEndpoint with access_scope and access_rules", func() {
		var message *RegistryMessage
		var payload []byte

		JustBeforeEach(func() {
			message = new(RegistryMessage)
			err := json.Unmarshal(payload, message)
			Expect(err).NotTo(HaveOccurred())
		})

		Describe("With access_scope=any and no access_rules", func() {
			BeforeEach(func() {
				payload = []byte(`{
					"app":"app1",
					"uris":["test.com"],
					"host":"1.2.3.4",
					"port":1234,
					"tags":{},
					"private_instance_id":"private_instance_id",
					"options": {
						"access_scope": "any"
					}
				}`)
			})

			It("parses access_scope correctly with empty rules", func() {
				endpoint, err := message.MakeEndpoint(false, "round-robin")
				Expect(err).NotTo(HaveOccurred())
				Expect(endpoint.AccessScope).To(Equal("any"))
				Expect(endpoint.AccessRules).To(BeEmpty())
			})
		})

		Describe("With access_scope=org and access_rules listing apps and spaces", func() {
			BeforeEach(func() {
				payload = []byte(`{
					"app":"app1",
					"uris":["test.com"],
					"host":"1.2.3.4",
					"port":1234,
					"tags":{},
					"private_instance_id":"private_instance_id",
					"options": {
						"access_scope": "org",
						"access_rules": "cf:app:app-guid-1,cf:space:space-guid-1,cf:org:org-guid-1"
					}
				}`)
			})

			It("parses access_scope and access_rules correctly", func() {
				endpoint, err := message.MakeEndpoint(false, "round-robin")
				Expect(err).NotTo(HaveOccurred())
				Expect(endpoint.AccessScope).To(Equal("org"))
				Expect(endpoint.AccessRules).To(ConsistOf(
					"cf:app:app-guid-1",
					"cf:space:space-guid-1",
					"cf:org:org-guid-1",
				))
			})
		})

		Describe("With access_scope=space and cf:any rule", func() {
			BeforeEach(func() {
				payload = []byte(`{
					"app":"app1",
					"uris":["test.com"],
					"host":"1.2.3.4",
					"port":1234,
					"tags":{},
					"private_instance_id":"private_instance_id",
					"options": {
						"access_scope": "space",
						"access_rules": "cf:any"
					}
				}`)
			})

			It("parses cf:any rule correctly", func() {
				endpoint, err := message.MakeEndpoint(false, "round-robin")
				Expect(err).NotTo(HaveOccurred())
				Expect(endpoint.AccessScope).To(Equal("space"))
				Expect(endpoint.AccessRules).To(ConsistOf("cf:any"))
			})
		})

		Describe("With no access_scope or access_rules", func() {
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

			It("leaves AccessScope empty and AccessRules nil", func() {
				endpoint, err := message.MakeEndpoint(false, "round-robin")
				Expect(err).NotTo(HaveOccurred())
				Expect(endpoint.AccessScope).To(BeEmpty())
				Expect(endpoint.AccessRules).To(BeEmpty())
			})
		})
	})
})
