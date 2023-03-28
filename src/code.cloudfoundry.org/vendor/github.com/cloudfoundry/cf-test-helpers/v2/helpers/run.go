package helpers

import (
	"github.com/cloudfoundry/cf-test-helpers/v2/commandreporter"
	"github.com/cloudfoundry/cf-test-helpers/v2/commandstarter"
	"github.com/onsi/gomega/gexec"
)

func Run(executable string, args ...string) *gexec.Session {
	cmdStarter := commandstarter.NewCommandStarter()
	reporter := commandreporter.NewCommandReporter()

	session, err := cmdStarter.Start(reporter, executable, args...)
	if err != nil {
		panic(err)
	}
	return session
}
