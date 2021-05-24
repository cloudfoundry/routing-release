package accesslog_test

import (
	"code.cloudfoundry.org/routing-release/gorouter/test_util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestAccessLog(t *testing.T) {
	RegisterFailHandler(Fail)
	test_util.RunSpecWithHoneyCombReporter(t, "AccessLog Suite")
}
