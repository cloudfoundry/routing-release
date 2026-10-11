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
	startTime := time.Now()
	reporter.Report(startTime, cmd)

	_, err := ginkgo.GinkgoWriter.Write([]byte("SILENCING COMMAND OUTPUT"))
	if err != nil {
		return nil, err
	}

	session, err := gexec.Start(cmd, nil, nil)
	if err != nil {
		return session, err
	}

	if cr, ok := reporter.(internal.CompletionReporter); ok {
		go func() {
			<-session.Exited
			cr.ReportCompletion(cmd, time.Since(startTime), session.ExitCode())
		}()
	}

	return session, nil
}
