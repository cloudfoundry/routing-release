package postselection_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPostselection(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Postselection Suite")
}
