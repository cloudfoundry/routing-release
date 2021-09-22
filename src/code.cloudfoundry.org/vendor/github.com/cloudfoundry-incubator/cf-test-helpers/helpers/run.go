package helpers

import (
	"github.com/cloudfoundry-incubator/cf-test-helpers/commandreporter"
	"github.com/cloudfoundry-incubator/cf-test-helpers/commandstarter"
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
