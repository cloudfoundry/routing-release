package haproxy_test

import (
	. "code.cloudfoundry.org/routing-release/cf-tcp-router/configurer/haproxy"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("CommandRunner", func() {
	var (
		cmdRunner *CommandRunner
		logger    lager.Logger
	)
	BeforeEach(func() {
		logger = lagertest.NewTestLogger("script-runner-test")
	})
	Describe("Run", func() {
		Context("when the underlying script exits successfully", func() {
			BeforeEach(func() {
				cmdRunner = CreateCommandRunner("fixtures/testscript", logger)
			})
			It("runs script successfully", func() {
				err := cmdRunner.Run()
				Expect(err).ToNot(HaveOccurred())
				Expect(logger).Should(gbytes.Say("hello test"))
			})
		})

		Context("when the underlying script does not exist", func() {
			BeforeEach(func() {
				cmdRunner = CreateCommandRunner("fixtures/non-existent-script", logger)
			})
			It("throws error", func() {
				err := cmdRunner.Run()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("no such file or directory"))
			})
		})

		Context("when the underlying script errors", func() {
			BeforeEach(func() {
				cmdRunner = CreateCommandRunner("fixtures/badscript", logger)
			})
			It("throws error", func() {
				err := cmdRunner.Run()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("exit status 1"))
				Expect(logger).Should(gbytes.Say("negative test"))
			})
		})
	})
})
