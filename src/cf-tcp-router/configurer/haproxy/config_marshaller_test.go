package haproxy_test

import (
	"code.cloudfoundry.org/routing-release/cf-tcp-router/configurer/haproxy"
	"code.cloudfoundry.org/routing-release/cf-tcp-router/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ConfigMarshaller", func() {
	Describe("Marshal", func() {
		var (
			haproxyConf models.HAProxyConfig
			marshaller  haproxy.ConfigMarshaller
		)

		BeforeEach(func() {
			haproxyConf = models.HAProxyConfig{}
			marshaller = haproxy.NewConfigMarshaller()
		})

		Context("when there is only a non-SNI route", func() {
			It("includes only the `default_backend` directive", func() {
				haproxyConf = models.HAProxyConfig{
					80: {
						"": {{Address: "default-host.internal", Port: 8080}},
					},
				}

				Expect(marshaller.Marshal(haproxyConf)).To(Equal(`
frontend frontend_80
  mode tcp
  bind :80
  default_backend backend_80

backend backend_80
  mode tcp
  server server_default-host.internal_8080 default-host.internal:8080
`))
			})
		})

		Context("when there is only an SNI route", func() {
			It("includes only the SNI `use_backend` directive", func() {
				haproxyConf = models.HAProxyConfig{
					80: {
						"external-host.example.com": {{Address: "default-host.internal", Port: 8080}},
					},
				}

				Expect(marshaller.Marshal(haproxyConf)).To(Equal(`
frontend frontend_80
  mode tcp
  bind :80
  tcp-request inspect-delay 5s
  tcp-request content accept if { req.ssl_hello_type gt 0 }
  use_backend backend_80_external-host.example.com if { req.ssl_sni external-host.example.com }

backend backend_80_external-host.example.com
  mode tcp
  server server_default-host.internal_8080 default-host.internal:8080
`))
			})
		})

		Context("when there is both an SNI route and a non-SNI route", func() {
			It("includes both types of directives", func() {
				haproxyConf = models.HAProxyConfig{
					80: {
						"":                          {{Address: "default-host.internal", Port: 8080}},
						"external-host.example.com": {{Address: "sni-host.internal", Port: 9090}},
					},
				}
				actual := marshaller.Marshal(haproxyConf)
				Expect(actual).To(Equal(`
frontend frontend_80
  mode tcp
  bind :80
  tcp-request inspect-delay 5s
  tcp-request content accept if { req.ssl_hello_type gt 0 }
  default_backend backend_80
  use_backend backend_80_external-host.example.com if { req.ssl_sni external-host.example.com }

backend backend_80
  mode tcp
  server server_default-host.internal_8080 default-host.internal:8080

backend backend_80_external-host.example.com
  mode tcp
  server server_sni-host.internal_9090 sni-host.internal:9090
`))
			})
		})

		Context("when there are multiple inbound ports", func() {
			It("sorts the inbound ports", func() {
				haproxyConf = models.HAProxyConfig{
					90: {
						"": {{Address: "host-90.internal", Port: 9090}},
					},
					70: {
						"": {{Address: "host-70.internal", Port: 7070}},
					},
					80: {
						"": {{Address: "host-80.internal", Port: 8080}},
					},
				}
				Expect(marshaller.Marshal(haproxyConf)).To(Equal(`
frontend frontend_70
  mode tcp
  bind :70
  default_backend backend_70

backend backend_70
  mode tcp
  server server_host-70.internal_7070 host-70.internal:7070

frontend frontend_80
  mode tcp
  bind :80
  default_backend backend_80

backend backend_80
  mode tcp
  server server_host-80.internal_8080 host-80.internal:8080

frontend frontend_90
  mode tcp
  bind :90
  default_backend backend_90

backend backend_90
  mode tcp
  server server_host-90.internal_9090 host-90.internal:9090
`))
			})
		})

		Context("when there are multiple SNI hostnames for an inbound port", func() {
			It("sorts the SNI hostnames", func() {
				haproxyConf = models.HAProxyConfig{
					80: {
						"host-99.example.com": {{Address: "host-99.internal", Port: 9999}},
						"":                    {{Address: "default-host.internal", Port: 8080}},
						"host-1.example.com":  {{Address: "host-1.internal", Port: 1111}},
					},
				}

				Expect(marshaller.Marshal(haproxyConf)).To(Equal(`
frontend frontend_80
  mode tcp
  bind :80
  tcp-request inspect-delay 5s
  tcp-request content accept if { req.ssl_hello_type gt 0 }
  default_backend backend_80
  use_backend backend_80_host-1.example.com if { req.ssl_sni host-1.example.com }
  use_backend backend_80_host-99.example.com if { req.ssl_sni host-99.example.com }

backend backend_80
  mode tcp
  server server_default-host.internal_8080 default-host.internal:8080

backend backend_80_host-1.example.com
  mode tcp
  server server_host-1.internal_1111 host-1.internal:1111

backend backend_80_host-99.example.com
  mode tcp
  server server_host-99.internal_9999 host-99.internal:9999
`))
			})
		})

		Context("when there are multiple servers for a backend", func() {
			It("retains the original order of the servers", func() {
				haproxyConf = models.HAProxyConfig{
					80: {
						"": {
							{Address: "host-88.internal", Port: 8888},
							{Address: "host-99.internal", Port: 9999},
							{Address: "host-77.internal", Port: 7777},
						},
					},
				}
				Expect(marshaller.Marshal(haproxyConf)).To(Equal(`
frontend frontend_80
  mode tcp
  bind :80
  default_backend backend_80

backend backend_80
  mode tcp
  server server_host-88.internal_8888 host-88.internal:8888
  server server_host-99.internal_9999 host-99.internal:9999
  server server_host-77.internal_7777 host-77.internal:7777
`))
			})
		})
	})
})
