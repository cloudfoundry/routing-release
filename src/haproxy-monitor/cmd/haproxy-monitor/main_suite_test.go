package main_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

var (
	monitorBinPath string
)

func TestHaproxyMonitor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HaproxyMonitor Suite")
}

var _ = SynchronizedBeforeSuite(
	func() []byte {
		monitorBin, err := gexec.Build("haproxy-monitor/cmd/haproxy-monitor", "-race")
		Expect(err).NotTo(HaveOccurred())
		return []byte(monitorBin)
	},
	func(monitorBin []byte) {
		monitorBinPath = string(monitorBin)
		SetDefaultEventuallyTimeout(15 * time.Second)
	},
)

var _ = SynchronizedAfterSuite(func() {}, func() {
	gexec.CleanupBuildArtifacts()
})
