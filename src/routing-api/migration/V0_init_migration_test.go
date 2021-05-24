package migration_test

import (
	"code.cloudfoundry.org/routing-release/routing-api/cmd/routing-api/testrunner"
	"code.cloudfoundry.org/routing-release/routing-api/db"
	"code.cloudfoundry.org/routing-release/routing-api/migration"
	"code.cloudfoundry.org/routing-release/routing-api/models"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("V0InitMigration", func() {
	var (
		mysqlAllocator testrunner.DbAllocator
		dbClient       db.Client
		sqlDB          *db.SqlDB
		err            error
	)
	BeforeEach(func() {
		mysqlAllocator = testrunner.NewMySQLAllocator()
		sqlCfg, err := mysqlAllocator.Create()
		Expect(err).NotTo(HaveOccurred())

		sqlDB, err = db.NewSqlDB(sqlCfg)
		Expect(err).ToNot(HaveOccurred())
		dbClient = sqlDB.Client
	})

	AfterEach(func() {
		err := mysqlAllocator.Delete()
		Expect(err).ToNot(HaveOccurred())
	})

	Context("when valid sql config is passed", func() {
		var v0Migration *migration.V0InitMigration
		BeforeEach(func() {
			v0Migration = migration.NewV0InitMigration()
		})

		It("should successfully create correct schema and does not close db connection", func() {
			err = v0Migration.Run(sqlDB)
			Expect(err).ToNot(HaveOccurred())

			Expect(dbClient.HasTable(&models.RouterGroupDB{})).To(BeTrue())
			Expect(dbClient.HasTable(&models.TcpRouteMapping{})).To(BeTrue())
			Expect(dbClient.HasTable(&models.Route{})).To(BeTrue())
		})
	})
})
