package main_test

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/net/context"

	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/locket"
	"code.cloudfoundry.org/routing-release/routing-api"
	"code.cloudfoundry.org/routing-release/routing-api/cmd/routing-api/testrunner"
	"code.cloudfoundry.org/routing-release/routing-api/config"
	"code.cloudfoundry.org/routing-release/routing-api/test_helpers"

	"code.cloudfoundry.org/locket/lock"
	locketmodels "code.cloudfoundry.org/locket/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"
)

var _ = Describe("Locket", func() {
	var (
		routingAPIConfig *config.Config
		configFilePath   string
		session          *gexec.Session

		logger lager.Logger
	)

	routingAPIShouldBeReachable := func() {
		Eventually(func() error {
			_, err := client.Routes()
			return err
		}).Should(Succeed())
	}

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("locket-test")

		cc := defaultConfig
		routingAPIConfig = getRoutingAPIConfig(cc)
	})

	JustBeforeEach(func() {
		configFilePath = writeConfigToTempFile(routingAPIConfig)
		args := testrunner.Args{
			IP:         routingAPIIP,
			ConfigPath: configFilePath,
			DevMode:    true,
		}
		args.ConfigPath = configFilePath
		session = RoutingApi(args.ArgSlice()...)
	})

	AfterEach(func() {
		if session != nil {
			session.Kill().Wait(10 * time.Second)
		}

		err := os.RemoveAll(configFilePath)
		Expect(err).ToNot(HaveOccurred())
	})

	Context("with invalid configuration", func() {
		Context("and the UUID is not present", func() {
			BeforeEach(func() {
				routingAPIConfig.UUID = ""
			})

			It("exits with an error", func() {
				Eventually(session).Should(gexec.Exit(1))
			})
		})
	})
	Context("with valid configuration", func() {
		It("uses the configured UUID as the owner", func() {
			locketClient, err := locket.NewClient(logger, routingAPIConfig.Locket)
			Expect(err).NotTo(HaveOccurred())

			var lock *locketmodels.FetchResponse
			Eventually(func() error {
				lock, err = locketClient.Fetch(context.Background(), &locketmodels.FetchRequest{
					Key: "routing_api_lock",
				})
				return err
			}).ShouldNot(HaveOccurred())

			Expect(lock.Resource.Owner).To(Equal(routingAPIConfig.UUID))
		})

		It("acquires the lock in locket and becomes active", func() {
			routingAPIShouldBeReachable()
		})

		Context("and the locking server becomes unreachable after grabbing the lock", func() {
			JustBeforeEach(func() {
				routingAPIShouldBeReachable()

				ginkgomon.Interrupt(locketProcess)
			})

			It("exits", func() {
				Eventually(session, 30).Should(gexec.Exit(1))
			})
		})

		Context("when the lock is not available", func() {
			var competingProcess ifrit.Process

			BeforeEach(func() {
				locketClient, err := locket.NewClient(logger, routingAPIConfig.Locket)
				Expect(err).NotTo(HaveOccurred())

				lockIdentifier := &locketmodels.Resource{
					Key:      "routing_api_lock",
					Owner:    "Your worst enemy.",
					Value:    "Something",
					TypeCode: locketmodels.LOCK,
				}

				clock := clock.NewClock()
				competingRunner := lock.NewLockRunner(logger, locketClient, lockIdentifier, 5, clock, locket.RetryInterval)
				competingProcess = ginkgomon.Invoke(competingRunner)
			})

			AfterEach(func() {
				ginkgomon.Interrupt(competingProcess)
			})

			It("does not become active", func() {
				Consistently(func() error {
					_, err := client.Routes()
					return err
				}).ShouldNot(Succeed())
			})

			Context("and the lock becomes available", func() {
				JustBeforeEach(func() {
					Consistently(func() error {
						_, err := client.Routes()
						return err
					}).ShouldNot(Succeed())

					ginkgomon.Interrupt(competingProcess)
				})

				It("grabs the lock and becomes active", func() {
					routingAPIShouldBeReachable()
				})
			})
		})
	})

	Context("when a rolling deploy occurs", func() {
		It("ensures there is no downtime", func() {
			Eventually(session, 10*time.Second).Should(gbytes.Say("routing-api.started"))

			session2Port := uint16(test_helpers.NextAvailPort())
			session2MTLSPort := uint16(test_helpers.NextAvailPort())
			apiConfig := getRoutingAPIConfig(defaultConfig)
			apiConfig.API.ListenPort = int(session2Port)
			apiConfig.API.MTLSListenPort = int(session2MTLSPort)
			apiConfig.AdminPort = test_helpers.NextAvailPort()
			configFilePath := writeConfigToTempFile(apiConfig)
			session2Args := testrunner.Args{
				IP:         routingAPIIP,
				ConfigPath: configFilePath,
				DevMode:    true,
			}
			session2 := RoutingApi(session2Args.ArgSlice()...)

			defer func() {
				session2.Interrupt().Wait(10 * time.Second)
			}()
			Eventually(session2, 10*time.Second).Should(gbytes.Say("locket-lock.started"))
			done := make(chan struct{})
			goRoutineFinished := make(chan struct{})
			client2 := routing_api.NewClient(fmt.Sprintf("http://127.0.0.1:%d", session2Port), false)

			go func() {
				defer GinkgoRecover()

				var err1, err2 error

				ticker := time.NewTicker(time.Second)
				for range ticker.C {
					select {
					case <-done:
						close(goRoutineFinished)
						ticker.Stop()
						return
					default:
						_, err1 = client.Routes()
						_, err2 = client2.Routes()
						Expect([]error{err1, err2}).To(ContainElement(Not(HaveOccurred())), "At least one of the errors should not have occurred")
					}
				}
			}()

			session.Interrupt().Wait(10 * time.Second)

			Eventually(session2, 10*time.Second).Should(gbytes.Say("locket-lock.acquired-lock"))
			Eventually(session2, 10*time.Second).Should(gbytes.Say("routing-api.started"))

			close(done)
			Eventually(done).Should(BeClosed())
			Eventually(goRoutineFinished).Should(BeClosed())

			_, err := client2.Routes()
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
