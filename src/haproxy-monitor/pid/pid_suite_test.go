package pid_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestPid(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Pid Suite")
}
