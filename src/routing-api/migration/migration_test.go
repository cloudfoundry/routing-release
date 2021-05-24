package migration_test

import (
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/routing-release/routing-api/cmd/routing-api/testrunner"
	"code.cloudfoundry.org/routing-release/routing-api/db"
	"code.cloudfoundry.org/routing-release/routing-api/migration"
	"code.cloudfoundry.org/routing-release/routing-api/migration/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Migration", func() {
	var (
		sqlDB                 *db.SqlDB
		allocator             testrunner.DbAllocator
		fakeMigration         *fakes.FakeMigration
		fakeLastMigration     *fakes.FakeMigration
		migrations            []migration.Migration
		lastMigrationVersion  int = 10
		firstMigrationVersion int = 1
		logger                lager.Logger
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("test-logger")
		fakeMigration = new(fakes.FakeMigration)
		fakeLastMigration = new(fakes.FakeMigration)

		fakeMigration.VersionReturns(firstMigrationVersion)
		fakeLastMigration.VersionReturns(lastMigrationVersion)
		migrations = []migration.Migration{}
		migrations = append(migrations, fakeMigration)
		migrations = append(migrations, fakeLastMigration)
	})

	TestMigrations := func() {
		Describe("InitializeMigrations", func() {
			It("initializes all possible migrations", func() {
				done := make(chan struct{})
				defer close(done)
				migrations := migration.InitializeMigrations()
				Expect(migrations).To(HaveLen(5))

				Expect(migrations[0]).To(BeAssignableToTypeOf(new(migration.V0InitMigration)))
				Expect(migrations[1]).To(BeAssignableToTypeOf(new(migration.V2UpdateRgMigration)))
				Expect(migrations[2]).To(BeAssignableToTypeOf(new(migration.V3UpdateTcpRouteMigration)))
				Expect(migrations[3]).To(BeAssignableToTypeOf(new(migration.V4AddRgUniqIdxTCPRoute)))
				Expect(migrations[4]).To(BeAssignableToTypeOf(new(migration.V5SniHostnameMigration)))
			})
		})

		It("runs all migrations", func() {
			err := migration.RunAllMigration(sqlDB, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		Describe("RunMigrations", func() {
			Context("when no SqlDB exists", func() {
				It("should be a no-op", func() {
					err := migration.RunMigrations(nil, migrations, logger)
					Expect(err).ToNot(HaveOccurred())
					Expect(fakeMigration.RunCallCount()).To(Equal(0))
					Expect(fakeLastMigration.RunCallCount()).To(Equal(0))
				})
			})

			Context("when no migration table exists", func() {
				It("should create the migration table and set the target version to last migration version", func() {
					err := migration.RunMigrations(sqlDB, migrations, logger)
					Expect(err).ToNot(HaveOccurred())
					Expect(sqlDB.Client.HasTable(&migration.MigrationData{})).To(BeTrue())

					var migrationVersions []migration.MigrationData
					err = sqlDB.Client.Find(&migrationVersions)
					Expect(err).ToNot(HaveOccurred())

					Expect(migrationVersions).To(HaveLen(1))

					migrationVersion := migrationVersions[0]
					Expect(migrationVersion.MigrationKey).To(Equal(migration.MigrationKey))
					Expect(migrationVersion.CurrentVersion).To(Equal(lastMigrationVersion))
					Expect(migrationVersion.TargetVersion).To(Equal(lastMigrationVersion))
				})

				It("should run all the migrations up to the current version", func() {
					err := migration.RunMigrations(sqlDB, migrations, logger)
					Expect(err).ToNot(HaveOccurred())
					Expect(fakeMigration.RunCallCount()).To(Equal(1))
					Expect(fakeLastMigration.RunCallCount()).To(Equal(1))
				})
			})

			Context("when a migration table exists", func() {
				BeforeEach(func() {
					err := sqlDB.Client.AutoMigrate(&migration.MigrationData{})
					Expect(err).ToNot(HaveOccurred())
				})

				Context("when a migration is necessary", func() {
					Context("when another routing-api has already started the migration", func() {
						BeforeEach(func() {
							migrationData := &migration.MigrationData{
								MigrationKey:   migration.MigrationKey,
								CurrentVersion: -1,
								TargetVersion:  lastMigrationVersion,
							}

							_, err := sqlDB.Client.Create(migrationData)
							Expect(err).ToNot(HaveOccurred())
						})

						It("should not update the migration data", func() {
							err := migration.RunMigrations(sqlDB, migrations, logger)
							Expect(err).ToNot(HaveOccurred())

							var migrationVersions []migration.MigrationData
							err = sqlDB.Client.Find(&migrationVersions)
							Expect(err).ToNot(HaveOccurred())

							Expect(migrationVersions).To(HaveLen(1))

							migrationVersion := migrationVersions[0]
							Expect(migrationVersion.MigrationKey).To(Equal(migration.MigrationKey))
							Expect(migrationVersion.CurrentVersion).To(Equal(-1))
							Expect(migrationVersion.TargetVersion).To(Equal(lastMigrationVersion))
						})

						It("should not run any migrations", func() {
							err := migration.RunMigrations(sqlDB, migrations, logger)
							Expect(err).ToNot(HaveOccurred())

							Expect(fakeMigration.RunCallCount()).To(BeZero())
						})
					})

					Context("when the migration has not been started", func() {
						BeforeEach(func() {
							migrationData := &migration.MigrationData{
								MigrationKey:   migration.MigrationKey,
								CurrentVersion: 1,
								TargetVersion:  1,
							}

							_, err := sqlDB.Client.Create(migrationData)
							Expect(err).ToNot(HaveOccurred())
						})

						It("should update the migration data with the target version", func() {
							err := migration.RunMigrations(sqlDB, migrations, logger)
							Expect(err).ToNot(HaveOccurred())

							var migrationVersions []migration.MigrationData
							err = sqlDB.Client.Find(&migrationVersions)
							Expect(err).ToNot(HaveOccurred())

							Expect(migrationVersions).To(HaveLen(1))

							migrationVersion := migrationVersions[0]
							Expect(migrationVersion.MigrationKey).To(Equal(migration.MigrationKey))
							Expect(migrationVersion.CurrentVersion).To(Equal(lastMigrationVersion))
							Expect(migrationVersion.TargetVersion).To(Equal(lastMigrationVersion))
						})

						It("should run all the migrations up to the current version", func() {
							err := migration.RunMigrations(sqlDB, migrations, logger)
							Expect(err).ToNot(HaveOccurred())
							Expect(fakeMigration.RunCallCount()).To(Equal(0))
							Expect(fakeLastMigration.RunCallCount()).To(Equal(1))
						})
					})
				})

				Context("when a migration is unnecessary", func() {
					BeforeEach(func() {
						migrationData := &migration.MigrationData{
							MigrationKey:   migration.MigrationKey,
							CurrentVersion: lastMigrationVersion,
							TargetVersion:  lastMigrationVersion,
						}

						_, err := sqlDB.Client.Create(migrationData)
						Expect(err).ToNot(HaveOccurred())
					})

					It("should not update the migration data", func() {
						err := migration.RunMigrations(sqlDB, migrations, logger)
						Expect(err).ToNot(HaveOccurred())

						var migrationVersions []migration.MigrationData
						err = sqlDB.Client.Find(&migrationVersions)
						Expect(err).ToNot(HaveOccurred())

						Expect(migrationVersions).To(HaveLen(1))

						migrationVersion := migrationVersions[0]
						Expect(migrationVersion.MigrationKey).To(Equal(migration.MigrationKey))
						Expect(migrationVersion.CurrentVersion).To(Equal(lastMigrationVersion))
						Expect(migrationVersion.TargetVersion).To(Equal(lastMigrationVersion))
					})

					It("should not run any migrations", func() {
						err := migration.RunMigrations(sqlDB, migrations, logger)
						Expect(err).ToNot(HaveOccurred())

						Expect(fakeMigration.RunCallCount()).To(BeZero())
						Expect(fakeLastMigration.RunCallCount()).To(BeZero())
					})
				})
			})
		})
	}

	Describe("Test with Mysql", func() {
		BeforeEach(func() {
			allocator = testrunner.NewMySQLAllocator()
			sqlCfg, err := allocator.Create()
			Expect(err).ToNot(HaveOccurred())

			sqlDB, err = db.NewSqlDB(sqlCfg)
			Expect(err).ToNot(HaveOccurred())
			err = migration.NewV0InitMigration().Run(sqlDB)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			err := allocator.Delete()
			Expect(err).ToNot(HaveOccurred())
		})

		TestMigrations()
	})

	Describe("Test with Postgres", func() {
		BeforeEach(func() {
			allocator = testrunner.NewPostgresAllocator()
			sqlCfg, err := allocator.Create()
			Expect(err).ToNot(HaveOccurred())

			sqlDB, err = db.NewSqlDB(sqlCfg)
			Expect(err).ToNot(HaveOccurred())
			err = migration.NewV0InitMigration().Run(sqlDB)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			err := allocator.Delete()
			Expect(err).ToNot(HaveOccurred())
		})

		TestMigrations()
	})
})
