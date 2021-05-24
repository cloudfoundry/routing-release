package messagebus_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	. "github.com/onsi/gomega"
)

var (
	natsPort int
)

func TestMessagebus(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Messagebus Suite")
}

var _ = BeforeSuite(func() {
	natsPort = 40000 + config.GinkgoConfig.ParallelNode
})
