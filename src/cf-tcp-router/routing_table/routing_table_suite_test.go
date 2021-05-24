package routing_table_test

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

func TestRoutingTable(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RoutingTable Suite")
}

var _ = BeforeEach(func() {
	logger = lagertest.NewTestLogger("test")
})
