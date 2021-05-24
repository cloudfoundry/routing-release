package varz_test

import (
	"code.cloudfoundry.org/routing-release/gorouter/test_util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestVarz(t *testing.T) {
	RegisterFailHandler(Fail)
	test_util.RunSpecWithHoneyCombReporter(t, "Varz Suite")
}
