package db_test

import (
	"testing"

	"code.cloudfoundry.org/routing-release/routing-api/cmd/routing-api/testrunner"
	"code.cloudfoundry.org/routing-release/routing-api/config"
	_ "github.com/lib/pq"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	mysqlCfg          *config.SqlDB
	postgresCfg       *config.SqlDB
	mysqlAllocator    testrunner.DbAllocator
	postgresAllocator testrunner.DbAllocator
)

func TestDB(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DB Suite")
}

var _ = BeforeSuite(func() {
	var err error

	postgresAllocator = testrunner.NewPostgresAllocator()
	postgresCfg, err = postgresAllocator.Create()
	Expect(err).ToNot(HaveOccurred(), "error occurred starting postgres client, is postgres running?")
	mysqlAllocator = testrunner.NewMySQLAllocator()
	mysqlCfg, err = mysqlAllocator.Create()
	Expect(err).ToNot(HaveOccurred(), "error occurred starting mysql client, is mysql running?")
})

var _ = AfterSuite(func() {
	err := mysqlAllocator.Delete()
	Expect(err).ToNot(HaveOccurred())

	err = postgresAllocator.Delete()
	Expect(err).ToNot(HaveOccurred())
})

var _ = BeforeEach(func() {
	err := mysqlAllocator.Reset()
	Expect(err).ToNot(HaveOccurred())
	err = postgresAllocator.Reset()
	Expect(err).ToNot(HaveOccurred())
})
