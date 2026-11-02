package registrar_test

import (
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"
	"gopkg.in/yaml.v3"

	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagertest"
	"code.cloudfoundry.org/route-registrar/config"
	"code.cloudfoundry.org/route-registrar/registrar"
)

var _ = Describe("RoutesConfigWatcher", func() {
	var (
		routesConfigWatcher ifrit.Runner
		logger              lager.Logger

		process ifrit.Process

		route1Schema, route2Schema, route3Schema, route4Schema config.RouteSchema
		route1, route2, route3, route4                         config.Route

		routesDiscovered, routesRemoved chan config.Route
		cfgDir                          string
		glob, host                      string
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("Registrar test")
		var err error
		cfgDir, err = os.MkdirTemp(os.TempDir(), "config-")
		Expect(err).NotTo(HaveOccurred())

		glob = fmt.Sprintf("%s/config-*.yml*", cfgDir)
		host = "127.0.0.1"
		routesDiscovered = make(chan config.Route)
		routesRemoved = make(chan config.Route)

		routesConfigWatcher = registrar.NewRoutesConfigWatcher(logger, time.Second, []string{glob}, host, routesDiscovered, routesRemoved)

		port := uint16(8080)
		route1 = config.Route{
			Name:                 "tcp-without-host",
			Type:                 "tcp",
			Host:                 "127.0.0.1",
			Port:                 &port,
			RouterGroup:          "some-router-group",
			RegistrationInterval: time.Second,
			URIs:                 []string{"some-route-1.apps.com"},
			Tags:                 map[string]string{},
		}
		route1Schema = config.RouteSchema{
			Name:                 "tcp-without-host",
			Type:                 "tcp",
			Port:                 &port,
			RouterGroup:          "some-router-group",
			RegistrationInterval: "1s",
			URIs:                 []string{"some-route-1.apps.com"},
		}

		route2 = config.Route{
			Name:                 "tcp-with-host",
			Type:                 "tcp",
			Host:                 "168.0.0.1",
			Port:                 &port,
			RouterGroup:          "some-router-group",
			RegistrationInterval: 2 * time.Second,
			URIs:                 []string{"some-route-2.apps.com"},
			Tags:                 map[string]string{},
		}
		route2Schema = config.RouteSchema{
			Name:                 "tcp-with-host",
			Type:                 "tcp",
			Host:                 "168.0.0.1",
			Port:                 &port,
			RouterGroup:          "some-router-group",
			RegistrationInterval: "2s",
			URIs:                 []string{"some-route-2.apps.com"},
		}

		route3 = config.Route{
			Name:                 "some-route-3",
			Host:                 "127.0.0.1",
			Port:                 &port,
			RegistrationInterval: 3 * time.Second,
			URIs:                 []string{"some-route-3.apps.com"},
			Tags:                 map[string]string{},
		}
		route3Schema = config.RouteSchema{
			Name:                 "some-route-3",
			Port:                 &port,
			RegistrationInterval: "3s",
			URIs:                 []string{"some-route-3.apps.com"},
		}

		route4 = config.Route{
			Name:                 "some-route-4",
			Host:                 "127.0.0.1",
			Port:                 &port,
			RegistrationInterval: 3 * time.Second,
			URIs:                 []string{"some-route-4.apps.com"},
			Tags:                 map[string]string{},
		}
		route4Schema = config.RouteSchema{
			Name:                 "some-route-4",
			Port:                 &port,
			RegistrationInterval: "3s",
			URIs:                 []string{"some-route-4.apps.com"},
		}
	})

	JustBeforeEach(func() {
		process = ifrit.Background(routesConfigWatcher)
	})

	AfterEach(func() {
		process.Signal(os.Interrupt)
		Eventually(process.Wait(), 5*time.Second).Should(Receive())
		os.RemoveAll(cfgDir)
	})

	Context("when directory has no files", func() {
		It("does not discover any routes", func() {
			Consistently(routesDiscovered).ShouldNot(Receive())
		})

		Context("when config file is created", func() {
			It("loads all routes from the config", func() {
				Consistently(routesDiscovered).ShouldNot(Receive())

				routesBytes1, err := yaml.Marshal(registrar.RoutesConfigSchema{Routes: []config.RouteSchema{route1Schema, route2Schema}})
				Expect(err).NotTo(HaveOccurred())
				cfgFile1, err := os.CreateTemp(cfgDir, "config-1.yml")
				Expect(err).NotTo(HaveOccurred())
				_, err = cfgFile1.Write(routesBytes1)
				Expect(err).NotTo(HaveOccurred())

				routesBytes2, err := yaml.Marshal(registrar.RoutesConfigSchema{Routes: []config.RouteSchema{route3Schema}})
				Expect(err).NotTo(HaveOccurred())
				cfgFile2, err := os.CreateTemp(cfgDir, "config-2.yml")
				Expect(err).NotTo(HaveOccurred())
				_, err = cfgFile2.Write(routesBytes2)
				Expect(err).NotTo(HaveOccurred())

				var receivedRoute config.Route
				Eventually(routesDiscovered, 2).Should(Receive(&receivedRoute))
				Expect(receivedRoute).To(Equal(route1))
				Eventually(routesDiscovered, 2).Should(Receive(&receivedRoute))
				Expect(receivedRoute).To(Equal(route2))
				Eventually(routesDiscovered, 2).Should(Receive(&receivedRoute))
				Expect(receivedRoute).To(Equal(route3))
			})
		})
	})

	Context("when directory has config files already", func() {
		var (
			cfgFile1 *os.File
			cfgFile2 *os.File
		)

		BeforeEach(func() {
			var err error
			cfgFile1, err = os.CreateTemp(cfgDir, "config-1.yml")
			Expect(err).NotTo(HaveOccurred())

			cfgFile2, err = os.CreateTemp(cfgDir, "config-2.yml")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			os.Remove(cfgFile1.Name())
			os.Remove(cfgFile2.Name())
		})

		Context("when config files have routes", func() {
			BeforeEach(func() {
				routesBytes1, err := yaml.Marshal(registrar.RoutesConfigSchema{Routes: []config.RouteSchema{route1Schema, route2Schema}})
				Expect(err).NotTo(HaveOccurred())
				_, err = cfgFile1.Write(routesBytes1)
				Expect(err).NotTo(HaveOccurred())

				routesBytes2, err := yaml.Marshal(registrar.RoutesConfigSchema{Routes: []config.RouteSchema{route3Schema}})
				Expect(err).NotTo(HaveOccurred())
				_, err = cfgFile2.Write(routesBytes2)
				Expect(err).NotTo(HaveOccurred())
			})

			It("loads all routes from the config", func() {
				var receivedRoute config.Route
				Eventually(routesDiscovered, 2).Should(Receive(&receivedRoute))
				Expect(receivedRoute).To(Equal(route1))
				Eventually(routesDiscovered, 2).Should(Receive(&receivedRoute))
				Expect(receivedRoute).To(Equal(route2))
				Eventually(routesDiscovered, 2).Should(Receive(&receivedRoute))
				Expect(receivedRoute).To(Equal(route3))
			})

			Context("when config file is updated and new route is added", func() {
				It("notifies that route is discovered", func() {
					var receivedRoute config.Route
					Eventually(routesDiscovered, 2).Should(Receive(&receivedRoute))
					Expect(receivedRoute).To(Equal(route1))
					Eventually(routesDiscovered, 2).Should(Receive(&receivedRoute))
					Expect(receivedRoute).To(Equal(route2))
					Eventually(routesDiscovered, 2).Should(Receive(&receivedRoute))
					Expect(receivedRoute).To(Equal(route3))

					routesBytes2, err := yaml.Marshal(registrar.RoutesConfigSchema{Routes: []config.RouteSchema{route3Schema, route4Schema}})
					Expect(err).NotTo(HaveOccurred())
					err = cfgFile2.Truncate(0)
					Expect(err).NotTo(HaveOccurred())
					_, err = cfgFile2.Seek(0, 0)
					Expect(err).NotTo(HaveOccurred())
					_, err = cfgFile2.Write(routesBytes2)
					Expect(err).NotTo(HaveOccurred())

					Eventually(routesDiscovered, 2).Should(Receive(&receivedRoute))
					Expect(receivedRoute).To(Equal(route4))
				})
			})

			Context("when config file is updated and route is removed", func() {
				It("notifies that route is removed", func() {
					var receivedRoute config.Route
					Eventually(routesDiscovered, 2).Should(Receive(&receivedRoute))
					Expect(receivedRoute).To(Equal(route1))
					Eventually(routesDiscovered, 2).Should(Receive(&receivedRoute))
					Expect(receivedRoute).To(Equal(route2))
					Eventually(routesDiscovered, 2).Should(Receive(&receivedRoute))
					Expect(receivedRoute).To(Equal(route3))

					routesBytes1, err := yaml.Marshal(registrar.RoutesConfigSchema{Routes: []config.RouteSchema{route1Schema}})
					Expect(err).NotTo(HaveOccurred())
					err = cfgFile1.Truncate(0)
					Expect(err).NotTo(HaveOccurred())
					_, err = cfgFile1.Seek(0, 0)
					Expect(err).NotTo(HaveOccurred())
					_, err = cfgFile1.Write(routesBytes1)
					Expect(err).NotTo(HaveOccurred())

					Eventually(routesRemoved, 2).Should(Receive(&receivedRoute))
					Expect(receivedRoute).To(Equal(route2))

					Consistently(routesRemoved).ShouldNot(Receive())
				})
			})

			Context("when config file is updated and several routes are removed", func() {
				BeforeEach(func() {
					// rewrite file1 so it holds three routes before the watcher first runs
					routesBytes, err := yaml.Marshal(registrar.RoutesConfigSchema{Routes: []config.RouteSchema{route1Schema, route2Schema, route4Schema}})
					Expect(err).NotTo(HaveOccurred())
					Expect(cfgFile1.Truncate(0)).To(Succeed())
					_, err = cfgFile1.Seek(0, 0)
					Expect(err).NotTo(HaveOccurred())
					_, err = cfgFile1.Write(routesBytes)
					Expect(err).NotTo(HaveOccurred())
				})

				updateFile1 := func(schemas ...config.RouteSchema) {
					routesBytes, err := yaml.Marshal(registrar.RoutesConfigSchema{Routes: schemas})
					Expect(err).NotTo(HaveOccurred())
					Expect(cfgFile1.Truncate(0)).To(Succeed())
					_, err = cfgFile1.Seek(0, 0)
					Expect(err).NotTo(HaveOccurred())
					_, err = cfgFile1.Write(routesBytes)
					Expect(err).NotTo(HaveOccurred())
				}

				drainDiscovered := func() {
					var r config.Route
					for i := 0; i < 4; i++ { // route1, route2, route4 from file1 + route3 from file2
						Eventually(routesDiscovered, 2).Should(Receive(&r))
					}
				}

				It("notifies about every removed route without panicking when several are removed at once", func() {
					drainDiscovered()
					updateFile1(route1Schema)

					var removed config.Route
					Eventually(routesRemoved, 3).Should(Receive(&removed))
					Expect(removed).To(Equal(route2))
					Eventually(routesRemoved, 3).Should(Receive(&removed))
					Expect(removed).To(Equal(route4))
					Consistently(routesRemoved).ShouldNot(Receive())
				})
			})

			Context("when config file is removed", func() {
				It("removes routes from that config file", func() {
					var receivedRoute config.Route
					Eventually(routesDiscovered, 2).Should(Receive(&receivedRoute))
					Expect(receivedRoute).To(Equal(route1))
					Eventually(routesDiscovered, 2).Should(Receive(&receivedRoute))
					Expect(receivedRoute).To(Equal(route2))
					Eventually(routesDiscovered, 2).Should(Receive(&receivedRoute))
					Expect(receivedRoute).To(Equal(route3))

					err := os.Remove(cfgFile1.Name())
					Expect(err).NotTo(HaveOccurred())

					Eventually(routesRemoved, 2).Should(Receive(&receivedRoute))
					Expect(receivedRoute).To(Equal(route1))

					Eventually(routesRemoved, 2).Should(Receive(&receivedRoute))
					Expect(receivedRoute).To(Equal(route2))
				})
			})
		})

		Context("when config file is in wrong format", func() {
			BeforeEach(func() {
				_, err := cfgFile1.Write([]byte(`invalid`))
				Expect(err).NotTo(HaveOccurred())
			})

			It("logs an error and continues scaning", func() {
				Eventually(logger, 2).Should(gbytes.Say("failed-to-parse-file"))
			})
		})

		Context("when host is not set globally and in config file", func() {
			BeforeEach(func() {
				routesConfigWatcher = registrar.NewRoutesConfigWatcher(logger, time.Second, []string{glob}, "", routesDiscovered, routesRemoved)
				port := uint16(8080)
				routesBytes1, err := yaml.Marshal(registrar.RoutesConfigSchema{Routes: []config.RouteSchema{
					{
						Name:                 "tcp-without-host",
						Type:                 "tcp",
						Port:                 &port,
						RouterGroup:          "some-router-group",
						RegistrationInterval: "1s",
						URIs:                 []string{"some-route-1.apps.com"},
					},
				}})
				Expect(err).NotTo(HaveOccurred())
				_, err = cfgFile1.Write(routesBytes1)
				Expect(err).NotTo(HaveOccurred())
			})

			It("logs an error and continues scaning", func() {
				Eventually(logger, 2).Should(gbytes.Say("failed-to-parse-route"))
				Eventually(logger).Should(gbytes.Say("no host"))
			})
		})
	})
})
