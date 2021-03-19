package main_test

import (
	"testing"

	"github.com/onsi/gomega/gexec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var healthBinary string

var _ = BeforeSuite(func() {
	var err error
	healthBinary, err = gexec.Build("github.com/cf-routing/routehealthparser")
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})

func TestRoutehealthparser(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Routehealthparser Suite")
}
