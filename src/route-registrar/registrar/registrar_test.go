package registrar_test

import (
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	tls_helpers "code.cloudfoundry.org/cf-routing-test-helpers/tls"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/routing-release/route-registrar/commandrunner"
	"code.cloudfoundry.org/routing-release/route-registrar/config"
	healthchecker_fakes "code.cloudfoundry.org/routing-release/route-registrar/healthchecker/fakes"
	messagebus_fakes "code.cloudfoundry.org/routing-release/route-registrar/messagebus/fakes"
	"code.cloudfoundry.org/routing-release/route-registrar/registrar"
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

		opts := nats.DefaultOptions
		opts.Servers = servers

		messageBusServer := config.MessageBusServer{
			fmt.Sprintf("%s:%d", natsHost, natsPort),
			natsUsername,
			natsPassword,
		}

		rrConfig = config.Config{
			// doesn't matter if these are the same, just want to send a slice
			MessageBusServers: []config.MessageBusServer{messageBusServer, messageBusServer},
			Host:              "my host",
			NATSmTLSConfig: config.ClientTLSConfig{
				Enabled:  false,
				CertPath: "should-not-be-used",
				KeyPath:  "should-not-be-used",
				CAPath:   "should-not-be-used",
			},
		}

		signals = make(chan os.Signal, 1)
		ready = make(chan struct{}, 1)
		port := 8080
		port2 := 8081

		registrationInterval := 100 * time.Millisecond
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
				RegistrationInterval: registrationInterval,
			},
			{
				Name:    "my route 2",
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

		r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil)
	})

	It("connects to messagebus", func() {
		runStatus := make(chan error)
		go func() {
			runStatus <- r.Run(signals, ready)
		}()
		<-ready

		Expect(fakeMessageBus.ConnectCallCount()).To(Equal(1))
		_, passedTLSConfig := fakeMessageBus.ConnectArgsForCall(0)
		Expect(passedTLSConfig).To(BeNil())
	})

	Context("when the client TLS config is enabled", func() {
		BeforeEach(func() {
			rrConfig.NATSmTLSConfig.Enabled = true
			natsCAPath, mtlsNATSClientCertPath, mtlsNATClientKeyPath, _ := tls_helpers.GenerateCaAndMutualTlsCerts()
			rrConfig.NATSmTLSConfig.CAPath = natsCAPath
			rrConfig.NATSmTLSConfig.CertPath = mtlsNATSClientCertPath
			rrConfig.NATSmTLSConfig.KeyPath = mtlsNATClientKeyPath
		})

		JustBeforeEach(func() {
			r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil)
		})

		It("connects to the message bus with a TLS config", func() {
			runStatus := make(chan error)
			go func() {
				runStatus <- r.Run(signals, ready)
			}()
			Eventually(ready).Should(BeClosed())

			Expect(fakeMessageBus.ConnectCallCount()).To(Equal(1))
			_, passedTLSConfig := fakeMessageBus.ConnectArgsForCall(0)
			Expect(passedTLSConfig).NotTo(BeNil())
		})

		Context("when the client TLS config is invalid", func() {
			BeforeEach(func() {
				rrConfig.NATSmTLSConfig.CertPath = "invalid"
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

		close(signals)
		err := <-runStatus
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(fakeMessageBus.SendMessageCallCount, 3).Should(BeNumerically(">", 1))

		subject, host, route, privateInstanceId := fakeMessageBus.SendMessageArgsForCall(0)
		Expect(subject).To(Equal("router.unregister"))
		Expect(host).To(Equal(rrConfig.Host))
		Expect(route.Name).To(Equal(rrConfig.Routes[0].Name))
		Expect(route.URIs).To(Equal(rrConfig.Routes[0].URIs))
		Expect(route.Port).To(Equal(rrConfig.Routes[0].Port))
		Expect(route.Tags).To(Equal(rrConfig.Routes[0].Tags))
		Expect(privateInstanceId).NotTo(Equal(""))

		subject, host, route, privateInstanceId = fakeMessageBus.SendMessageArgsForCall(1)
		Expect(subject).To(Equal("router.unregister"))
		Expect(host).To(Equal(rrConfig.Host))
		Expect(route.Name).To(Equal(rrConfig.Routes[1].Name))
		Expect(route.URIs).To(Equal(rrConfig.Routes[1].URIs))
		Expect(route.Port).To(Equal(rrConfig.Routes[1].Port))
		Expect(route.Tags).To(Equal(rrConfig.Routes[1].Tags))
		Expect(privateInstanceId).NotTo(Equal(""))
	})

	Context("when unregistering routes errors", func() {
		var err error

		BeforeEach(func() {
			err = errors.New("Failed to register")

			fakeMessageBus.SendMessageStub = func(string, string, config.Route, string) error {
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

	It("periodically registers all URIs for all routes", func() {
		runStatus := make(chan error)
		go func() {
			runStatus <- r.Run(signals, ready)
		}()
		<-ready

		Eventually(fakeMessageBus.SendMessageCallCount, 3).Should(BeNumerically(">", 1))

		subject, host, route, privateInstanceId := fakeMessageBus.SendMessageArgsForCall(0)
		Expect(subject).To(Equal("router.register"))
		Expect(host).To(Equal(rrConfig.Host))

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
		Expect(privateInstanceId).NotTo(Equal(""))

		subject, host, route, privateInstanceId = fakeMessageBus.SendMessageArgsForCall(1)
		Expect(subject).To(Equal("router.register"))
		Expect(host).To(Equal(rrConfig.Host))

		Expect(route.Name).To(Equal(secondRoute.Name))
		Expect(route.URIs).To(Equal(secondRoute.URIs))
		Expect(route.Port).To(Equal(secondRoute.Port))
		Expect(privateInstanceId).NotTo(Equal(""))
	})

	Context("when registering routes errors", func() {
		var err error

		BeforeEach(func() {
			err = errors.New("Failed to register")

			fakeMessageBus.SendMessageStub = func(string, string, config.Route, string) error {
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

			r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil)
		})

		Context("and the healthcheck succeeds", func() {
			BeforeEach(func() {
				fakeHealthChecker.CheckReturns(true, nil)

				r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil)
			})

			It("registers routes", func() {
				runStatus := make(chan error)
				go func() {
					runStatus <- r.Run(signals, ready)
				}()
				<-ready

				Eventually(fakeMessageBus.SendMessageCallCount, 3).Should(BeNumerically(">", 1))

				subject, host, route, privateInstanceId := fakeMessageBus.SendMessageArgsForCall(0)
				Expect(subject).To(Equal("router.register"))
				Expect(host).To(Equal(rrConfig.Host))

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
				Expect(privateInstanceId).NotTo(Equal(""))

				subject, host, route, privateInstanceId = fakeMessageBus.SendMessageArgsForCall(1)
				Expect(subject).To(Equal("router.register"))
				Expect(host).To(Equal(rrConfig.Host))

				Expect(route.Name).To(Equal(secondRoute.Name))
				Expect(route.URIs).To(Equal(secondRoute.URIs))
				Expect(route.Port).To(Equal(secondRoute.Port))
				Expect(privateInstanceId).NotTo(Equal(""))
			})

			Context("when registering routes errors", func() {
				var err error

				BeforeEach(func() {
					err = errors.New("Failed to register")

					fakeMessageBus.SendMessageStub = func(string, string, config.Route, string) error {
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

				r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil)
			})

			It("unregisters routes", func() {
				runStatus := make(chan error)
				go func() {
					runStatus <- r.Run(signals, ready)
				}()
				<-ready

				Eventually(fakeMessageBus.SendMessageCallCount, 3).Should(BeNumerically(">", 1))

				subject, host, route, privateInstanceId := fakeMessageBus.SendMessageArgsForCall(0)
				Expect(subject).To(Equal("router.unregister"))
				Expect(host).To(Equal(rrConfig.Host))

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
				Expect(privateInstanceId).NotTo(Equal(""))

				subject, host, route, privateInstanceId = fakeMessageBus.SendMessageArgsForCall(1)
				Expect(subject).To(Equal("router.unregister"))
				Expect(host).To(Equal(rrConfig.Host))

				Expect(route.Name).To(Equal(secondRoute.Name))
				Expect(route.URIs).To(Equal(secondRoute.URIs))
				Expect(route.Port).To(Equal(secondRoute.Port))
				Expect(privateInstanceId).NotTo(Equal(""))
			})

			Context("when unregistering routes errors", func() {
				var err error

				BeforeEach(func() {
					err = errors.New("Failed to unregister")

					fakeMessageBus.SendMessageStub = func(string, string, config.Route, string) error {
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

		Context("when the healthcheck errors", func() {
			var healthcheckErr error

			BeforeEach(func() {
				healthcheckErr = fmt.Errorf("boom")
				fakeHealthChecker.CheckReturns(true, healthcheckErr)

				r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil)
			})

			It("unregisters routes", func() {
				runStatus := make(chan error)
				go func() {
					runStatus <- r.Run(signals, ready)
				}()
				<-ready

				Eventually(fakeMessageBus.SendMessageCallCount, 3).Should(BeNumerically(">", 1))

				subject, host, route, privateInstanceId := fakeMessageBus.SendMessageArgsForCall(0)
				Expect(subject).To(Equal("router.unregister"))
				Expect(host).To(Equal(rrConfig.Host))

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
				Expect(privateInstanceId).NotTo(Equal(""))

				subject, host, route, privateInstanceId = fakeMessageBus.SendMessageArgsForCall(1)
				Expect(subject).To(Equal("router.unregister"))
				Expect(host).To(Equal(rrConfig.Host))

				Expect(route.Name).To(Equal(secondRoute.Name))
				Expect(route.URIs).To(Equal(secondRoute.URIs))
				Expect(route.Port).To(Equal(secondRoute.Port))
				Expect(privateInstanceId).NotTo(Equal(""))
			})

			Context("when unregistering routes errors", func() {
				var err error

				BeforeEach(func() {
					err = errors.New("Failed to unregister")

					fakeMessageBus.SendMessageStub = func(string, string, config.Route, string) error {
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

				r = registrar.NewRegistrar(rrConfig, fakeHealthChecker, logger, fakeMessageBus, nil)
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
				Eventually(runStatus, 100*time.Millisecond).Should(Receive(nil))
			})
		})
	})
})
