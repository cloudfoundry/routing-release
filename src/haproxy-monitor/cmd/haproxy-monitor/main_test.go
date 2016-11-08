package main_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"syscall"

	"haproxy-monitor/cmd/haproxy-monitor/testrunner"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("HaproxyMonitor", func() {
	var (
		session     *Session
		monitorArgs testrunner.Args
	)

	JustBeforeEach(func() {
		var err error
		session, err = Start(exec.Command(monitorBinPath, monitorArgs.ArgSlice()...), GinkgoWriter, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if session != nil {
			session.Kill()
		}
		monitorArgs = testrunner.Args{}
	})

	Context("when there are haproxy PIDs", func() {
		var (
			catCmd  *exec.Cmd
			pidFile string
			pidFd   int
		)

		BeforeEach(func() {
			// Start long lived process and write PID file
			catCmd = exec.Command("cat")
			err := catCmd.Start()
			Expect(err).ToNot(HaveOccurred())
			pid := catCmd.Process.Pid

			file, err := ioutil.TempFile(os.TempDir(), "test-pid-file")
			Expect(err).ToNot(HaveOccurred())
			_, err = file.WriteString(fmt.Sprintf("%d", pid))
			Expect(err).ToNot(HaveOccurred())

			pidFile = file.Name()
			pidFd = int(file.Fd())
			monitorArgs.PidFile = pidFile
		})

		AfterEach(func() {
			err := os.Remove(pidFile)
			Expect(err).ToNot(HaveOccurred())
			if catCmd.ProcessState == nil {
				err := catCmd.Process.Kill()
				Expect(err).ToNot(HaveOccurred())
			}
		})

		It("continues running", func() {
			Consistently(session, "3s").ShouldNot(Exit())
		})

		Context("when the PID inside the pid file changes", func() {
			var newPid int
			BeforeEach(func() {
				newCmd := exec.Command("cat")
				err := newCmd.Start()
				Expect(err).ToNot(HaveOccurred())
				newPid = newCmd.Process.Pid
			})

			It("does not block on flock syscall", func() {
				err := syscall.Flock(pidFd, syscall.LOCK_EX)
				Expect(err).ToNot(HaveOccurred())

				err = ioutil.WriteFile(pidFile, []byte(""), os.ModePerm)
				Expect(err).ToNot(HaveOccurred())
				Eventually(session, "3s").Should(Exit())

				err = syscall.Flock(pidFd, syscall.LOCK_UN)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when haproxy PIDs go away", func() {
			It("exits non-zero exit code", func() {
				Consistently(session, "3s").ShouldNot(Exit())

				err := catCmd.Process.Kill()
				Expect(err).ToNot(HaveOccurred())
				catCmd.Wait()

				Eventually(session).Should(Exit(1))
			})
		})
	})

	Context("when a PID file is not provided", func() {
		It("exits non-zero exit code", func() {
			Eventually(session).Should(Exit(1))
			Expect(session.Out).To(gbytes.Say("pidfile-not-found"))
		})
	})

	Context("when there are no haproxy PIDs", func() {
		var (
			pidFile string
		)

		BeforeEach(func() {
			file, err := ioutil.TempFile(os.TempDir(), "test-pid-file")
			Expect(err).ToNot(HaveOccurred())

			pidFile = file.Name()
			monitorArgs.PidFile = pidFile
		})

		AfterEach(func() {
			err := os.Remove(pidFile)
			Expect(err).ToNot(HaveOccurred())
		})

		It("exits non-zero exit code", func() {
			Eventually(session).Should(Exit(1))
		})
	})
})
