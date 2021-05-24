package syncer_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestSyncer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Syncer Suite")
}
