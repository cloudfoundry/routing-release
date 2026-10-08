package registrar_test

import (
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nats-io/nats.go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"

	tls_helpers "code.cloudfoundry.org/cf-routing-test-helpers/tls"
	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagertest"
	"code.cloudfoundry.org/route-registrar/commandrunner"
	"code.cloudfoundry.org/route-registrar/config"
	healthchecker_fakes "code.cloudfoundry.org/route-registrar/healthchecker/fakes"
	messagebus_fakes "code.cloudfoundry.org/route-registrar/messagebus/messagebusfakes"
	"code.cloudfoundry.org/route-registrar/registrar"
)

var _ = Describe("Registrar.RegisterRoutes", func() {
	var (
		fakeMessageBus *messagebus_fakes.FakeMessageBus

		natsHost     string
		natsUsername string
		natsPassword string

		rrConfig config.Config

		logger lager.Logger

		signals chan os.Signal
		ready   chan struct{}

		r registrar.Registrar

		fakeHealthChecker *healthchecker_fakes.FakeHealthChecker
	)

	BeforeEach(func() {
		natsUsername = "nats-user"
		natsPassword = "nats-pw"
		natsHost = "127.0.0.1"

		logger = lagertest.NewTestLogger("Registrar test")
		servers := []string{
			fmt.Sprintf(
				"nats://%s:%s@%s:%d",
				natsUsername,
				natsPassword,
				natsHost,
				natsPort,
			),
		}

		opts := nats.GetDefaultOptions()
		opts.Servers = servers

		messageBusServer := config.MessageBusServer{
			Host:     fmt.Sprintf("%s:%d", natsHost, natsPort),
			User:     natsUsername,
			Password: natsPassword,
		}

		natsCAPath, mtlsNATSClientCertPath, mtlsNATSClientKeyPath, _ := tls_helpers.GenerateCaAndMutualTlsCerts()

		rrConfig = config.Config{
			// doesn't matter if these are the same, just want to send a slice
			MessageBusServers: []config.MessageBusServer{messageBusServer, messageBusServer},
			Host:              "my host",
			NATSmTLSConfig: config.ClientTLSConfig{
				CertPath: mtlsNATSClientCertPath,
				KeyPath:  mtlsNATSClientKeyPath,
				CAPath:   natsCAPath,
			},
			UnregistrationMessageLimit: 5,
		}

		signals = make(chan os.Signal, 1)
		ready = make(chan struct{}, 1)
		port := uint16(8080)
		port2 := uint16(8081)

		registrationInterval := 100 * time.Millisecond
		rrConfig.Routes = []config.Route{
			{
				Name: "my route 1",
				Host: "route 1 host",
				Port: &port,
				URIs: []string{
					"my uri 1.1",
					"my uri 1.2",
				},
				Tags: map[string]string{
					"tag1.1": "value1.1",
					"tag1.2": "value1.2",
				},
				RegistrationInterval: registrationInterval,
			},
			{
				Name:    "my route 2",
				Host:    "route 2 host",
				TLSPort: &port2,
				URIs: []string{
					"my uri 2.1",
					"my uri 2.2",
				},
				Tags: map[string]string{
					"tag2.1": "value2.1",
					"tag2.2": "value2.2",
				},
				RegistrationInterval: registrationInterval,
				ServerCertDomainSAN:  "my.internal.cert",
			},
		}

		fakeHealthChecker = new(healthchecker_fakes.FakeHealthChecker)
		fakeMessageBus = new(messagebus_fakes.FakeMessageBus)

		r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil, time.Minute)
	})

	It("connects to messagebus", func() {
		runStatus := make(chan error)
		go func() {
			runStatus <- r.Run(signals, ready)
		}()
		<-ready

		Expect(fakeMessageBus.ConnectCallCount()).To(Equal(1))
		_, passedTLSConfig := fakeMessageBus.ConnectArgsForCall(0)
		Expect(passedTLSConfig).NotTo(BeNil())
	})

	Context("when the client TLS config is invalid", func() {
		BeforeEach(func() {
			rrConfig.NATSmTLSConfig.CertPath = "invalid"
		})

		JustBeforeEach(func() {
			r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil, time.Minute)
		})

		It("forwards the error parsing the TLS config", func() {
			runStatus := make(chan error)
			go func() {
				runStatus <- r.Run(signals, ready)
			}()

			var returned error
			Eventually(runStatus, 3).Should(Receive(&returned))

			Expect(returned).To(MatchError(ContainSubstring("failed building NATS mTLS config")))
		})
	})

	Context("when connecting to messagebus errors", func() {
		var err error

		BeforeEach(func() {
			err = errors.New("Failed to connect")

			fakeMessageBus.ConnectStub = func([]config.MessageBusServer, *tls.Config) error {
				return err
			}
		})

		It("forwards the error", func() {
			runStatus := make(chan error)
			go func() {
				runStatus <- r.Run(signals, ready)
			}()

			returned := <-runStatus

			Expect(returned).To(Equal(err))
		})
	})

	It("unregisters on shutdown", func() {
		runStatus := make(chan error)
		go func() {
			runStatus <- r.Run(signals, ready)
		}()
		<-ready

		// wait for the initial events to be sent upon calling Run(), before shutting it off
		Eventually(fakeMessageBus.SendMessageCallCount, 100*time.Millisecond).Should(BeNumerically(">", 1))
		close(signals)
		err := <-runStatus
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(fakeMessageBus.SendMessageCallCount, 3).Should(BeNumerically(">", 3))

		subject, route, privateInstanceId := fakeMessageBus.SendMessageArgsForCall(2)
		Expect(subject).To(Equal("router.unregister"))
		Expect(route.Name).To(Equal(rrConfig.Routes[0].Name))
		Expect(route.URIs).To(Equal(rrConfig.Routes[0].URIs))
		Expect(route.Port).To(Equal(rrConfig.Routes[0].Port))
		Expect(route.Host).To(Equal(rrConfig.Routes[0].Host))
		Expect(route.Tags).To(Equal(rrConfig.Routes[0].Tags))
		Expect(privateInstanceId).NotTo(Equal(""))

		subject, route, privateInstanceId = fakeMessageBus.SendMessageArgsForCall(3)
		Expect(subject).To(Equal("router.unregister"))
		Expect(route.Name).To(Equal(rrConfig.Routes[1].Name))
		Expect(route.URIs).To(Equal(rrConfig.Routes[1].URIs))
		Expect(route.Port).To(Equal(rrConfig.Routes[1].Port))
		Expect(route.Host).To(Equal(rrConfig.Routes[1].Host))
		Expect(route.Tags).To(Equal(rrConfig.Routes[1].Tags))
		Expect(privateInstanceId).NotTo(Equal(""))
	})

	Context("when unregistering routes errors", func() {
		var err error

		BeforeEach(func() {
			err = errors.New("Failed to register")

			fakeMessageBus.SendMessageStub = func(string, config.Route, string) error {
				return err
			}
		})

		It("forwards the error", func() {
			runStatus := make(chan error)
			go func() {
				runStatus <- r.Run(signals, ready)
			}()

			<-ready
			close(signals)
			returned := <-runStatus

			Expect(returned).To(Equal(err))
		})
	})

	Context("on startup", func() {
		BeforeEach(func() {
			port := uint16(8080)
			rrConfig.Routes = []config.Route{
				{
					Name: "my route 1",
					Port: &port,
					URIs: []string{
						"my uri 1.1",
						"my uri 1.2",
					},
					Tags: map[string]string{
						"tag1.1": "value1.1",
						"tag1.2": "value1.2",
					},
					// RegistrationInterval is > the wait period in our Eventually() to ensure we've triggered on initial Run()
					RegistrationInterval: 10 * time.Second,
				},
			}
			r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil, time.Minute)
		})
		It("immediately registers all URIs", func() {
			runStatus := make(chan error)
			go func() {
				runStatus <- r.Run(signals, ready)
			}()
			<-ready

			Eventually(fakeMessageBus.SendMessageCallCount, 1).Should(Equal(1))

			subject, route, privateInstanceId := fakeMessageBus.SendMessageArgsForCall(0)
			Expect(subject).To(Equal("router.register"))

			Expect(len(rrConfig.Routes)).To(Equal(1))
			firstRoute := rrConfig.Routes[0]

			Expect(route.Name).To(Equal(firstRoute.Name))
			Expect(route.URIs).To(Equal(firstRoute.URIs))
			Expect(route.Host).To(Equal(firstRoute.Host))
			Expect(route.Port).To(Equal(firstRoute.Port))
			Expect(privateInstanceId).NotTo(Equal(""))
		})
	})

	Context("when configured with dynamic config blobs", func() {
		var (
			dynamicConfigDir string
			port             uint16
		)

		BeforeEach(func() {
			port = 8080
			rrConfig.Routes = []config.Route{
				{
					Name: "my route 1",
					Port: &port,
					URIs: []string{
						"my uri 1.1",
						"my uri 1.2",
					},
					Tags: map[string]string{
						"tag1.1": "value1.1",
						"tag1.2": "value1.2",
					},
					// RegistrationInterval is > the wait period in our Eventually() to ensure we've triggered on initial Run()
					RegistrationInterval: 1 * time.Minute,
				},
			}

			var err error
			dynamicConfigDir, err = os.MkdirTemp(os.TempDir(), "config-")
			Expect(err).NotTo(HaveOccurred())

			rrConfig.DynamicConfigGlobs = []string{fmt.Sprintf("%s/config.yml", dynamicConfigDir)}

			r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil, 100*time.Millisecond)
		})

		AfterEach(func() {
			Expect(os.RemoveAll(dynamicConfigDir)).To(Succeed())
		})

		It("starts health checking discovered routes", func() {
			runStatus := make(chan error)
			go func() {
				runStatus <- r.Run(signals, ready)
			}()
			<-ready
			Eventually(fakeMessageBus.SendMessageCallCount, 1).Should(Equal(1))

			routesBytes, err := yaml.Marshal(registrar.RoutesConfigSchema{Routes: []config.RouteSchema{
				{
					Name:                 "some-dynamic-route",
					Port:                 &port,
					RegistrationInterval: "1s",
					URIs:                 []string{"some-dynamic-route.apps.com"},
				},
			}})
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(filepath.Join(dynamicConfigDir, "config.yml"), routesBytes, 0644)
			Expect(err).NotTo(HaveOccurred())

			Eventually(fakeMessageBus.SendMessageCallCount, 2).Should(Equal(2))

			subject, route, privateInstanceId := fakeMessageBus.SendMessageArgsForCall(1)
			Expect(subject).To(Equal("router.register"))

			Expect(len(rrConfig.Routes)).To(Equal(1))

			Expect(route.Name).To(Equal("some-dynamic-route"))
			Expect(route.URIs).To(Equal([]string{"some-dynamic-route.apps.com"}))
			Expect(route.Port).To(Equal(&port))
			Expect(route.Host).To(Equal(rrConfig.Host))
			Expect(privateInstanceId).NotTo(Equal(""))

			routesBytes, err = yaml.Marshal(registrar.RoutesConfigSchema{Routes: []config.RouteSchema{}})
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(filepath.Join(dynamicConfigDir, "config.yml"), routesBytes, 0644)
			Expect(err).NotTo(HaveOccurred())

			Eventually(fakeMessageBus.SendMessageCallCount, 2).Should(Equal(3))
			subject, route, privateInstanceId = fakeMessageBus.SendMessageArgsForCall(2)
			Expect(subject).To(Equal("router.unregister"))

			Expect(len(rrConfig.Routes)).To(Equal(1))

			Expect(route.Name).To(Equal("some-dynamic-route"))
			Expect(route.URIs).To(Equal([]string{"some-dynamic-route.apps.com"}))
			Expect(route.Host).To(Equal(rrConfig.Host))
			Expect(route.Port).To(Equal(&port))
			Expect(privateInstanceId).NotTo(Equal(""))
		})
	})

	It("periodically registers all URIs for all routes", func() {
		runStatus := make(chan error)
		go func() {
			runStatus <- r.Run(signals, ready)
		}()
		<-ready

		Eventually(fakeMessageBus.SendMessageCallCount, 3).Should(BeNumerically(">", 1))

		subject, route, privateInstanceId := fakeMessageBus.SendMessageArgsForCall(0)
		Expect(subject).To(Equal("router.register"))

		var firstRoute, secondRoute config.Route
		if route.Name == rrConfig.Routes[0].Name {
			firstRoute = rrConfig.Routes[0]
			secondRoute = rrConfig.Routes[1]
		} else {
			firstRoute = rrConfig.Routes[1]
			secondRoute = rrConfig.Routes[0]
		}

		Expect(route.Name).To(Equal(firstRoute.Name))
		Expect(route.URIs).To(Equal(firstRoute.URIs))
		Expect(route.Host).To(Equal(firstRoute.Host))
		Expect(route.Port).To(Equal(firstRoute.Port))
		Expect(privateInstanceId).NotTo(Equal(""))

		subject, route, privateInstanceId = fakeMessageBus.SendMessageArgsForCall(1)
		Expect(subject).To(Equal("router.register"))

		Expect(route.Name).To(Equal(secondRoute.Name))
		Expect(route.URIs).To(Equal(secondRoute.URIs))
		Expect(route.Host).To(Equal(secondRoute.Host))
		Expect(route.Port).To(Equal(secondRoute.Port))
		Expect(privateInstanceId).NotTo(Equal(""))
	})

	Context("when registering routes errors", func() {
		var err error

		BeforeEach(func() {
			err = errors.New("Failed to register")

			fakeMessageBus.SendMessageStub = func(string, config.Route, string) error {
				return err
			}
		})

		It("forwards the error", func() {
			runStatus := make(chan error)
			go func() {
				runStatus <- r.Run(signals, ready)
			}()

			<-ready
			returned := <-runStatus

			Expect(returned).To(Equal(err))
		})
	})

	Context("given a healthcheck", func() {
		var scriptPath string

		BeforeEach(func() {
			scriptPath = "/path/to/some/script/"

			timeout := 100 * time.Millisecond
			rrConfig.Routes[0].HealthCheck = &config.HealthCheck{
				Name:       "My Healthcheck process",
				ScriptPath: scriptPath,
				Timeout:    timeout,
			}
			rrConfig.Routes[1].HealthCheck = &config.HealthCheck{
				Name:       "My Healthcheck process 2",
				ScriptPath: scriptPath,
				Timeout:    timeout,
			}

			r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil, time.Minute)
		})

		Context("and the healthcheck succeeds", func() {
			BeforeEach(func() {
				fakeHealthChecker.CheckReturns(true, nil)

				r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil, time.Minute)
			})

			It("registers routes", func() {
				runStatus := make(chan error)
				go func() {
					runStatus <- r.Run(signals, ready)
				}()
				<-ready

				Eventually(fakeMessageBus.SendMessageCallCount, 3).Should(BeNumerically(">", 1))

				subject, route, privateInstanceId := fakeMessageBus.SendMessageArgsForCall(0)
				Expect(subject).To(Equal("router.register"))

				var firstRoute, secondRoute config.Route
				if route.Name == rrConfig.Routes[0].Name {
					firstRoute = rrConfig.Routes[0]
					secondRoute = rrConfig.Routes[1]
				} else {
					firstRoute = rrConfig.Routes[1]
					secondRoute = rrConfig.Routes[0]
				}

				Expect(route.Name).To(Equal(firstRoute.Name))
				Expect(route.URIs).To(Equal(firstRoute.URIs))
				Expect(route.Host).To(Equal(firstRoute.Host))
				Expect(route.Port).To(Equal(firstRoute.Port))
				Expect(privateInstanceId).NotTo(Equal(""))

				subject, route, privateInstanceId = fakeMessageBus.SendMessageArgsForCall(1)
				Expect(subject).To(Equal("router.register"))

				Expect(route.Name).To(Equal(secondRoute.Name))
				Expect(route.URIs).To(Equal(secondRoute.URIs))
				Expect(route.Host).To(Equal(secondRoute.Host))
				Expect(route.Port).To(Equal(secondRoute.Port))
				Expect(privateInstanceId).NotTo(Equal(""))
			})

			Context("when registering routes errors", func() {
				var err error

				BeforeEach(func() {
					err = errors.New("Failed to register")

					fakeMessageBus.SendMessageStub = func(string, config.Route, string) error {
						return err
					}
				})

				It("forwards the error", func() {
					runStatus := make(chan error)
					go func() {
						runStatus <- r.Run(signals, ready)
					}()

					<-ready
					returned := <-runStatus

					Expect(returned).To(Equal(err))
				})
			})
		})

		Context("when the healthcheck fails", func() {
			BeforeEach(func() {
				fakeHealthChecker.CheckReturns(false, nil)

				r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil, time.Minute)
			})

			It("unregisters routes", func() {
				runStatus := make(chan error)
				go func() {
					runStatus <- r.Run(signals, ready)
				}()
				<-ready

				Eventually(fakeMessageBus.SendMessageCallCount, 3).Should(BeNumerically(">", 1))

				subject, route, privateInstanceId := fakeMessageBus.SendMessageArgsForCall(0)
				Expect(subject).To(Equal("router.unregister"))

				var firstRoute, secondRoute config.Route
				if route.Name == rrConfig.Routes[0].Name {
					firstRoute = rrConfig.Routes[0]
					secondRoute = rrConfig.Routes[1]
				} else {
					firstRoute = rrConfig.Routes[1]
					secondRoute = rrConfig.Routes[0]
				}

				Expect(route.Name).To(Equal(firstRoute.Name))
				Expect(route.URIs).To(Equal(firstRoute.URIs))
				Expect(route.Port).To(Equal(firstRoute.Port))
				Expect(route.Host).To(Equal(firstRoute.Host))
				Expect(privateInstanceId).NotTo(Equal(""))

				subject, route, privateInstanceId = fakeMessageBus.SendMessageArgsForCall(1)
				Expect(subject).To(Equal("router.unregister"))

				Expect(route.Name).To(Equal(secondRoute.Name))
				Expect(route.URIs).To(Equal(secondRoute.URIs))
				Expect(route.Port).To(Equal(secondRoute.Port))
				Expect(route.Host).To(Equal(secondRoute.Host))
				Expect(privateInstanceId).NotTo(Equal(""))
			})

			Context("when unregistering routes errors", func() {
				var err error

				BeforeEach(func() {
					err = errors.New("Failed to unregister")

					fakeMessageBus.SendMessageStub = func(string, config.Route, string) error {
						return err
					}
				})

				It("forwards the error", func() {
					runStatus := make(chan error)
					go func() {
						runStatus <- r.Run(signals, ready)
					}()

					<-ready
					returned := <-runStatus

					Expect(returned).To(Equal(err))
				})
			})
		})

		Context("when the healthcheck keeps failing", func() {
			Context("when there is one route with a failing endpoint", func() {
				BeforeEach(func() {
					timeout := 100 * time.Millisecond
					registrationInterval := 100 * time.Millisecond
					port := uint16(8080)
					rrConfig.Routes = []config.Route{
						{
							Name: "my route 1",
							Port: &port,
							URIs: []string{
								"my uri 1.1",
							},
							Tags: map[string]string{
								"tag1.1": "value1.1",
								"tag1.2": "value1.2",
							},
							RegistrationInterval: registrationInterval,
							HealthCheck: &config.HealthCheck{
								Name:       "My Healthcheck process",
								ScriptPath: "pass",
								Timeout:    timeout,
							},
						},
					}

					fakeHealthChecker.CheckReturns(false, nil)
					r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil, time.Minute)
				})

				It("only sends five unregistration messages per route", func() {
					runStatus := make(chan error)
					go func() {
						runStatus <- r.Run(signals, ready)
					}()
					<-ready

					Eventually(fakeMessageBus.SendMessageCallCount, 3).Should(Equal(5))

					for i := 0; i < 5; i++ {
						subject, route, privateInstanceId := fakeMessageBus.SendMessageArgsForCall(i)
						Expect(subject).To(Equal("router.unregister"))
						Expect(route.Name).To(Equal(rrConfig.Routes[0].Name))
						Expect(route.URIs).To(Equal(rrConfig.Routes[0].URIs))
						Expect(route.Port).To(Equal(rrConfig.Routes[0].Port))
						Expect(route.Host).To(Equal(rrConfig.Routes[0].Host))
						Expect(privateInstanceId).NotTo(Equal(""))
					}

					Consistently(fakeMessageBus.SendMessageCallCount, 3).Should(Equal(5))
				})
			})

			Context("when there are multiple routes with failing endpoints", func() {
				var (
					route1Name string
					route2Name string
				)

				BeforeEach(func() {
					timeout := 100 * time.Millisecond
					registrationInterval := 100 * time.Millisecond
					port := uint16(8080)
					route1Name = "my route 1"
					route2Name = "my route 2"
					rrConfig.Routes = []config.Route{
						{
							Name: route1Name,
							Port: &port,
							URIs: []string{
								"my uri 1.1",
							},
							Tags: map[string]string{
								"tag1.1": "value1.1",
								"tag1.2": "value1.2",
							},
							RegistrationInterval: registrationInterval,
							HealthCheck: &config.HealthCheck{
								Name:       "My Healthcheck process",
								ScriptPath: "pass",
								Timeout:    timeout,
							},
						},
						{
							Name: route2Name,
							Port: &port,
							URIs: []string{
								"my uri 1.1",
							},
							Tags: map[string]string{
								"tag1.1": "value1.1",
								"tag1.2": "value1.2",
							},
							RegistrationInterval: registrationInterval,
							HealthCheck: &config.HealthCheck{
								Name:       "My Healthcheck process",
								ScriptPath: "fail",
								Timeout:    timeout,
							},
						},
					}

					fakeHealthChecker.CheckReturns(false, nil)
					r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil, time.Minute)
				})

				It("sends five registration messages for each route", func() {
					runStatus := make(chan error)
					go func() {
						runStatus <- r.Run(signals, ready)
					}()
					<-ready

					Eventually(fakeMessageBus.SendMessageCallCount, 3).Should(Equal(10))

					route1Counter := 0
					route2Counter := 0

					for i := 0; i < 10; i++ {
						subject, route, _ := fakeMessageBus.SendMessageArgsForCall(i)
						Expect(subject).To(Equal("router.unregister"))

						if route.Name == route1Name {
							route1Counter++
						}

						if route.Name == route2Name {
							route2Counter++
						}
					}

					Expect(route1Counter).To(Equal(5))
					Expect(route2Counter).To(Equal(5))
					Consistently(fakeMessageBus.SendMessageCallCount, 3).Should(Equal(10))
				})
			})
			Context("when one route has a failing healthcheck and another route has as passing healthcheck", func() {
				var (
					route1Name string
					route2Name string
				)

				BeforeEach(func() {
					timeout := 100 * time.Millisecond
					registrationInterval := 100 * time.Millisecond
					port := uint16(8080)
					route1Name = "my route 1"
					route2Name = "my route 2"
					rrConfig.Routes = []config.Route{
						{
							Name: route1Name,
							Port: &port,
							Host: "my host 1",
							URIs: []string{
								"my uri 1.1",
							},
							Tags: map[string]string{
								"tag1.1": "value1.1",
								"tag1.2": "value1.2",
							},
							RegistrationInterval: registrationInterval,
							HealthCheck: &config.HealthCheck{
								Name:       "My Healthcheck process",
								ScriptPath: "pass",
								Timeout:    timeout,
							},
						},
						{
							Name: route2Name,
							Port: &port,
							Host: "my host 2",
							URIs: []string{
								"my uri 1.1",
							},
							Tags: map[string]string{
								"tag1.1": "value1.1",
								"tag1.2": "value1.2",
							},
							RegistrationInterval: registrationInterval,
							HealthCheck: &config.HealthCheck{
								Name:       "My Healthcheck process",
								ScriptPath: "fail",
								Timeout:    timeout,
							},
						},
					}

					fakeHealthChecker.CheckStub = func(cr commandrunner.Runner, path string, timeout time.Duration) (bool, error) {
						if path == "pass" {
							return true, nil
						}
						return false, errors.New("oh no I failed")
					}

					r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil, time.Minute)
				})

				It("only sends five unregistration messages for the failing app", func() {
					runStatus := make(chan error)
					go func() {
						runStatus <- r.Run(signals, ready)
					}()
					<-ready

					Eventually(fakeMessageBus.SendMessageCallCount, 3).Should(BeNumerically(">", 15))

					route2Counter := 0 // failing app

					for i := 0; i < 15; i++ {
						subject, route, _ := fakeMessageBus.SendMessageArgsForCall(i)

						if route.Name == route1Name {
							Expect(subject).To(Equal("router.register"))
						}

						if route.Name == route2Name {
							Expect(subject).To(Equal("router.unregister"))
							route2Counter++
						}
					}

					Expect(route2Counter).To(Equal(5))
				})
			})
		})

		Context("when a route is healthy, then becomes unhealthy, then healthy, and then unhealthy again", func() {
			var (
				routeName  string
				runCounter int
			)

			BeforeEach(func() {
				timeout := 100 * time.Millisecond
				registrationInterval := 100 * time.Millisecond
				port := uint16(8080)
				routeName = "my route 1"
				rrConfig.Routes = []config.Route{
					{
						Name: routeName,
						Host: "my host 1",
						Port: &port,
						URIs: []string{
							"my uri 1.1",
						},
						Tags: map[string]string{
							"tag1.1": "value1.1",
							"tag1.2": "value1.2",
						},
						RegistrationInterval: registrationInterval,
						HealthCheck: &config.HealthCheck{
							Name:       "My Healthcheck process",
							ScriptPath: "fail->pass->fail",
							Timeout:    timeout,
						},
					},
				}

				runCounter = 0

				fakeHealthChecker.CheckStub = func(
					runner commandrunner.Runner,
					path string,
					timeout time.Duration,
				) (bool, error) {
					runCounter++
					if runCounter <= 5 {
						return true, nil
					}

					if runCounter <= 10 {
						return false, errors.New("some failure")
					}

					if runCounter <= 15 {
						return true, nil
					}

					return false, errors.New("some failure")
				}

				r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil, time.Minute)
			})

			It("registers and unregisters properly as the route's health changes", func() {
				runStatus := make(chan error)
				go func() {
					runStatus <- r.Run(signals, ready)
				}()
				<-ready

				Eventually(fakeMessageBus.SendMessageCallCount, 3).Should(Equal(20))

				for i := 0; i < 20; i++ {
					subject, route, _ := fakeMessageBus.SendMessageArgsForCall(i)
					Expect(route.Name).To(Equal(routeName))
					if i < 5 {
						Expect(subject).To(Equal("router.register"))
						continue
					}
					if i < 10 {
						Expect(subject).To(Equal("router.unregister"))
						continue
					}
					if i < 15 {
						Expect(subject).To(Equal("router.register"))
						continue
					}
					if i < 20 {
						Expect(subject).To(Equal("router.unregister"))
					}
				}
			})
		})

		Context("when the healthcheck errors", func() {
			var healthcheckErr error

			BeforeEach(func() {
				healthcheckErr = fmt.Errorf("boom")
				fakeHealthChecker.CheckReturns(true, healthcheckErr)

				r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil, time.Minute)
			})

			It("unregisters routes", func() {
				runStatus := make(chan error)
				go func() {
					runStatus <- r.Run(signals, ready)
				}()
				<-ready

				Eventually(fakeMessageBus.SendMessageCallCount, 3).Should(BeNumerically(">", 1))

				subject, route, privateInstanceId := fakeMessageBus.SendMessageArgsForCall(0)
				Expect(subject).To(Equal("router.unregister"))

				var firstRoute, secondRoute config.Route
				if route.Name == rrConfig.Routes[0].Name {
					firstRoute = rrConfig.Routes[0]
					secondRoute = rrConfig.Routes[1]
				} else {
					firstRoute = rrConfig.Routes[1]
					secondRoute = rrConfig.Routes[0]
				}

				Expect(route.Name).To(Equal(firstRoute.Name))
				Expect(route.URIs).To(Equal(firstRoute.URIs))
				Expect(route.Host).To(Equal(firstRoute.Host))
				Expect(route.Port).To(Equal(firstRoute.Port))
				Expect(privateInstanceId).NotTo(Equal(""))

				subject, route, privateInstanceId = fakeMessageBus.SendMessageArgsForCall(1)
				Expect(subject).To(Equal("router.unregister"))

				Expect(route.Name).To(Equal(secondRoute.Name))
				Expect(route.URIs).To(Equal(secondRoute.URIs))
				Expect(route.Host).To(Equal(secondRoute.Host))
				Expect(route.Port).To(Equal(secondRoute.Port))
				Expect(privateInstanceId).NotTo(Equal(""))
			})

			Context("when unregistering routes errors", func() {
				var err error

				BeforeEach(func() {
					err = errors.New("Failed to unregister")

					fakeMessageBus.SendMessageStub = func(string, config.Route, string) error {
						return err
					}
				})

				It("forwards the error", func() {
					runStatus := make(chan error)
					go func() {
						runStatus <- r.Run(signals, ready)
					}()

					<-ready
					returned := <-runStatus

					Expect(returned).To(Equal(err))
				})
			})
		})

		Context("when the healthcheck is in progress", func() {
			BeforeEach(func() {
				fakeHealthChecker.CheckStub = func(commandrunner.Runner, string, time.Duration) (bool, error) {
					time.Sleep(10 * time.Second)
					return true, nil
				}

				r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil, time.Minute)
			})

			It("returns instantly upon interrupt", func() {
				runStatus := make(chan error)
				go func() {
					runStatus <- r.Run(signals, ready)
				}()
				<-ready

				// Must be greater than the registration interval to ensure the loop runs
				// at least once
				time.Sleep(1500 * time.Millisecond)

				close(signals)
				Eventually(runStatus, 100*time.Millisecond).Should(Receive(BeNil()))
			})
		})
	})
})
