package routingapi_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestRoutingapi(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Routing API Suite")
}
