package main_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

func TestRoutingApiCli(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RoutingApiCli Suite")
}

var path string

var _ = SynchronizedBeforeSuite(func() []byte {
	binaryPath, err := gexec.Build("code.cloudfoundry.org/routing-release/routing-api-cli")
	Expect(err).NotTo(HaveOccurred())
	return []byte(binaryPath)
}, func(data []byte){
	path = string(data)
})
