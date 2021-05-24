package migration_test

import (
	"time"

	"code.cloudfoundry.org/routing-release/routing-api/cmd/routing-api/testrunner"
	"code.cloudfoundry.org/routing-release/routing-api/db"
	"code.cloudfoundry.org/routing-release/routing-api/migration"
	"code.cloudfoundry.org/routing-release/routing-api/migration/v0"
	"code.cloudfoundry.org/routing-release/routing-api/models"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("V4AddRgUniqIdxTCPRouteMigration", func() {
	var (
		sqlDB          *db.SqlDB
		mysqlAllocator testrunner.DbAllocator
	)

	BeforeEach(func() {
		mysqlAllocator = testrunner.NewMySQLAllocator()
		sqlCfg, err := mysqlAllocator.Create()
		Expect(err).NotTo(HaveOccurred())

		sqlDB, err = db.NewSqlDB(sqlCfg)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		err := mysqlAllocator.Delete()
		Expect(err).ToNot(HaveOccurred())
	})

	runTests := func() {
		Context("when no routes exist", func() {
			BeforeEach(func() {
				v4Migration := migration.NewV4AddRgUniqIdxTCPRouteMigration()
				err := v4Migration.Run(sqlDB)
				Expect(err).ToNot(HaveOccurred())

				tcpRoutes, err := sqlDB.ReadTcpRouteMappings()
				Expect(err).NotTo(HaveOccurred())
				Expect(tcpRoutes).To(HaveLen(0))
			})

			It("allows multiple TCP routes with different router group guids", func() {
				tcpRoute1 := models.TcpRouteMapping{
					Model:     models.Model{Guid: "guid-1"},
					ExpiresAt: time.Now().Add(1 * time.Hour),
					TcpMappingEntity: models.TcpMappingEntity{
						RouterGroupGuid: "test1",
						HostPort:        80,
						HostIP:          "1.2.3.4",
						ExternalPort:    80,
					},
				}

				tcpRoute2 := models.TcpRouteMapping{
					Model:     models.Model{Guid: "guid-2"},
					ExpiresAt: time.Now().Add(1 * time.Hour),
					TcpMappingEntity: models.TcpMappingEntity{
						RouterGroupGuid: "test2",
						HostPort:        80,
						HostIP:          "1.2.3.4",
						ExternalPort:    80,
					},
				}

				_, err := sqlDB.Client.Create(&tcpRoute1)
				Expect(err).NotTo(HaveOccurred())

				_, err = sqlDB.Client.Create(&tcpRoute2)
				Expect(err).NotTo(HaveOccurred())

				tcpRoutes, err := sqlDB.ReadTcpRouteMappings()
				Expect(err).NotTo(HaveOccurred())
				Expect(tcpRoutes).To(HaveLen(2))
			})
		})

		Context("when there are existing records", func() {
			BeforeEach(func() {
				tcpRoute1 := models.TcpRouteMapping{
					Model:     models.Model{Guid: "guid-1"},
					ExpiresAt: time.Now().Add(1 * time.Hour),
					TcpMappingEntity: models.TcpMappingEntity{
						RouterGroupGuid: "test1",
						HostPort:        80,
						HostIP:          "1.2.3.4",
						ExternalPort:    80,
					},
				}
				_, err := sqlDB.Client.Create(&tcpRoute1)
				Expect(err).NotTo(HaveOccurred())
			})

			It("allows adding the same TCP routes with different router group GUIDs", func() {
				v4Migration := migration.NewV4AddRgUniqIdxTCPRouteMigration()
				err := v4Migration.Run(sqlDB)
				Expect(err).NotTo(HaveOccurred())

				tcpRoute2 := models.TcpRouteMapping{
					Model:     models.Model{Guid: "guid-2"},
					ExpiresAt: time.Now().Add(1 * time.Hour),
					TcpMappingEntity: models.TcpMappingEntity{
						RouterGroupGuid: "test2",
						HostPort:        80,
						HostIP:          "1.2.3.4",
						ExternalPort:    80,
					},
				}
				_, err = sqlDB.Client.Create(&tcpRoute2)
				Expect(err).NotTo(HaveOccurred())

				routes, err := sqlDB.ReadTcpRouteMappings()
				Expect(err).NotTo(HaveOccurred())
				Expect(routes).To(HaveLen(2))
			})

			Context("when the routes have unique ports", func() {
				BeforeEach(func() {
					tcpRoute2 := models.TcpRouteMapping{
						Model:     models.Model{Guid: "guid-2"},
						ExpiresAt: time.Now().Add(1 * time.Hour),
						TcpMappingEntity: models.TcpMappingEntity{
							RouterGroupGuid: "test2",
							HostPort:        53,
							HostIP:          "1.2.3.4",
							ExternalPort:    53,
						},
					}
					_, err := sqlDB.Client.Create(&tcpRoute2)
					Expect(err).NotTo(HaveOccurred())
				})
				It("should successfully migrate", func() {
					v4Migration := migration.NewV4AddRgUniqIdxTCPRouteMigration()
					err := v4Migration.Run(sqlDB)
					Expect(err).NotTo(HaveOccurred())
					routes, err := sqlDB.ReadTcpRouteMappings()
					Expect(err).NotTo(HaveOccurred())
					Expect(routes).To(HaveLen(2))
				})
			})
		})
	}

	Describe("Version", func() {
		It("returns 4 for the version", func() {
			v4Migration := migration.NewV4AddRgUniqIdxTCPRouteMigration()
			Expect(v4Migration.Version()).To(Equal(4))
		})
	})

	Describe("Run", func() {
		Context("when there are existing tables with the old tcp_route model", func() {
			BeforeEach(func() {
				err := sqlDB.Client.AutoMigrate(&v0.RouterGroupDB{}, &v0.TcpRouteMapping{}, &v0.Route{})
				Expect(err).ToNot(HaveOccurred())
				v3Migration := migration.NewV3UpdateTcpRouteMigration()
				err = v3Migration.Run(sqlDB)
				Expect(err).ToNot(HaveOccurred())
			})
			runTests()
		})

		Context("when the tables are newly created (by V0 init migration)", func() {
			BeforeEach(func() {
				v0Migration := migration.NewV0InitMigration()
				err := v0Migration.Run(sqlDB)
				Expect(err).ToNot(HaveOccurred())
			})
			runTests()
		})
	})
})
