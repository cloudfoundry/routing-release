package router_group_port_checker_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestRouterGroupPortChecker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RouterGroupPortChecker Suite")
}
