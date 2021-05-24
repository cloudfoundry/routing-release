package haproxy_test

import (
	"code.cloudfoundry.org/routing-release/cf-tcp-router/configurer/haproxy"
	"code.cloudfoundry.org/routing-release/cf-tcp-router/configurer/haproxy/fakes"
	"code.cloudfoundry.org/routing-release/cf-tcp-router/models"
	monitorFakes "code.cloudfoundry.org/routing-release/cf-tcp-router/monitor/fakes"
	"code.cloudfoundry.org/routing-release/cf-tcp-router/testutil"
	"code.cloudfoundry.org/routing-release/cf-tcp-router/utils"
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"os"
)

var _ = Describe("HaproxyConfigurer", func() {
	Describe("Configure", func() {
		const (
			haproxyConfigTemplate = "fixtures/haproxy.cfg.template"
			haproxyConfigFile     = "fixtures/haproxy.cfg"
		)

		var (
			haproxyConfigurer *haproxy.Configurer
			fakeMonitor       *monitorFakes.FakeMonitor
		)

		BeforeEach(func() {
			fakeMonitor = &monitorFakes.FakeMonitor{}
		})

		Context("when empty base configuration file is passed", func() {
			It("returns a ErrRouterConfigFileNotFound error", func() {
				_, err := haproxy.NewHaProxyConfigurer(logger, haproxy.NewConfigMarshaller(), "", haproxyConfigFile, fakeMonitor, nil)
				Expect(err).Should(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(haproxy.ErrRouterConfigFileNotFound))
			})
		})

		Context("when empty configuration file is passed", func() {
			It("returns a ErrRouterConfigFileNotFound error", func() {
				_, err := haproxy.NewHaProxyConfigurer(logger, haproxy.NewConfigMarshaller(), haproxyConfigTemplate, "", fakeMonitor, nil)
				Expect(err).Should(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(haproxy.ErrRouterConfigFileNotFound))
			})
		})

		Context("when base configuration file does not exist", func() {
			It("returns a ErrRouterConfigFileNotFound error", func() {
				_, err := haproxy.NewHaProxyConfigurer(logger, haproxy.NewConfigMarshaller(), "file/path/does/not/exist", haproxyConfigFile, fakeMonitor, nil)
				Expect(err).Should(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(haproxy.ErrRouterConfigFileNotFound))
			})
		})

		Context("when configuration file does not exist", func() {
			It("returns a ErrRouterConfigFileNotFound error", func() {
				_, err := haproxy.NewHaProxyConfigurer(logger, haproxy.NewConfigMarshaller(), haproxyConfigTemplate, "file/path/does/not/exist", fakeMonitor, nil)
				Expect(err).Should(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(haproxy.ErrRouterConfigFileNotFound))
			})
		})

		Context("when necessary files exist", func() {
			var (
				originalConfigTemplateContent []byte
				currentConfigTemplateContent  []byte
				routingTable                  models.RoutingTable
				fakeMarshaller                *fakes.FakeConfigMarshaller
				fakeScriptRunner              *fakes.FakeScriptRunner
				generatedHaproxyCfgFile       string
				haproxyCfgBackupFile          string
				err                           error
			)

			marshallerContent := "whatever the marshaller generates to represent the HAProxyConfig"

			BeforeEach(func() {
				currentConfigTemplateContent = []byte{}
				routingTable = models.NewRoutingTable(logger)

				generatedHaproxyCfgFile = testutil.RandomFileName("fixtures/haproxy_", ".cfg")
				haproxyCfgBackupFile = fmt.Sprintf("%s.bak", generatedHaproxyCfgFile)
				_ = utils.CopyFile(haproxyConfigTemplate, generatedHaproxyCfgFile)

				originalConfigTemplateContent, err = ioutil.ReadFile(generatedHaproxyCfgFile)
				Expect(err).ShouldNot(HaveOccurred())

				fakeMarshaller = new(fakes.FakeConfigMarshaller)
				fakeScriptRunner = new(fakes.FakeScriptRunner)
				haproxyConfigurer, err = haproxy.NewHaProxyConfigurer(logger, fakeMarshaller, haproxyConfigTemplate, generatedHaproxyCfgFile, fakeMonitor, fakeScriptRunner)
				Expect(err).ShouldNot(HaveOccurred())

				fakeMarshaller.MarshalCalls(func(haproxyConf models.HAProxyConfig) string {
					return marshallerContent
				})
			})

			AfterEach(func() {
				err := os.Remove(generatedHaproxyCfgFile)
				Expect(err).ShouldNot(HaveOccurred())

				Expect(utils.FileExists(haproxyCfgBackupFile)).To(BeTrue())
				err = os.Remove(haproxyCfgBackupFile)
				Expect(err).ShouldNot(HaveOccurred())
			})

			Context("when Configure is called once", func() {
				It("writes contents to file", func() {
					err = haproxyConfigurer.Configure(routingTable)
					Expect(err).ToNot(HaveOccurred())

					currentConfigTemplateContent, err = ioutil.ReadFile(generatedHaproxyCfgFile)
					Expect(err).ToNot(HaveOccurred())

					expected := string(originalConfigTemplateContent) + marshallerContent
					Expect(string(currentConfigTemplateContent)).To(Equal(expected))

					Expect(fakeMonitor.StopWatchingCallCount()).To(Equal(1))
					Expect(fakeScriptRunner.RunCallCount()).To(Equal(1))
					Expect(fakeMonitor.StartWatchingCallCount()).To(Equal(1))
				})
			})

			Context("when Configure is called twice", func() {
				It("overwrites the file every time (does not accumulate marshalled contents)", func() {
					err = haproxyConfigurer.Configure(routingTable)
					Expect(err).ToNot(HaveOccurred())

					err = haproxyConfigurer.Configure(routingTable)
					Expect(err).ToNot(HaveOccurred())

					currentConfigTemplateContent, err = ioutil.ReadFile(generatedHaproxyCfgFile)
					Expect(err).ToNot(HaveOccurred())

					// File contains only the most recent copy of marshallerContent
					expected := string(originalConfigTemplateContent) + marshallerContent
					Expect(string(currentConfigTemplateContent)).To(Equal(expected))

					// Restarts after each call, though
					Expect(fakeMonitor.StopWatchingCallCount()).To(Equal(2))
					Expect(fakeScriptRunner.RunCallCount()).To(Equal(2))
					Expect(fakeMonitor.StartWatchingCallCount()).To(Equal(2))
				})
			})
		})
	})
})
