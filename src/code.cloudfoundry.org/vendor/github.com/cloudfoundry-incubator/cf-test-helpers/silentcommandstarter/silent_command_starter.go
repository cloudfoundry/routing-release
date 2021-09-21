package silentcommandstarter

import (
	"os/exec"
	"time"

	"github.com/cloudfoundry-incubator/cf-test-helpers/internal"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega/gexec"
)

type CommandStarter struct {
}

func NewCommandStarter() *CommandStarter {
	return &CommandStarter{}
}

func (r *CommandStarter) Start(reporter internal.Reporter, executable string, args ...string) (*gexec.Session, error) {
	cmd := exec.Command(executable, args...)
	reporter.Report(time.Now(), cmd)

	writer := ginkgo.GinkgoWriter
	writer.Write([]byte("SILENCING COMMAND OUTPUT"))

	return gexec.Start(cmd, nil, nil)
}
