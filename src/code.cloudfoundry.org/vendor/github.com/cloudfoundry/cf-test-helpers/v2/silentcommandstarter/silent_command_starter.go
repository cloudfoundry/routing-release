package silentcommandstarter

import (
	"os/exec"
	"time"

	"github.com/cloudfoundry/cf-test-helpers/v2/internal"
	"github.com/onsi/ginkgo/v2"
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

	_, err := ginkgo.GinkgoWriter.Write([]byte("SILENCING COMMAND OUTPUT"))
	if err != nil {
		return nil, err
	}

	return gexec.Start(cmd, nil, nil)
}
