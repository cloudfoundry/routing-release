package utils_test

import (
	"code.cloudfoundry.org/routing-release/gorouter/test_util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	test_util.RunSpecWithHoneyCombReporter(t, "Utils Suite")
}
