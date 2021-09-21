package internal

import (
	"github.com/cloudfoundry-incubator/cf-test-helpers/commandreporter"
	"github.com/onsi/gomega/gexec"
)

func Cf(cmdStarter Starter, args ...string) *gexec.Session {
	return CfWithCustomReporter(cmdStarter, commandreporter.NewCommandReporter(), args...)
}

func CfWithCustomReporter(cmdStarter Starter, reporter Reporter, args ...string) *gexec.Session {
	request, err := cmdStarter.Start(reporter, "cf", args...)
	if err != nil {
		panic(err)
	}

	return request
}
