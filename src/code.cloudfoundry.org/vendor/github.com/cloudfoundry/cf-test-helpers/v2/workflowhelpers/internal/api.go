package internal

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/cloudfoundry/cf-test-helpers/v2/commandreporter"
	"github.com/cloudfoundry/cf-test-helpers/v2/internal"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func ApiRequest(cmdStarter internal.Starter, method, endpoint string, response interface{}, timeout time.Duration, data ...string) {
	args := []string{
		"curl",
		endpoint,
		"-X", method,
	}

	dataArg := strings.Join(data, "")
	if len(dataArg) > 0 {
		args = append(args, "-d", dataArg)
	}

	reporter := internal.ObservedReporter(commandreporter.NewCommandReporter())
	request, err := cmdStarter.Start(reporter, "cf", args...)
	gomega.ExpectWithOffset(2, err).NotTo(gomega.HaveOccurred())

	request.Wait(timeout)
	gomega.ExpectWithOffset(2, request).To(gexec.Exit(0))

	if response != nil {
		err := json.Unmarshal(request.Out.Contents(), response)
		gomega.ExpectWithOffset(2, err).ToNot(gomega.HaveOccurred())
	}
}
