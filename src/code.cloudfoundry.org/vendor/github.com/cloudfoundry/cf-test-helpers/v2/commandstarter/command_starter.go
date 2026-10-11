package commandstarter

import (
	"io"
	"os/exec"
	"time"

	"github.com/cloudfoundry/cf-test-helpers/v2/internal"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega/gexec"
)

type CommandStarter struct {
	stdin io.Reader
}

func NewCommandStarter() *CommandStarter {
	return &CommandStarter{}
}

func NewCommandStarterWithStdin(stdin io.Reader) *CommandStarter {
	return &CommandStarter{
		stdin: stdin,
	}
}

func (r *CommandStarter) Start(reporter internal.Reporter, executable string, args ...string) (*gexec.Session, error) {
	cmd := exec.Command(executable, args...)
	cmd.Stdin = r.stdin
	startTime := time.Now()
	reporter.Report(startTime, cmd)

	session, err := gexec.Start(cmd, ginkgo.GinkgoWriter, ginkgo.GinkgoWriter)
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
