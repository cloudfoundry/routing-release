package monitor_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"code.cloudfoundry.org/routing-release/cf-tcp-router/monitor"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/tedsuo/ifrit"
)

var _ = Describe("Monitor", func() {
	var (
		testMonitor monitor.Monitor
		logger      lager.Logger
		process     ifrit.Process
		pidFile     string
		catCmd      *exec.Cmd
	)
	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test")

		// Start long lived process and write PID file
		catCmd = exec.Command("cat")
		err := catCmd.Start()
		Expect(err).ToNot(HaveOccurred())
		pid := catCmd.Process.Pid

		file, err := ioutil.TempFile(os.TempDir(), "test-pid-file")
		Expect(err).ToNot(HaveOccurred())
		_, err = file.WriteString(fmt.Sprintf("%d", pid))
		Expect(err).ToNot(HaveOccurred())
		defer file.Close()

		pidFile = file.Name()

		testMonitor = monitor.New(pidFile, logger)

		//signaling that process can be monitored
		testMonitor.StartWatching()
	})

	JustBeforeEach(func() {
		process = ifrit.Invoke(testMonitor)
	})

	AfterEach(func() {
		err := os.Remove(pidFile)
		Expect(err).ToNot(HaveOccurred())

		if catCmd.ProcessState == nil {
			err := catCmd.Process.Kill()
			Expect(err).ToNot(HaveOccurred())
		}
	})

	Context("when there are haproxy PIDs", func() {
		AfterEach(func() {
			process.Signal(os.Interrupt)
			Eventually(process.Wait()).Should(Receive())
			Eventually(logger).Should(gbytes.Say("test.monitor.stopping"))
		})

		It("continues running", func() {
			waitChan := process.Wait()
			Consistently(waitChan, "3s").ShouldNot(Receive())
		})

		Context("when haproxy PIDs go away", func() {
			It("exits non-zero exit code", func() {
				waitChan := process.Wait()
				Consistently(waitChan, "3s").ShouldNot(Receive())

				err := catCmd.Process.Kill()
				Expect(err).ToNot(HaveOccurred())
				catCmd.Wait()

				err = <-process.Wait()

				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("when haproxy is reloading", func() {
		JustBeforeEach(func() {
			testMonitor.StopWatching()

			err := ioutil.WriteFile(pidFile, []byte(""), 0644)
			Expect(err).ToNot(HaveOccurred())
		})

		It("does not error", func() {
			waitChan := process.Wait()
			Consistently(waitChan, "3s").ShouldNot(Receive())
		})

		Context("when the process reloads", func() {
			JustBeforeEach(func() {
				pid := catCmd.Process.Pid
				err := ioutil.WriteFile(pidFile, []byte(fmt.Sprintf("%d", pid)), 0644)
				Expect(err).ToNot(HaveOccurred())

				testMonitor.StartWatching()
			})

			It("monitors the new pid", func() {
				waitChan := process.Wait()
				Consistently(waitChan, "3s").ShouldNot(Receive())

				err := ioutil.WriteFile(pidFile, []byte(""), 0644)
				Expect(err).ToNot(HaveOccurred())

				Eventually(waitChan, "2s").Should(Receive())
			})
		})
	})

	Context("when there are no haproxy PIDs", func() {
		BeforeEach(func() {
			err := ioutil.WriteFile(pidFile, []byte(""), 0644)
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an err", func() {
			err := <-process.Wait()
			Expect(err).To(HaveOccurred())
		})
	})

})
