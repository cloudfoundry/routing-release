package cf

import (
	"github.com/cloudfoundry-incubator/cf-test-helpers/commandstarter"
	"github.com/cloudfoundry-incubator/cf-test-helpers/internal"
	"github.com/cloudfoundry-incubator/cf-test-helpers/silentcommandstarter"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega/gexec"
)

var Cf = func(args ...string) *gexec.Session {
	cmdStarter := commandstarter.NewCommandStarter()
	return internal.Cf(cmdStarter, args...)
}

var CfRedact = func(stringToRedact string, args ...string) *gexec.Session {
	var (
		redactor          internal.Redactor
		redactingReporter internal.Reporter
	)
	cmdStarter := silentcommandstarter.NewCommandStarter()
	redactor = internal.NewRedactor(stringToRedact)
	redactingReporter = internal.NewRedactingReporter(ginkgo.GinkgoWriter, redactor)

	return internal.CfWithCustomReporter(cmdStarter, redactingReporter, args...)
}
