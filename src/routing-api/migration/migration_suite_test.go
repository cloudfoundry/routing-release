package migration_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestMigration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Migration Suite")
}
