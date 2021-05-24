package config_test

import (
	"encoding/json"
	"errors"
	"time"

	"code.cloudfoundry.org/locket"
	"code.cloudfoundry.org/routing-release/routing-api/config"
	"code.cloudfoundry.org/routing-release/routing-api/models"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {
	Describe("NewConfigFromFile", func() {
		Context("when auth is enabled", func() {
			Context("when the file exists", func() {
				It("returns a valid Config struct", func() {
					cfg_file := "../example_config/example.yml"
					cfg, err := config.NewConfigFromFile(cfg_file, false)

					Expect(err).NotTo(HaveOccurred())
					Expect(cfg.API.ListenPort).To(Equal(3000))
					Expect(cfg.AdminPort).To(Equal(9999))
					Expect(cfg.LogGuid).To(Equal("my_logs"))
					Expect(cfg.MetronConfig.Address).To(Equal("1.2.3.4"))
					Expect(cfg.MetronConfig.Port).To(Equal("4567"))
					Expect(cfg.StatsdClientFlushInterval).To(Equal(10 * time.Millisecond))
					Expect(cfg.OAuth.TokenEndpoint).To(Equal("127.0.0.1"))
					Expect(cfg.OAuth.Port).To(Equal(8080))
					Expect(cfg.OAuth.CACerts).To(Equal("some-ca-cert"))
					Expect(cfg.OAuth.SkipSSLValidation).To(Equal(true))
					Expect(cfg.SystemDomain).To(Equal("example.com"))
					Expect(cfg.SqlDB.Username).To(Equal("username"))
					Expect(cfg.SqlDB.Password).To(Equal("password"))
					Expect(cfg.SqlDB.Port).To(Equal(1234))
					Expect(cfg.SqlDB.CACert).To(Equal("some CA cert"))
					Expect(cfg.SqlDB.SkipSSLValidation).To(Equal(true))
					Expect(cfg.SqlDB.SkipHostnameValidation).To(Equal(true))
					Expect(cfg.SqlDB.MaxIdleConns).To(Equal(2))
					Expect(cfg.SqlDB.MaxOpenConns).To(Equal(5))
					Expect(cfg.SqlDB.ConnMaxLifetime).To(Equal(1200))
					Expect(cfg.MaxTTL).To(Equal(2 * time.Minute))
					Expect(cfg.LockResouceKey).To(Equal("my-key"))
					Expect(cfg.LockTTL).To(Equal(10 * time.Second))
					Expect(cfg.RetryInterval).To(Equal(5 * time.Second))
					Expect(cfg.Locket.LocketAddress).To(Equal("http://localhost:5678"))
					Expect(cfg.Locket.LocketCACertFile).To(Equal("some-locket-ca-cert"))
					Expect(cfg.Locket.LocketClientCertFile).To(Equal("some-locket-client-cert"))
					Expect(cfg.Locket.LocketClientKeyFile).To(Equal("some-locket-client-key"))
					Expect(cfg.API.HTTPEnabled).To(Equal(false))
					Expect(cfg.API.MTLSListenPort).To(Equal(3001))
					Expect(cfg.API.MTLSClientCAPath).To(Equal("client ca file path"))
					Expect(cfg.API.MTLSServerCertPath).To(Equal("server cert file path"))
					Expect(cfg.API.MTLSServerKeyPath).To(Equal("server key file path"))
					Expect(cfg.ReservedSystemComponentPorts).To(Equal([]int{5555, 6666}))
				})

				Context("when there is no token endpoint specified", func() {
					It("returns an error", func() {
						cfg_file := "../example_config/missing_uaa_url.yml"
						_, err := config.NewConfigFromFile(cfg_file, false)
						Expect(err).To(HaveOccurred())
					})
				})
			})

			Context("when the file does not exists", func() {
				It("returns an error", func() {
					cfg_file := "notexist"
					_, err := config.NewConfigFromFile(cfg_file, false)

					Expect(err).To(HaveOccurred())
				})
			})
		})

		Context("when auth is disabled", func() {
			Context("when the file exists", func() {
				It("returns a valid config", func() {
					cfg_file := "../example_config/example.yml"
					cfg, err := config.NewConfigFromFile(cfg_file, true)

					Expect(err).NotTo(HaveOccurred())
					Expect(cfg.LogGuid).To(Equal("my_logs"))
					Expect(cfg.MetronConfig.Address).To(Equal("1.2.3.4"))
					Expect(cfg.MetronConfig.Port).To(Equal("4567"))
					Expect(cfg.StatsdClientFlushInterval).To(Equal(10 * time.Millisecond))
					Expect(cfg.OAuth.TokenEndpoint).To(Equal("127.0.0.1"))
					Expect(cfg.OAuth.Port).To(Equal(8080))
					Expect(cfg.OAuth.CACerts).To(Equal("some-ca-cert"))
				})

				Context("when there is no token endpoint url", func() {
					It("returns a valid config", func() {
						cfg_file := "../example_config/missing_uaa_url.yml"
						cfg, err := config.NewConfigFromFile(cfg_file, true)

						Expect(err).NotTo(HaveOccurred())
						Expect(cfg.LogGuid).To(Equal("my_logs"))
						Expect(cfg.MetronConfig.Address).To(Equal("1.2.3.4"))
						Expect(cfg.MetronConfig.Port).To(Equal("4567"))
						Expect(cfg.DebugAddress).To(Equal("1.2.3.4:1234"))
						Expect(cfg.MaxTTL).To(Equal(2 * time.Minute))
						Expect(cfg.StatsdClientFlushInterval).To(Equal(10 * time.Millisecond))
						Expect(cfg.OAuth.TokenEndpoint).To(BeEmpty())
						Expect(cfg.OAuth.Port).To(Equal(0))
					})
				})
			})
		})
	})

	Describe("NewConfigFromBytes", func() {
		var (
			validHash  map[string]interface{}
			testConfig []byte
		)

		BeforeEach(func() {
			validHash = map[string]interface{}{
				"system_domain": "example.com",
				"log_guid":      "my_logs",
				"uuid":          "fake-uuid",
				"admin_port":    9999,
				"api": map[string]interface{}{
					"listen_port":           3000,
					"mtls_listen_port":      3001,
					"mtls_client_ca_file":   "client ca file path",
					"mtls_server_key_file":  "server key file path",
					"mtls_server_cert_file": "server cert file path",
				},
				"metrics_reporting_interval":   "500ms",
				"statsd_client_flush_interval": "10ms",
				"locket": map[string]interface{}{
					"locket_address": "127.0.0.1:5678",
				},
			}
		})

		JustBeforeEach(func() {
			var err error
			testConfig, err = json.Marshal(validHash)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when UUID property is set", func() {
			It("populates the value", func() {
				cfg, err := config.NewConfigFromBytes(testConfig, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.UUID).To(Equal("fake-uuid"))
			})
		})

		Context("when api http enabled is set true", func() {
			BeforeEach(func() {
				validHash["api"].(map[string]interface{})["http_enabled"] = true
			})
			It("parses http_enabled", func() {
				cfg, err := config.NewConfigFromBytes(testConfig, true)
				Expect(err).ToNot(HaveOccurred())
				Expect(cfg.API.HTTPEnabled).To(BeTrue())
			})
			Context("when the api listen port is invalid", func() {
				Context("when it is too high", func() {
					BeforeEach(func() {
						validHash["api"].(map[string]interface{})["listen_port"] = 65536
					})
					It("returns an error", func() {
						_, err := config.NewConfigFromBytes(testConfig, true)
						Expect(err).To(HaveOccurred())
					})
				})

				Context("when it is too low", func() {
					BeforeEach(func() {
						validHash["api"].(map[string]interface{})["listen_port"] = 0
					})
					It("returns an error", func() {
						_, err := config.NewConfigFromBytes(testConfig, true)
						Expect(err).To(HaveOccurred())
					})
				})
			})
		})

		Context("when api mtls port is too high", func() {
			BeforeEach(func() {
				validHash["api"].(map[string]interface{})["mtls_listen_port"] = 928334
			})

			It("does not validate", func() {
				_, err := config.NewConfigFromBytes(testConfig, true)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when system domain is not set", func() {
			BeforeEach(func() {
				delete(validHash, "system_domain")
			})
			It("returns an error", func() {
				_, err := config.NewConfigFromBytes(testConfig, true)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("system_domain"))
			})
		})

		Context("when UUID property is not set", func() {
			BeforeEach(func() {
				delete(validHash, "uuid")
			})
			It("populates the value", func() {
				_, err := config.NewConfigFromBytes(testConfig, true)
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(errors.New("No UUID is specified")))
			})
		})

		Context("when AdminPort property is set", func() {
			It("populates the value", func() {
				cfg, err := config.NewConfigFromBytes(testConfig, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.AdminPort).To(Equal(9999))
			})
		})

		Context("when AdminPort property is not set", func() {
			BeforeEach(func() {
				delete(validHash, "admin_port")
			})
			It("returns an error", func() {
				_, err := config.NewConfigFromBytes(testConfig, true)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when Locket Address property is not set", func() {
			BeforeEach(func() {
				delete(validHash["locket"].(map[string]interface{}), "locket_address")
			})

			It("returns an error", func() {
				_, err := config.NewConfigFromBytes(testConfig, true)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when lock properties are not set", func() {
			It("populates the default value for LockResourceKey", func() {
				cfg, err := config.NewConfigFromBytes(testConfig, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.LockResouceKey).To(Equal(config.DefaultLockResourceKey))
			})

			It("populates the default value for LockTTL from locket library", func() {
				cfg, err := config.NewConfigFromBytes(testConfig, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.LockTTL).To(Equal(locket.DefaultSessionTTL))
			})

			It("populates the default value for RetryInterval from locket library", func() {
				cfg, err := config.NewConfigFromBytes(testConfig, true)
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.RetryInterval).To(Equal(locket.RetryInterval))
			})
		})

		Context("when multiple router groups are seeded with different names", func() {
			BeforeEach(func() {
				validHash["router_groups"] = []interface{}{
					map[string]interface{}{
						"name":             "router-group-1",
						"reservable_ports": 12000,
						"type":             "tcp",
					},
					map[string]interface{}{
						"name":             "router-group-2",
						"reservable_ports": "1024-10000,42000",
						"type":             "udp",
					},
					map[string]interface{}{
						"name":             "router-group-special",
						"reservable_ports": "1122,1123",
						"type":             "tcp",
					},
				}
			})
			It("should not error", func() {
				cfg, err := config.NewConfigFromBytes(testConfig, true)
				Expect(err).NotTo(HaveOccurred())
				expectedGroups := models.RouterGroups{
					{
						Name:            "router-group-1",
						ReservablePorts: "12000",
						Type:            "tcp",
					},
					{
						Name:            "router-group-2",
						ReservablePorts: "1024-10000,42000",
						Type:            "udp",
					},
					{
						Name:            "router-group-special",
						ReservablePorts: "1122,1123",
						Type:            "tcp",
					},
				}
				Expect(cfg.RouterGroups).To(Equal(expectedGroups))
			})
		})

		Context("when router groups are seeded in the configuration file", func() {
			addRouterGroupWithPorts := func(ports string) {
				validHash["router_groups"] = []interface{}{
					map[string]interface{}{
						"name":             "router-group-1",
						"reservable_ports": ports,
						"type":             "tcp",
					},
				}
			}

			Context("when the router group port has an invalid type", func() {
				BeforeEach(func() {
					validHash["router_groups"] = []interface{}{
						map[string]interface{}{
							"name": "router-group-1",
							"reservable_ports": []interface{}{
								"1122",
								1123,
							},
							"type": "tcp",
						},
					}
				})
				It("returns an error when port array has invalid type", func() {
					_, err := config.NewConfigFromBytes(testConfig, true)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("invalid type for reservable port"))
				})
			})

			Context("when there are alphabetic ports", func() {
				BeforeEach(func() {
					addRouterGroupWithPorts("abc")
				})

				It("returns error", func() {
					_, err := config.NewConfigFromBytes(testConfig, true)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Port must be between 1024 and 65535"))
				})
			})

			Context("when the port is prefixed with zero", func() {
				BeforeEach(func() {
					addRouterGroupWithPorts("00003202-4000")
				})

				It("does not returns error for ports prefixed with zero", func() {
					_, err := config.NewConfigFromBytes(testConfig, true)
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("when the port is too high", func() {
				BeforeEach(func() {
					addRouterGroupWithPorts("70000")
				})

				It("returns an error", func() {
					_, err := config.NewConfigFromBytes(testConfig, true)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Port must be between 1024 and 65535"))
				})
			})

			Context("when the port ranges overlap", func() {
				BeforeEach(func() {
					addRouterGroupWithPorts("1024-65535,10000-20000")
				})

				It("returns an error", func() {
					_, err := config.NewConfigFromBytes(testConfig, true)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Overlapping values: [1024-65535] and [10000-20000]"))
				})
			})

			Context("when the port range ends begins to low", func() {
				BeforeEach(func() {
					addRouterGroupWithPorts("1023-65530")
				})

				It("returns an error", func() {
					_, err := config.NewConfigFromBytes(testConfig, true)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Port must be between 1024 and 65535"))
				})
			})

			Context("when the port range is missing a start value", func() {
				BeforeEach(func() {
					addRouterGroupWithPorts("1024-65535,-10000")
				})

				It("returns an error", func() {
					_, err := config.NewConfigFromBytes(testConfig, true)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("range (-10000) requires a starting port"))
				})
			})

			Context("when the port range is missing an end value", func() {
				BeforeEach(func() {
					addRouterGroupWithPorts("10000-")
				})

				It("returns an error", func() {
					_, err := config.NewConfigFromBytes(testConfig, true)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("range (10000-) requires an ending port"))
				})
			})

			Context("when the router group type is missing", func() {
				BeforeEach(func() {
					validHash["router_groups"] = []interface{}{
						map[string]interface{}{
							"name":             "router-group-1",
							"reservable_ports": 1200,
						},
					}
				})
				It("returns an error", func() {
					_, err := config.NewConfigFromBytes(testConfig, true)
					Expect(err).To(HaveOccurred())
				})
			})

			Context("when the router group name is missing", func() {
				BeforeEach(func() {
					validHash["router_groups"] = []interface{}{
						map[string]interface{}{
							"reservable_ports": 1200,
							"type":             "tcp",
						},
					}
				})
				It("returns an error", func() {
					_, err := config.NewConfigFromBytes(testConfig, true)
					Expect(err).To(HaveOccurred())
				})
			})

			Context("when the router group reservable ports is missing", func() {
				BeforeEach(func() {
					validHash["router_groups"] = []interface{}{
						map[string]interface{}{
							"name": "router-group-1",
							"type": "tcp",
						},
					}
				})
				It("returns an error", func() {
					_, err := config.NewConfigFromBytes(testConfig, true)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Missing reservable_ports in router group:"))
				})
			})

			Context("UAA errors", func() {
				var authDisabled bool

				Context("when auth is enabled", func() {
					BeforeEach(func() {
						authDisabled = false
					})
					It("errors if no token endpoint url is found", func() {
						_, err := config.NewConfigFromBytes(testConfig, authDisabled)
						Expect(err).To(HaveOccurred())
					})
				})

				Context("when auth is disabled", func() {
					BeforeEach(func() {
						authDisabled = true
					})
					It("it return valid config", func() {
						_, err := config.NewConfigFromBytes(testConfig, authDisabled)
						Expect(err).NotTo(HaveOccurred())
					})
				})
			})

			Context("when there are no router groups seeded in the configuration file", func() {
				It("does not populates the router group", func() {
					cfg, err := config.NewConfigFromBytes(testConfig, true)
					Expect(err).NotTo(HaveOccurred())
					Expect(cfg.RouterGroups).To(BeNil())
				})
			})
		})

		Context("when reserved_system_component_ports are provided", func() {
			Context("when a port is too high", func() {
				BeforeEach(func() {
					validHash["reserved_system_component_ports"] = []int{1234, 70000, 5555}
				})

				It("returns an error", func() {
					_, err := config.NewConfigFromBytes(testConfig, true)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Invalid reserved system component port '70000'. Ports must be between 0 and 65535."))
				})
			})

			Context("when a port is too low", func() {
				BeforeEach(func() {
					validHash["reserved_system_component_ports"] = []int{1234, -10, 5555}
				})

				It("returns an error", func() {
					_, err := config.NewConfigFromBytes(testConfig, true)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Invalid reserved system component port '-10'. Ports must be between 0 and 65535."))
				})
			})
		})
	})
})
