package watcher_test

import (
	"os/exec"

	"haproxy-monitor/watcher"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Watcher", func() {
	Describe("Running", func() {
		Context("when pid is still running", func() {
			var (
				catCmd *exec.Cmd
				pid    int
			)
			BeforeEach(func() {
				catCmd = exec.Command("cat")
				err := catCmd.Start()
				Expect(err).ToNot(HaveOccurred())
				pid = catCmd.Process.Pid
			})

			AfterEach(func() {
				if catCmd.ProcessState == nil {
					err := catCmd.Process.Kill()
					Expect(err).ToNot(HaveOccurred())
				}
			})

			It("returns true", func() {
				Expect(watcher.Running(pid)).To(BeTrue())
			})
		})

		Context("when process for pid is killed", func() {
			var (
				pid int
			)

			BeforeEach(func() {
				catCmd := exec.Command("cat")
				err := catCmd.Start()
				Expect(err).ToNot(HaveOccurred())
				pid = catCmd.Process.Pid
				err = catCmd.Process.Kill()
				Expect(err).ToNot(HaveOccurred())
				catCmd.Wait()
			})

			It("returns false", func() {
				Eventually(watcher.Running(pid)).Should(BeFalse())
			})
		})

		Context("when pid does not exist", func() {
			It("returns false", func() {
				Expect(watcher.Running(65535)).To(BeFalse())
			})
		})
	})
})
