package haproxy_test

import (
	"code.cloudfoundry.org/cf-tcp-router/config"
	"code.cloudfoundry.org/cf-tcp-router/configurer/haproxy"
	"code.cloudfoundry.org/cf-tcp-router/models"
	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagertest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("ConfigMarshaller", func() {
	Describe("Marshal", func() {
		var (
			haproxyConf    models.HAProxyConfig
			marshaller     haproxy.ConfigMarshaller
			logger         lager.Logger
			backendTlsCfg  config.BackendTLSConfig
			frontendTlsCfg []config.FrontendTLSConfig
		)

		BeforeEach(func() {
			logger = lagertest.NewTestLogger("config-marshaller-test")
			haproxyConf = models.HAProxyConfig{}
			marshaller = haproxy.NewConfigMarshaller(logger)
			backendTlsCfg = config.BackendTLSConfig{
				Enabled:           false,
				CACertificatePath: "/fake/path/to/ca.pem",
			}
			frontendTlsCfg = []config.FrontendTLSConfig{
				{
					Enabled:        false,
					CertificateDir: "/fake/path/to/certs/",
				},
			}
		})

		Context("when there is only a non-SNI route", func() {
			It("includes only the `default_backend` directive", func() {
				haproxyConf = models.HAProxyConfig{
					80: {
						"": {{Address: "default-host.internal", Port: 8080}},
					},
				}

				Expect(marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)).To(Equal(`
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

				Expect(marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)).To(Equal(`
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
				actual := marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)
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
				Expect(marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)).To(Equal(`
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

				Expect(marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)).To(Equal(`
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
				Expect(marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)).To(Equal(`
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

		Context("when backend_tls is enabled", func() {
			BeforeEach(func() {
				backendTlsCfg.Enabled = true
			})
			Context("when TLS port is specified", func() {
				It("configures the backend server to use the TLSPort", func() {
					haproxyConf = models.HAProxyConfig{
						80: {
							"": {
								{Address: "host-88.internal", Port: 8888, TLSPort: 8443, InstanceID: "host-88-instance-id"},
							},
						},
					}
					Expect(marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)).To(Equal(`
frontend frontend_80
  mode tcp
  bind :80
  default_backend backend_80

backend backend_80
  mode tcp
  server server_host-88.internal_8443 host-88.internal:8443 ssl verify required ca-file /fake/path/to/ca.pem verifyhost host-88-instance-id
`))
				})

				Context("when a client cert is provided", func() {
					BeforeEach(func() {
						backendTlsCfg.ClientCertAndKeyPath = "/fake/path/to/client_cert_and_key.pem"
					})
					It("configures the backend server to use the TLSPort with mTLS", func() {
						haproxyConf = models.HAProxyConfig{
							80: {
								"": {
									{Address: "host-88.internal", Port: 8888, TLSPort: 8443, InstanceID: "host-88-instance-id"},
								},
							},
						}
						Expect(marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)).To(Equal(`
frontend frontend_80
  mode tcp
  bind :80
  default_backend backend_80

backend backend_80
  mode tcp
  server server_host-88.internal_8443 host-88.internal:8443 ssl verify required ca-file /fake/path/to/ca.pem crt /fake/path/to/client_cert_and_key.pem verifyhost host-88-instance-id
`))

					})

					Context("when frontend TLS is not terminated (pre-existing behavior)", func() {
						Context("when EnableBackendMTLS is false (default/unset)", func() {
							It("still presents the client cert, preserving behavior that predates EnableBackendMTLS", func() {
								haproxyConf = models.HAProxyConfig{
									80: {
										"": {
											{Address: "host-88.internal", Port: 8888, TLSPort: 8443, InstanceID: "host-88-instance-id"},
										},
									},
								}
								Expect(marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)).To(Equal(`
frontend frontend_80
  mode tcp
  bind :80
  default_backend backend_80

backend backend_80
  mode tcp
  server server_host-88.internal_8443 host-88.internal:8443 ssl verify required ca-file /fake/path/to/ca.pem crt /fake/path/to/client_cert_and_key.pem verifyhost host-88-instance-id
`))
							})
						})
					})

					Context("when frontend TLS is terminated", func() {
						BeforeEach(func() {
							frontendTlsCfg[0].Enabled = true
						})

						Context("when EnableBackendMTLS is false (default)", func() {
							It("does not present the client cert", func() {
								haproxyConf = models.HAProxyConfig{
									80: {
										"": {
											{Address: "host-88.internal", Port: 8888, TLSPort: 8443, InstanceID: "host-88-instance-id", TerminateFrontendTLS: true},
										},
									},
								}
								Expect(marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)).To(Equal(`
frontend frontend_80
  mode tcp
  bind :80 ssl crt /fake/path/to/certs/
  default_backend backend_80

backend backend_80
  mode tcp
  server server_host-88.internal_8443 host-88.internal:8443 ssl verify required ca-file /fake/path/to/ca.pem verifyhost host-88-instance-id
`))
							})
						})

						Context("when EnableBackendMTLS is true", func() {
							It("presents the client cert", func() {
								haproxyConf = models.HAProxyConfig{
									80: {
										"": {
											{Address: "host-88.internal", Port: 8888, TLSPort: 8443, InstanceID: "host-88-instance-id", TerminateFrontendTLS: true, EnableBackendMTLS: true},
										},
									},
								}
								Expect(marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)).To(Equal(`
frontend frontend_80
  mode tcp
  bind :80 ssl crt /fake/path/to/certs/
  default_backend backend_80

backend backend_80
  mode tcp
  server server_host-88.internal_8443 host-88.internal:8443 ssl verify required ca-file /fake/path/to/ca.pem crt /fake/path/to/client_cert_and_key.pem verifyhost host-88-instance-id
`))
							})
						})
					})
				})

				Context("when SniRewriteHostname is provided", func() {
					It("does not configure the backend server with sni str directive when frontend TLS is disabled", func() {
						haproxyConf = models.HAProxyConfig{
							80: {
								"": {
									{Address: "host-88.internal", Port: 8888, TLSPort: 8443, InstanceID: "host-88-instance-id", SniRewriteHostname: "rewrite.example.com"},
								},
							},
						}
						Expect(marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)).To(Equal(`
frontend frontend_80
  mode tcp
  bind :80
  default_backend backend_80

backend backend_80
  mode tcp
  server server_host-88.internal_8443 host-88.internal:8443 ssl verify required ca-file /fake/path/to/ca.pem verifyhost host-88-instance-id
`))
					})

					It("configures the backend server with sni str directive when frontend TLS is enabled", func() {
						frontendTlsCfg[0].Enabled = true
						haproxyConf = models.HAProxyConfig{
							80: {
								"": {
									{Address: "host-88.internal", Port: 8888, TLSPort: 8443, InstanceID: "host-88-instance-id", SniRewriteHostname: "rewrite.example.com", TerminateFrontendTLS: true},
								},
							},
						}
						Expect(marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)).To(Equal(`
frontend frontend_80
  mode tcp
  bind :80 ssl crt /fake/path/to/certs/
  default_backend backend_80

backend backend_80
  mode tcp
  server server_host-88.internal_8443 host-88.internal:8443 ssl verify required ca-file /fake/path/to/ca.pem verifyhost host-88-instance-id sni str(rewrite.example.com)
`))
					})
				})
			})
			Context("when TLSPort is 0", func() {
				It("Logs an error indicating that the backend is not being encrypted", func() {
					haproxyConf = models.HAProxyConfig{
						80: {
							"": {
								{Address: "host-88.internal", Port: 8888, TLSPort: 0, InstanceID: "host-88-instance-id"},
							},
						},
					}
					marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)
					Expect(logger).To(gbytes.Say("route-missing-tls-information"))
				})
				It("uses the non-tls backend port", func() {
					haproxyConf = models.HAProxyConfig{
						80: {
							"": {
								{Address: "host-88.internal", Port: 8888, TLSPort: 0, InstanceID: "host-88-instance-id"},
							},
						},
					}
					Expect(marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)).To(Equal(`
frontend frontend_80
  mode tcp
  bind :80
  default_backend backend_80

backend backend_80
  mode tcp
  server server_host-88.internal_8888 host-88.internal:8888
`))
				})
			})
			Context("when TLSPort is -1", func() {
				It("does not log an error", func() {
					haproxyConf = models.HAProxyConfig{
						80: {
							"": {
								{Address: "host-88.internal", Port: 8888, TLSPort: -1, InstanceID: "host-88-instance-id"},
							},
						},
					}
					marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)
					Expect(logger).NotTo(gbytes.Say("route-missing-tls-information"))
				})
				It("uses the non-tls backend port", func() {
					haproxyConf = models.HAProxyConfig{
						80: {
							"": {
								{Address: "host-88.internal", Port: 8888, TLSPort: -1, InstanceID: "host-88-instance-id"},
							},
						},
					}
					Expect(marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)).To(Equal(`
frontend frontend_80
  mode tcp
  bind :80
  default_backend backend_80

backend backend_80
  mode tcp
  server server_host-88.internal_8888 host-88.internal:8888
`))

				})
			})
		})

		Context("when backend_tls is disabled", func() {
			Context("when a TLSPort is provided", func() {
				It("logs an error", func() {
					haproxyConf = models.HAProxyConfig{
						80: {
							"": {
								{Address: "host-88.internal", Port: 8888, TLSPort: 8443, InstanceID: "host-88-instance-id"},
							},
						},
					}
					Expect(marshaller.Marshal(haproxyConf, config.BackendTLSConfig{Enabled: false}, frontendTlsCfg)).To(Equal(`
frontend frontend_80
  mode tcp
  bind :80
  default_backend backend_80

backend backend_80
  mode tcp
`))
					Expect(logger).To(gbytes.Say("Backend TLS Port was set, but backend_tls has not been enabled for tcp-router"))
				})
			})
		})

		Context("when frontend_tls is disabled", func() {
			It("and backend_tls is disabled", func() {
				haproxyConf = models.HAProxyConfig{
					1025: {
						"": {
							{Address: "88.0.0.36", Port: 8888, InstanceID: "host-88-instance-id"},
							{Address: "88.0.0.37", Port: 8889, InstanceID: "host-89-instance-id"},
						},
					},
				}
				actual := marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)

				Expect(actual).To(Equal(`
frontend frontend_1025
  mode tcp
  bind :1025
  default_backend backend_1025

backend backend_1025
  mode tcp
  server server_88.0.0.36_8888 88.0.0.36:8888
  server server_88.0.0.37_8889 88.0.0.37:8889
`))
			})
		})

		Context("when frontend_tls is enabled", func() {
			BeforeEach(func() {
				frontendTlsCfg[0].Enabled = true
			})

			It("and backend_tls is disabled", func() {
				haproxyConf = models.HAProxyConfig{
					1025: {
						"rmq1.sys.tas.foobar.com": {
							{Address: "88.0.0.36", Port: 8888, TLSPort: 0, InstanceID: "host-88-instance-id", TerminateFrontendTLS: true},
						},
						"rmq2.sys.tas.foobar.com": {
							{Address: "88.0.0.37", Port: 8889, TLSPort: 0, InstanceID: "host-89-instance-id", TerminateFrontendTLS: true},
							{Address: "88.0.0.38", Port: 8890, TLSPort: 0, InstanceID: "host-90-instance-id", TerminateFrontendTLS: true},
						},
					},
				}
				actual := marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)

				Expect(actual).To(Equal(`
frontend frontend_1025
  mode tcp
  bind :1025 ssl crt /fake/path/to/certs/
  tcp-request inspect-delay 5s
  tcp-request content accept if { req.ssl_hello_type gt 0 }
  use_backend backend_1025_rmq1.sys.tas.foobar.com if { ssl_fc_sni rmq1.sys.tas.foobar.com }
  use_backend backend_1025_rmq2.sys.tas.foobar.com if { ssl_fc_sni rmq2.sys.tas.foobar.com }

backend backend_1025_rmq1.sys.tas.foobar.com
  mode tcp
  server server_88.0.0.36_8888 88.0.0.36:8888

backend backend_1025_rmq2.sys.tas.foobar.com
  mode tcp
  server server_88.0.0.37_8889 88.0.0.37:8889
  server server_88.0.0.38_8890 88.0.0.38:8890
`))
			})

			It("and backend_tls is enabled with instance ids", func() {
				backendTlsCfg.Enabled = true

				haproxyConf = models.HAProxyConfig{
					1025: {
						"rmq1.sys.tas.foobar.com": {
							{Address: "88.0.0.36", TLSPort: 8888, InstanceID: "host-88-instance-id", TerminateFrontendTLS: true},
						},
						"rmq2.sys.tas.foobar.com": {
							{Address: "88.0.0.37", TLSPort: 8889, InstanceID: "host-89-instance-id", TerminateFrontendTLS: true},
							{Address: "88.0.0.38", TLSPort: 8890, InstanceID: "host-90-instance-id", TerminateFrontendTLS: true},
						},
					},
				}
				actual := marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)

				Expect(actual).To(Equal(`
frontend frontend_1025
  mode tcp
  bind :1025 ssl crt /fake/path/to/certs/
  tcp-request inspect-delay 5s
  tcp-request content accept if { req.ssl_hello_type gt 0 }
  use_backend backend_1025_rmq1.sys.tas.foobar.com if { ssl_fc_sni rmq1.sys.tas.foobar.com }
  use_backend backend_1025_rmq2.sys.tas.foobar.com if { ssl_fc_sni rmq2.sys.tas.foobar.com }

backend backend_1025_rmq1.sys.tas.foobar.com
  mode tcp
  server server_88.0.0.36_8888 88.0.0.36:8888 ssl verify required ca-file /fake/path/to/ca.pem verifyhost host-88-instance-id

backend backend_1025_rmq2.sys.tas.foobar.com
  mode tcp
  server server_88.0.0.37_8889 88.0.0.37:8889 ssl verify required ca-file /fake/path/to/ca.pem verifyhost host-89-instance-id
  server server_88.0.0.38_8890 88.0.0.38:8890 ssl verify required ca-file /fake/path/to/ca.pem verifyhost host-90-instance-id
`))
			})

			It("and backend_tls is enabled without instance ids", func() {
				backendTlsCfg.Enabled = true

				haproxyConf = models.HAProxyConfig{
					1025: {
						"rmq1.sys.tas.foobar.com": {
							{Address: "88.0.0.36", TLSPort: 8888, TerminateFrontendTLS: true},
						},
						"rmq2.sys.tas.foobar.com": {
							{Address: "88.0.0.37", TLSPort: 8889, TerminateFrontendTLS: true},
							{Address: "88.0.0.38", TLSPort: 8890, TerminateFrontendTLS: true},
						},
					},
				}
				actual := marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)

				Expect(actual).To(Equal(`
frontend frontend_1025
  mode tcp
  bind :1025 ssl crt /fake/path/to/certs/
  tcp-request inspect-delay 5s
  tcp-request content accept if { req.ssl_hello_type gt 0 }
  use_backend backend_1025_rmq1.sys.tas.foobar.com if { ssl_fc_sni rmq1.sys.tas.foobar.com }
  use_backend backend_1025_rmq2.sys.tas.foobar.com if { ssl_fc_sni rmq2.sys.tas.foobar.com }

backend backend_1025_rmq1.sys.tas.foobar.com
  mode tcp
  server server_88.0.0.36_8888 88.0.0.36:8888 ssl verify required ca-file /fake/path/to/ca.pem

backend backend_1025_rmq2.sys.tas.foobar.com
  mode tcp
  server server_88.0.0.37_8889 88.0.0.37:8889 ssl verify required ca-file /fake/path/to/ca.pem
  server server_88.0.0.38_8890 88.0.0.38:8890 ssl verify required ca-file /fake/path/to/ca.pem
`))
			})

			It("and backend_tls is disabled and ALPNs are defined", func() {
				haproxyConf = models.HAProxyConfig{
					1025: {
						"rmq1.sys.tas.foobar.com": {
							{Address: "88.0.0.36", Port: 8888, TLSPort: 0, InstanceID: "host-88-instance-id", TerminateFrontendTLS: true, ALPNs: "alpn1,alpn2"},
						},
						"rmq2.sys.tas.foobar.com": {
							{Address: "88.0.0.37", Port: 8889, TLSPort: 0, InstanceID: "host-89-instance-id", TerminateFrontendTLS: true, ALPNs: "alpn1,alpn3"},
							{Address: "88.0.0.38", Port: 8890, TLSPort: 0, InstanceID: "host-90-instance-id", TerminateFrontendTLS: true, ALPNs: "alpn4,alpn1"},
						},
					},
				}
				actual := marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)

				Expect(actual).To(Equal(`
frontend frontend_1025
  mode tcp
  bind :1025 ssl crt /fake/path/to/certs/ alpn alpn1,alpn2,alpn3,alpn4
  tcp-request inspect-delay 5s
  tcp-request content accept if { req.ssl_hello_type gt 0 }
  use_backend backend_1025_rmq1.sys.tas.foobar.com if { ssl_fc_sni rmq1.sys.tas.foobar.com }
  use_backend backend_1025_rmq2.sys.tas.foobar.com if { ssl_fc_sni rmq2.sys.tas.foobar.com }

backend backend_1025_rmq1.sys.tas.foobar.com
  mode tcp
  server server_88.0.0.36_8888 88.0.0.36:8888

backend backend_1025_rmq2.sys.tas.foobar.com
  mode tcp
  server server_88.0.0.37_8889 88.0.0.37:8889
  server server_88.0.0.38_8890 88.0.0.38:8890
`))
			})

			It("and backend_tls is enabled and ALPNs are defined", func() {
				backendTlsCfg.Enabled = true
				backendTlsCfg.ClientCertAndKeyPath = "/fake/path/to/client_cert_and_key.pem"

				haproxyConf = models.HAProxyConfig{
					1025: {
						"rmq1.sys.tas.foobar.com": {
							{Address: "88.0.0.36", TLSPort: 8888, InstanceID: "host-88-instance-id", TerminateFrontendTLS: true, EnableBackendMTLS: true, ALPNs: "alpn1,alpn2"},
						},
						"rmq2.sys.tas.foobar.com": {
							{Address: "88.0.0.37", TLSPort: 8889, InstanceID: "host-89-instance-id", TerminateFrontendTLS: true, EnableBackendMTLS: true, ALPNs: "alpn1,alpn3"},
							{Address: "88.0.0.38", TLSPort: 8890, InstanceID: "host-90-instance-id", TerminateFrontendTLS: true, EnableBackendMTLS: true, ALPNs: "alpn4,alpn1"},
						},
					},
				}
				actual := marshaller.Marshal(haproxyConf, backendTlsCfg, frontendTlsCfg)

				Expect(actual).To(Equal(`
frontend frontend_1025
  mode tcp
  bind :1025 ssl crt /fake/path/to/certs/ alpn alpn1,alpn2,alpn3,alpn4
  tcp-request inspect-delay 5s
  tcp-request content accept if { req.ssl_hello_type gt 0 }
  use_backend backend_1025_rmq1.sys.tas.foobar.com if { ssl_fc_sni rmq1.sys.tas.foobar.com }
  use_backend backend_1025_rmq2.sys.tas.foobar.com if { ssl_fc_sni rmq2.sys.tas.foobar.com }

backend backend_1025_rmq1.sys.tas.foobar.com
  mode tcp
  server server_88.0.0.36_8888 88.0.0.36:8888 ssl verify required ca-file /fake/path/to/ca.pem crt /fake/path/to/client_cert_and_key.pem alpn alpn1,alpn2 verifyhost host-88-instance-id

backend backend_1025_rmq2.sys.tas.foobar.com
  mode tcp
  server server_88.0.0.37_8889 88.0.0.37:8889 ssl verify required ca-file /fake/path/to/ca.pem crt /fake/path/to/client_cert_and_key.pem alpn alpn1,alpn3 verifyhost host-89-instance-id
  server server_88.0.0.38_8890 88.0.0.38:8890 ssl verify required ca-file /fake/path/to/ca.pem crt /fake/path/to/client_cert_and_key.pem alpn alpn4,alpn1 verifyhost host-90-instance-id
`))
			})
		})
	})
})
