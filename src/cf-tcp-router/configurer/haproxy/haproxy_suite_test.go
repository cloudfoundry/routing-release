package haproxy_test

import (
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

var (
	logger lager.Logger
)

func TestHaproxy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Haproxy Suite")
}

var _ = BeforeEach(func() {
	logger = lagertest.NewTestLogger("test")
})
