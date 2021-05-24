package db_test

import (
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"code.cloudfoundry.org/routing-release/routing-api/config"
	"code.cloudfoundry.org/routing-release/routing-api/db"
	"code.cloudfoundry.org/routing-release/routing-api/db/fakes"
	"code.cloudfoundry.org/routing-release/routing-api/matchers"
	"code.cloudfoundry.org/routing-release/routing-api/migration"
	"code.cloudfoundry.org/routing-release/routing-api/models"
	uuid "github.com/nu7hatch/gouuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("SqlDB", func() {

	var (
		sqlCfg *config.SqlDB
		sqlDB  *db.SqlDB
	)

	PostgresConnectionString := func() {
		Describe("When db type is postgres", func() {
			var cfg *config.SqlDB
			BeforeEach(func() {
				cfg = &config.SqlDB{
					Username: "root",
					Password: "",
					Host:     "localhost",
					Port:     5432,
					Type:     "postgres",
					Schema:   "testDB",
				}
			})

			AfterEach(func() {
				cfg = nil
			})

			Context("when no CACert is provided", func() {
				Context("when SkipSSLValidation is true", func() {
					BeforeEach(func() {
						cfg.SkipSSLValidation = true
					})
					It("returns a correct connection string with 'sslmode=disable'", func() {
						connStr, err := db.ConnectionString(cfg)
						connectionString := fmt.Sprintf(
							"postgres://%s:%s@%s:%d/%s?sslmode=disable",
							cfg.Username,
							cfg.Password,
							cfg.Host,
							cfg.Port,
							cfg.Schema,
						)
						Expect(err).ToNot(HaveOccurred())
						Expect(connStr).To(Equal(connectionString))
					})
				})
				Context("when SkipSSLValidation is false", func() {
					BeforeEach(func() {
						cfg.SkipSSLValidation = false
					})
					It("returns a correct connection string with 'sslmode=disable", func() {
						connStr, err := db.ConnectionString(cfg)
						connectionString := fmt.Sprintf(
							"postgres://%s:%s@%s:%d/%s?sslmode=disable",
							cfg.Username,
							cfg.Password,
							cfg.Host,
							cfg.Port,
							cfg.Schema,
						)
						Expect(err).ToNot(HaveOccurred())
						Expect(connStr).To(Equal(connectionString))
					})
				})
			})

			Context("when CACert is provided", func() {
				BeforeEach(func() {
					cfg.CACert = "testCA"
				})
				Context("when SkipSSLValidation is true", func() {
					BeforeEach(func() {
						cfg.SkipSSLValidation = true
					})
					It("returns a correct connection string with 'sslmode=require'", func() {
						connStr, err := db.ConnectionString(cfg)
						connectionString := fmt.Sprintf(
							"postgres://%s:%s@%s:%d/%s?sslmode=require",
							cfg.Username,
							cfg.Password,
							cfg.Host,
							cfg.Port,
							cfg.Schema,
						)
						Expect(err).ToNot(HaveOccurred())
						Expect(connStr).To(Equal(connectionString))
					})
				})
				Context("when SkipSSLValidation is false", func() {
					BeforeEach(func() {
						cfg.SkipSSLValidation = false
					})
					It("returns a correct connection string with 'sslmode=verify-full&sslrootcert=/some/path/postgres_cert.pem", func() {
						connStr, err := db.ConnectionString(cfg)
						connectionString := fmt.Sprintf(
							`postgres://%s:%s@%s:%d/%s\?sslmode=verify-full&sslrootcert=.*/postgres_cert\.pem`,
							cfg.Username,
							cfg.Password,
							cfg.Host,
							cfg.Port,
							cfg.Schema,
						)
						Expect(err).ToNot(HaveOccurred())
						Expect(connStr).To(MatchRegexp(connectionString))
					})
				})
			})
		})
	}

	MySQLConnectionString := func() {
		Describe("When db type is mySQL", func() {
			var cfg *config.SqlDB
			BeforeEach(func() {
				cfg = &config.SqlDB{
					Username: "root",
					Password: "password",
					Host:     "localhost",
					Port:     3306,
					Type:     "mysql",
					Schema:   "testDB",
				}
			})

			AfterEach(func() {
				cfg = nil
			})

			Context("when no CACert is provided", func() {
				Context("when SkipSSLValidation is true", func() {
					BeforeEach(func() {
						cfg.SkipSSLValidation = true
					})
					It("returns a correct connection string with 'tls=dbTLSSkipVerify'", func() {
						connStr, err := db.ConnectionString(cfg)
						configKey := "dbTLSSkipVerify"
						connectionString := fmt.Sprintf(
							"%s:%s@tcp(%s:%d)/%s?parseTime=true&tls=%s",
							cfg.Username,
							cfg.Password,
							cfg.Host,
							cfg.Port,
							cfg.Schema,
							configKey,
						)
						Expect(err).ToNot(HaveOccurred())
						Expect(connStr).To(Equal(connectionString))
					})
				})
				Context("when SkipSSLValidation is false", func() {
					BeforeEach(func() {
						cfg.SkipSSLValidation = false
					})
					It("returns a correct connection string without 'tls' param", func() {
						connStr, err := db.ConnectionString(cfg)

						connectionString := fmt.Sprintf(
							"%s:%s@tcp(%s:%d)/%s?parseTime=true",
							cfg.Username,
							cfg.Password,
							cfg.Host,
							cfg.Port,
							cfg.Schema,
						)
						Expect(err).ToNot(HaveOccurred())
						Expect(connStr).ToNot(ContainSubstring("tls"))
						Expect(connStr).To(Equal(connectionString))
					})
				})
			})

			Context("when CACert is provided", func() {
				BeforeEach(func() {
					cfg.CACert = "testCA"
				})
				Context("when SkipSSLValidation is true", func() {
					BeforeEach(func() {
						cfg.SkipSSLValidation = true
					})
					It("returns a correct connection string with 'tls=dbTLSSkipVerify'", func() {
						connStr, err := db.ConnectionString(cfg)
						configKey := "dbTLSSkipVerify"
						connectionString := fmt.Sprintf(
							"%s:%s@tcp(%s:%d)/%s?parseTime=true&tls=%s",
							cfg.Username,
							cfg.Password,
							cfg.Host,
							cfg.Port,
							cfg.Schema,
							configKey,
						)
						Expect(err).ToNot(HaveOccurred())
						Expect(connStr).To(Equal(connectionString))
					})
				})
				Context("when SkipSSLValidation is false", func() {
					BeforeEach(func() {
						cfg.SkipSSLValidation = false
					})
					It("returns a correct connection string with 'tls=dbTLSSkipVerify'", func() {
						connStr, err := db.ConnectionString(cfg)
						configKey := "dbTLSCertVerify"
						connectionString := fmt.Sprintf(
							"%s:%s@tcp(%s:%d)/%s?parseTime=true&tls=%s",
							cfg.Username,
							cfg.Password,
							cfg.Host,
							cfg.Port,
							cfg.Schema,
							configKey,
						)
						Expect(err).ToNot(HaveOccurred())
						Expect(connStr).To(Equal(connectionString))
					})
				})
			})
		})
	}

	Connection := func() {
		Describe("Connection", func() {
			var (
				sqlDB db.DB
				err   error
			)

			JustBeforeEach(func() {
				sqlDB, err = db.NewSqlDB(sqlCfg)
			})

			Describe("Locking", func() {
				Context("When reads are locked", func() {
					JustBeforeEach(func() {
						sqlDB.LockRouterGroupReads()
					})
					It("ReadRouterGroups returns an error", func() {
						_, err = sqlDB.ReadRouterGroups()
						Expect(err.Error()).To(ContainSubstring("Database unavailable due to backup or restore"))
					})
					It("ReadRouterGroup returns an error", func() {
						_, err = sqlDB.ReadRouterGroup("foobar")
						Expect(err.Error()).To(ContainSubstring("Database unavailable due to backup or restore"))
					})
					It("ReadRouterGroupByName returns an error", func() {
						_, err = sqlDB.ReadRouterGroupByName("foobar")
						Expect(err.Error()).To(ContainSubstring("Database unavailable due to backup or restore"))
					})
					It("SaveRouterGroup returns an error", func() {
						err = sqlDB.SaveRouterGroup(models.RouterGroup{})
						Expect(err.Error()).To(ContainSubstring("Database unavailable due to backup or restore"))
					})
					It("DeleteRouterGroup returns an error", func() {
						err = sqlDB.DeleteRouterGroup("")
						Expect(err.Error()).To(ContainSubstring("Database unavailable due to backup or restore"))
					})
				})

				Context("When reads are unlocked", func() {
					JustBeforeEach(func() {
						sqlDB.LockRouterGroupReads()
						sqlDB.UnlockRouterGroupReads()
					})
					It("ReadRouterGroups does not return an error", func() {
						_, err = sqlDB.ReadRouterGroups()
						Expect(err).ToNot(HaveOccurred())
					})
					It("ReadRouterGroup does not return an error", func() {
						_, err = sqlDB.ReadRouterGroup("foobar")
						Expect(err).ToNot(HaveOccurred())
					})
					It("ReadRouterGroupByName does not return an error", func() {
						_, err = sqlDB.ReadRouterGroupByName("foobar")
						Expect(err).ToNot(HaveOccurred())
					})
					It("SaveRouterGroup does not return an error", func() {
						err = sqlDB.SaveRouterGroup(models.RouterGroup{Guid: "foo"})
						Expect(err).ToNot(HaveOccurred())
					})
					It("DeleteRouterGroup does not return a backup/restore error", func() {
						err = sqlDB.DeleteRouterGroup("")
						Expect(err.Error()).NotTo(ContainSubstring("Database unavailable due to backup or restore"))
					})
				})

				Context("When writes are locked", func() {
					JustBeforeEach(func() {
						sqlDB.LockRouterGroupWrites()
					})

					It("does not throw read errors", func() {
						_, err = sqlDB.ReadRouterGroups()
						Expect(err).NotTo(HaveOccurred())
					})

					It("SaveRouterGroup returns an error", func() {
						err = sqlDB.SaveRouterGroup(models.RouterGroup{})
						Expect(err.Error()).To(ContainSubstring("Database unavailable due to backup or restore"))
					})

					It("DeleteRouterGroup returns an error", func() {
						err = sqlDB.DeleteRouterGroup("")
						Expect(err.Error()).To(ContainSubstring("Database unavailable due to backup or restore"))
					})
				})

				Context("When writes are unlocked", func() {
					JustBeforeEach(func() {
						sqlDB.LockRouterGroupWrites()
						sqlDB.UnlockRouterGroupWrites()
					})

					It("does not throw read errors", func() {
						_, err = sqlDB.ReadRouterGroups()
						Expect(err).NotTo(HaveOccurred())
					})

					It("SaveRouterGroup does not return an error", func() {
						err = sqlDB.SaveRouterGroup(models.RouterGroup{Guid: "foo"})
						Expect(err).NotTo(HaveOccurred())
					})

					It("DeleteRouterGroup does not return a backup/restore error", func() {
						err = sqlDB.DeleteRouterGroup("")
						Expect(err.Error()).NotTo(ContainSubstring("Database unavailable due to backup or restore"))
					})
				})
			})

			It("returns a sql db client", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(sqlDB).ToNot(BeNil())
			})

			Context("when config is nil", func() {
				It("returns an error", func() {
					failedSqlDB, err := db.NewSqlDB(nil)
					Expect(err).To(HaveOccurred())
					Expect(failedSqlDB).To(BeNil())
				})
			})

			Context("when authentication fails", func() {
				var badAuthConfig config.SqlDB
				BeforeEach(func() {
					badAuthConfig = *sqlCfg
					badAuthConfig.Username = "wrong_username"
					badAuthConfig.Password = "wrong_password"
				})

				It("returns an error", func() {
					failedSqlDB, err := db.NewSqlDB(&badAuthConfig)
					Expect(err).To(HaveOccurred())
					Expect(failedSqlDB).To(BeNil())
				})
			})

			Context("when connecting to SQL DB fails", func() {
				var badPortConfig config.SqlDB
				BeforeEach(func() {
					badPortConfig = *sqlCfg
					badPortConfig.Port = 1234
				})

				It("returns an error", func() {
					failedSqlDB, err := db.NewSqlDB(&badPortConfig)
					Expect(err).To(HaveOccurred())
					Expect(failedSqlDB).To(BeNil())
				})
			})
		})
	}

	ReadRouterGroups := func() {
		Describe("ReadRouterGroups", func() {
			var (
				routerGroups models.RouterGroups
				err          error
				rg           models.RouterGroupDB
			)

			Context("when there are router groups", func() {
				BeforeEach(func() {
					rg = models.RouterGroupDB{
						Model:           models.Model{Guid: newUuid()},
						Name:            "rg-1",
						Type:            "tcp",
						ReservablePorts: "120",
					}
					_, err = sqlDB.Client.Create(&rg)
					Expect(err).ToNot(HaveOccurred())
				})

				AfterEach(func() {
					_, err = sqlDB.Client.Delete(&rg)
					Expect(err).ToNot(HaveOccurred())
				})

				It("returns list of router groups", func() {
					routerGroups, err = sqlDB.ReadRouterGroups()
					Expect(err).ToNot(HaveOccurred())
					Expect(routerGroups).ToNot(BeNil())
					Expect(routerGroups).To(HaveLen(1))
					Expect(routerGroups[0]).Should(matchers.MatchRouterGroup(rg.ToRouterGroup()))
				})
			})

			Context("when there are no router groups", func() {
				BeforeEach(func() {
					_, err = sqlDB.Client.Where("1 = 1").Delete(&models.RouterGroupDB{})
					Expect(err).ToNot(HaveOccurred())
				})

				It("returns an empty slice", func() {
					routerGroups, err = sqlDB.ReadRouterGroups()
					Expect(err).ToNot(HaveOccurred())
					Expect(routerGroups).ToNot(BeNil())
					Expect(routerGroups).To(HaveLen(0))
				})
			})

			Context("when there is a connection error", func() {
				BeforeEach(func() {
					fakeClient := &fakes.FakeClient{}
					fakeClient.FindReturns(errors.New("connection refused"))
					sqlDB.Client = fakeClient
				})

				It("returns an error", func() {
					routerGroups, err = sqlDB.ReadRouterGroups()
					Expect(err).To(HaveOccurred())
				})
			})
		})
	}

	ReadRouterGroupByName := func() {
		Describe("ReadRouterGroupByName", func() {
			Context("When the router group exists", func() {
				BeforeEach(func() {
					rg := models.RouterGroupDB{
						Model:           models.Model{Guid: newUuid()},
						Name:            "other-rg",
						Type:            "tcp",
						ReservablePorts: "120",
					}
					_, err := sqlDB.Client.Create(&rg)
					Expect(err).ToNot(HaveOccurred())

					rg = models.RouterGroupDB{
						Model:           models.Model{Guid: newUuid()},
						Name:            "pineapple",
						Type:            "http",
						ReservablePorts: "",
					}
					_, err = sqlDB.Client.Create(&rg)
					Expect(err).ToNot(HaveOccurred())
				})

				It("returns the router group with that name", func() {
					routerGroup, err := sqlDB.ReadRouterGroupByName("pineapple")
					Expect(err).ToNot(HaveOccurred())
					Expect(routerGroup.Name).To(Equal("pineapple"))
				})
			})

			Context("when the router group doesn't exist", func() {
				BeforeEach(func() {
					rg := models.RouterGroupDB{
						Model:           models.Model{Guid: newUuid()},
						Name:            "other-rg",
						Type:            "tcp",
						ReservablePorts: "120",
					}
					_, err := sqlDB.Client.Create(&rg)
					Expect(err).ToNot(HaveOccurred())
				})

				It("returns an empty struct", func() {
					routerGroup, err := sqlDB.ReadRouterGroupByName("pineapple")
					Expect(err).ToNot(HaveOccurred())
					Expect(routerGroup).To(Equal(models.RouterGroup{}))
				})
			})
		})
	}

	ReadRouterGroup := func() {
		Describe("ReadRouterGroup", func() {
			var (
				routerGroup   models.RouterGroup
				err           error
				rg            models.RouterGroupDB
				routerGroupId string
			)

			JustBeforeEach(func() {
				routerGroup, err = sqlDB.ReadRouterGroup(routerGroupId)
			})

			Context("when router group exists", func() {
				BeforeEach(func() {
					routerGroupId = newUuid()
					rg = models.RouterGroupDB{
						Model:           models.Model{Guid: routerGroupId},
						Name:            "rg-1",
						Type:            "tcp",
						ReservablePorts: "120",
					}
					_, err = sqlDB.Client.Create(&rg)
					Expect(err).ToNot(HaveOccurred())
				})

				AfterEach(func() {
					_, err = sqlDB.Client.Delete(&rg)
					Expect(err).ToNot(HaveOccurred())
				})

				It("returns the router group", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(routerGroup.Guid).To(Equal(rg.Guid))
					Expect(routerGroup.Name).To(Equal(rg.Name))
					Expect(string(routerGroup.ReservablePorts)).To(Equal(rg.ReservablePorts))
					Expect(string(routerGroup.Type)).To(Equal(rg.Type))
				})
			})

			Context("when router group doesn't exist", func() {
				BeforeEach(func() {
					routerGroupId = newUuid()
				})

				It("returns an empty struct", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(routerGroup).To(Equal(models.RouterGroup{}))
				})
			})
		})
	}

	SaveRouterGroup := func() {
		Describe("SaveRouterGroup", func() {
			var (
				routerGroup   models.RouterGroup
				err           error
				routerGroupId string
			)
			BeforeEach(func() {
				routerGroupId = newUuid()
				routerGroup = models.RouterGroup{
					Guid:            routerGroupId,
					Name:            "router-group-1",
					Type:            "tcp",
					ReservablePorts: "65000-65002",
				}
			})

			Context("when the router group already exists", func() {
				BeforeEach(func() {
					_, err = sqlDB.Client.Create(&models.RouterGroupDB{
						Model:           models.Model{Guid: routerGroupId},
						Name:            "rg-1",
						Type:            "tcp",
						ReservablePorts: "120",
					})
					Expect(err).ToNot(HaveOccurred())
				})

				AfterEach(func() {
					_, err = sqlDB.Client.Delete(&models.RouterGroupDB{
						Model: models.Model{Guid: routerGroupId},
					})
					Expect(err).ToNot(HaveOccurred())
				})

				It("updates the existing router group", func() {
					err = sqlDB.SaveRouterGroup(routerGroup)
					Expect(err).ToNot(HaveOccurred())
					rg, err := sqlDB.ReadRouterGroup(routerGroup.Guid)
					Expect(err).ToNot(HaveOccurred())

					Expect(rg.Guid).To(Equal(routerGroup.Guid))
					Expect(rg.Name).To(Equal(routerGroup.Name))
					Expect(rg.ReservablePorts).To(Equal(routerGroup.ReservablePorts))
					Expect(rg.Type).To(Equal(routerGroup.Type))
				})

			})

			It("Can remove ReservablePorts", func() {
				testRg := &models.RouterGroupDB{
					Model:           models.Model{Guid: routerGroupId},
					Name:            "rg-1",
					Type:            "other",
					ReservablePorts: "120",
				}
				rgGroup := testRg.ToRouterGroup()
				Expect(err).ToNot(HaveOccurred())
				err = sqlDB.SaveRouterGroup(rgGroup)
				Expect(err).ToNot(HaveOccurred())
				rg, err := sqlDB.ReadRouterGroup(testRg.Guid)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(rg.ReservablePorts)).To(Equal("120"))

				rgGroup.ReservablePorts = ""
				err = sqlDB.SaveRouterGroup(rgGroup)
				Expect(err).ToNot(HaveOccurred())
				rgAgain, err := sqlDB.ReadRouterGroup(testRg.Guid)
				Expect(err).ToNot(HaveOccurred())
				Expect(string(rgAgain.ReservablePorts)).To(Equal(""))
			})

			Context("when router group doesn't exist", func() {
				It("creates the router group", func() {
					err = sqlDB.SaveRouterGroup(routerGroup)
					Expect(err).ToNot(HaveOccurred())
					rg, err := sqlDB.ReadRouterGroup(routerGroup.Guid)
					Expect(err).ToNot(HaveOccurred())
					Expect(rg.Guid).To(Equal(routerGroup.Guid))
					Expect(rg.Name).To(Equal(routerGroup.Name))
					Expect(rg.ReservablePorts).To(Equal(routerGroup.ReservablePorts))
					Expect(rg.Type).To(Equal(routerGroup.Type))
				})
			})
		})

	}

	DeleteRouterGroup := func() {
		Describe("DeleteRouterGroup", func() {
			var (
				err           error
				routerGroup   models.RouterGroup
				routerGroupDB models.RouterGroupDB
			)
			BeforeEach(func() {
				routerGroup = models.RouterGroup{
					Guid:            "abc123foo",
					Name:            "rg-123",
					Type:            models.RouterGroup_TCP,
					ReservablePorts: "2000-3000",
				}
				routerGroupDB = models.NewRouterGroupDB(routerGroup)
			})
			JustBeforeEach(func() {
				err = sqlDB.DeleteRouterGroup(routerGroup.Guid)
			})

			Context("when at least one router group exists", func() {
				BeforeEach(func() {
					_, err = sqlDB.Client.Create(&routerGroupDB)
					Expect(err).ToNot(HaveOccurred())
					routerGroups, err := sqlDB.ReadRouterGroups()
					Expect(err).ToNot(HaveOccurred())
					Expect(routerGroups).To(HaveLen(1))
					Expect(routerGroups[0].Guid).To(Equal(routerGroup.Guid))
					Expect(routerGroups[0].Name).To(Equal(routerGroup.Name))
					Expect(routerGroups[0].Type).To(Equal(routerGroup.Type))
					Expect(routerGroups[0].ReservablePorts).To(Equal(routerGroup.ReservablePorts))
				})

				It("deletes the router group and it no longer exists in the database", func() {
					Expect(err).ToNot(HaveOccurred())

					routerGroups, err := sqlDB.ReadRouterGroups()
					Expect(err).ToNot(HaveOccurred())
					Expect(routerGroups).To(BeEmpty())
				})

				Context("when multiple router groups exist", func() {
					var (
						routerGroup2   models.RouterGroup
						routerGroupDB2 models.RouterGroupDB
					)
					BeforeEach(func() {
						routerGroup2 = models.RouterGroup{
							Guid:            "abc234foo",
							Name:            "rg-234",
							Type:            models.RouterGroup_TCP,
							ReservablePorts: "3000-4000",
						}
						routerGroupDB2 = models.NewRouterGroupDB(routerGroup2)
						_, err = sqlDB.Client.Create(&routerGroupDB2)
						Expect(err).ToNot(HaveOccurred())
					})

					AfterEach(func() {
						_, err = sqlDB.Client.Delete(&routerGroupDB2)
						Expect(err).ToNot(HaveOccurred())
					})

					It("deletes the specified router group", func() {
						Expect(err).ToNot(HaveOccurred())

						routerGroups, err := sqlDB.ReadRouterGroups()
						Expect(err).ToNot(HaveOccurred())
						Expect(routerGroups).To(HaveLen(1))
						Expect(routerGroups[0].Guid).To(Equal(routerGroup2.Guid))
						Expect(routerGroups[0].Name).To(Equal(routerGroup2.Name))
						Expect(routerGroups[0].Type).To(Equal(routerGroup2.Type))
						Expect(routerGroups[0].ReservablePorts).To(Equal(routerGroup2.ReservablePorts))
					})
				})
			})

			Context("when the router group doesn't exist", func() {
				It("returns a DB error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).Should(MatchError(db.DeleteRouterGroupError))
					dberr, ok := err.(db.DBError)
					Expect(ok).To(BeTrue())
					Expect(dberr.Type).To(Equal(db.KeyNotFound))
				})
			})
		})
	}

	SaveTcpRouteMapping := func() {
		Describe("SaveTcpRouteMapping", func() {
			var (
				routerGroupId string
				err           error
				tcpRoute      models.TcpRouteMapping
			)

			BeforeEach(func() {
				routerGroupId = newUuid()
				tcpRoute = models.NewTcpRouteMapping(routerGroupId, 3056, "127.0.0.1", 2990, 5)
			})

			AfterEach(func() {
				_, err = sqlDB.Client.Delete(&tcpRoute)
				Expect(err).ToNot(HaveOccurred())
			})

			Context("when tcp route exists", func() {
				BeforeEach(func() {
					err = sqlDB.SaveTcpRouteMapping(tcpRoute)
					Expect(err).ToNot(HaveOccurred())
				})

				It("updates the existing tcp route mapping and increments modification tag", func() {
					tcpRoute.IsolationSegment = "some-iso-seg"
					myTTL := 77
					tcpRoute.TTL = &myTTL

					err := sqlDB.SaveTcpRouteMapping(tcpRoute)
					Expect(err).ToNot(HaveOccurred())

					var dbTcpRoute models.TcpRouteMapping
					err = sqlDB.Client.Where("host_ip = ?", "127.0.0.1").First(&dbTcpRoute)
					Expect(err).ToNot(HaveOccurred())
					Expect(dbTcpRoute).ToNot(BeNil())
					Expect(dbTcpRoute.IsolationSegment).To(Equal("some-iso-seg"))
					Expect(dbTcpRoute.TTL).To(Equal(&myTTL))
					Expect(dbTcpRoute.ModificationTag.Index).To(BeNumerically("==", 1))
				})

				It("refreshes the expiration time of the mapping", func() {
					var dbTcpRoute models.TcpRouteMapping
					var ttl = 9
					err = sqlDB.Client.Where("host_ip = ?", "127.0.0.1").First(&dbTcpRoute)
					Expect(err).ToNot(HaveOccurred())
					Expect(dbTcpRoute).ToNot(BeNil())
					initialExpiration := dbTcpRoute.ExpiresAt

					tcpRoute.TTL = &ttl
					err := sqlDB.SaveTcpRouteMapping(tcpRoute)
					Expect(err).ToNot(HaveOccurred())

					err = sqlDB.Client.Where("host_ip = ?", "127.0.0.1").First(&dbTcpRoute)
					Expect(err).ToNot(HaveOccurred())
					Expect(dbTcpRoute).ToNot(BeNil())
					Expect(initialExpiration).To(BeTemporally("<", dbTcpRoute.ExpiresAt))
				})

				Context("and router group is changed", func() {
					var (
						routerGroupId2 string
						tcpRoute2      models.TcpRouteMapping
					)
					BeforeEach(func() {
						routerGroupId2 = newUuid()
						tcpRoute2 = models.NewTcpRouteMapping(routerGroupId2, 3056, "127.0.0.1", 2990, 5)
					})

					AfterEach(func() {
						_, err = sqlDB.Client.Delete(&tcpRoute2)
						Expect(err).ToNot(HaveOccurred())
					})

					It("creates another tcp route", func() {
						err = sqlDB.SaveTcpRouteMapping(tcpRoute2)
						Expect(err).ToNot(HaveOccurred())
						var dbTcpRoutes []models.TcpRouteMapping
						err = sqlDB.Client.Where("host_ip = ?", "127.0.0.1").Find(&dbTcpRoutes)
						Expect(err).ToNot(HaveOccurred())
						Expect(dbTcpRoutes).To(HaveLen(2))
						Expect(dbTcpRoutes).To(ConsistOf(
							matchers.MatchTcpRoute(tcpRoute),
							matchers.MatchTcpRoute(tcpRoute2),
						))
					})
				})
			})

			Context("when the tcp route doesn't exist", func() {
				It("creates a modification tag", func() {
					err := sqlDB.SaveTcpRouteMapping(tcpRoute)
					Expect(err).ToNot(HaveOccurred())
					var dbTcpRoute models.TcpRouteMapping
					err = sqlDB.Client.Where("host_ip = ?", "127.0.0.1").First(&dbTcpRoute)
					Expect(err).ToNot(HaveOccurred())
					Expect(dbTcpRoute.ModificationTag.Guid).ToNot(BeEmpty())
					Expect(dbTcpRoute.ModificationTag.Index).To(BeZero())
				})

				It("creates a tcp route", func() {
					err := sqlDB.SaveTcpRouteMapping(tcpRoute)
					Expect(err).ToNot(HaveOccurred())
					var dbTcpRoute models.TcpRouteMapping
					err = sqlDB.Client.Where("host_ip = ?", "127.0.0.1").First(&dbTcpRoute)
					Expect(err).ToNot(HaveOccurred())
					Expect(dbTcpRoute).To(matchers.MatchTcpRoute(tcpRoute))
				})
			})
		})
	}

	ReadTcpRouteMappings := func() {
		Describe("ReadTcpRouteMappings", func() {
			var (
				err       error
				tcpRoutes []models.TcpRouteMapping
			)

			JustBeforeEach(func() {
				tcpRoutes, err = sqlDB.ReadTcpRouteMappings()
			})

			Context("when at least one tcp route exists", func() {
				var (
					routerGroupId     string
					tcpRoute          models.TcpRouteMapping
					tcpRouteWithModel models.TcpRouteMapping
				)

				BeforeEach(func() {
					routerGroupId = newUuid()
					modTag := models.ModificationTag{Guid: "some-tag", Index: 10}
					tcpRoute = models.NewTcpRouteMapping(routerGroupId, 3056, "127.0.0.1", 2990, 5)
					tcpRoute.ModificationTag = modTag
					tcpRouteWithModel, err = models.NewTcpRouteMappingWithModel(tcpRoute)
					Expect(err).NotTo(HaveOccurred())
					_, err = sqlDB.Client.Create(&tcpRouteWithModel)
					Expect(err).ToNot(HaveOccurred())
				})

				AfterEach(func() {
					_, err = sqlDB.Client.Delete(&tcpRouteWithModel)
					Expect(err).ToNot(HaveOccurred())
				})

				It("returns the tcp routes", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(tcpRoutes).To(HaveLen(1))
					Expect(tcpRoutes[0].TcpMappingEntity).To(Equal(tcpRoute.TcpMappingEntity))
				})

				Context("when tcp routes have outlived their ttl", func() {
					var (
						routerGroupId            string
						expiredTcpRoute          models.TcpRouteMapping
						expiredTcpRouteWithModel models.TcpRouteMapping
					)

					BeforeEach(func() {
						modTag := models.ModificationTag{Guid: "some-tag", Index: 10}
						expiredTcpRoute = models.NewTcpRouteMapping(routerGroupId, 3057, "127.0.0.1", 2990, -9)
						expiredTcpRoute.ModificationTag = modTag
						expiredTcpRouteWithModel, err = models.NewTcpRouteMappingWithModel(expiredTcpRoute)
						Expect(err).NotTo(HaveOccurred())
						_, err = sqlDB.Client.Create(&expiredTcpRouteWithModel)
						Expect(err).ToNot(HaveOccurred())
					})

					AfterEach(func() {
						_, err = sqlDB.Client.Delete(&expiredTcpRouteWithModel)
						Expect(err).ToNot(HaveOccurred())
					})

					It("does not return the tcp route", func() {
						Expect(err).ToNot(HaveOccurred())

						var tcpDBRoutes []models.TcpRouteMapping
						err := sqlDB.Client.Find(&tcpDBRoutes)
						Expect(err).NotTo(HaveOccurred())
						Expect(tcpDBRoutes).To(HaveLen(2))

						Expect(tcpRoutes).To(HaveLen(1))
						Expect(tcpRoutes[0].TcpMappingEntity).To(Equal(tcpRoute.TcpMappingEntity))
					})
				})
			})

			Context("when tcp route doesn't exist", func() {
				It("returns an empty array", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(tcpRoutes).To(Equal([]models.TcpRouteMapping{}))
				})
			})
		})
	}

	ReadFilteredTcpRouteMappings := func() {
		Describe("ReadFilteredTcpRouteMappings", func() {
			var (
				err       error
				tcpRoutes []models.TcpRouteMapping
			)

			JustBeforeEach(func() {
				tcpRoutes, err = sqlDB.ReadFilteredTcpRouteMappings(
					"isolation_segment", []string{"is1", ""},
				)
			})

			Context("when at least one tcp route exists", func() {
				var (
					routerGroupID      string
					tcpRoutesWithModel []models.TcpRouteMapping
				)

				createTcpRouteWithIsoSeg := func(externalPort uint16, routerGroupID string, ttl int, isoSeg string) {
					tcpRoute := models.NewTcpRouteMapping(routerGroupID, externalPort, "127.0.0.1", 2990, ttl)
					tcpRoute.IsolationSegment = isoSeg
					tcpRoute.ModificationTag = models.ModificationTag{Guid: "some-tag", Index: 10}
					tcpRouteWithModel, err := models.NewTcpRouteMappingWithModel(tcpRoute)
					Expect(err).NotTo(HaveOccurred())
					_, err = sqlDB.Client.Create(&tcpRouteWithModel)
					Expect(err).ToNot(HaveOccurred())
					tcpRoutesWithModel = append(tcpRoutesWithModel, tcpRouteWithModel)
				}

				BeforeEach(func() {
					routerGroupID = newUuid()
					tcpRoutesWithModel = nil

					createTcpRouteWithIsoSeg(3056, routerGroupID, 5, "")
					createTcpRouteWithIsoSeg(3057, routerGroupID, 5, "is1")
					createTcpRouteWithIsoSeg(3058, routerGroupID, 5, "is2")
				})

				AfterEach(func() {
					for _, tcpRouteWithModel := range tcpRoutesWithModel {
						_, err = sqlDB.Client.Delete(&tcpRouteWithModel)
						Expect(err).ToNot(HaveOccurred())
					}
				})

				It("returns the tcp routes", func() {
					Expect(err).ToNot(HaveOccurred())

					var tcpDBRoutes []models.TcpRouteMapping
					err := sqlDB.Client.Find(&tcpDBRoutes)
					Expect(err).NotTo(HaveOccurred())
					Expect(tcpDBRoutes).To(HaveLen(3))

					Expect(tcpRoutes).To(HaveLen(2))
					Expect(tcpRoutes).To(ConsistOf(
						matchers.MatchTcpRoute(tcpRoutesWithModel[0]),
						matchers.MatchTcpRoute(tcpRoutesWithModel[1]),
					))
				})

				Context("when tcp routes have outlived their ttl", func() {
					BeforeEach(func() {
						createTcpRouteWithIsoSeg(3059, routerGroupID, -9, "is1")
					})

					It("does not return the tcp route", func() {
						Expect(err).ToNot(HaveOccurred())

						var tcpDBRoutes []models.TcpRouteMapping
						err := sqlDB.Client.Find(&tcpDBRoutes)
						Expect(err).NotTo(HaveOccurred())
						Expect(tcpDBRoutes).To(HaveLen(4))

						Expect(tcpRoutes).To(HaveLen(2))
						Expect(tcpRoutes).To(ConsistOf(
							matchers.MatchTcpRoute(tcpRoutesWithModel[0]),
							matchers.MatchTcpRoute(tcpRoutesWithModel[1]),
						))
					})
				})
			})

			Context("when tcp route doesn't exist", func() {
				It("returns an empty array", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(tcpRoutes).To(Equal([]models.TcpRouteMapping{}))
				})
			})
		})
	}

	DeleteTcpRouteMapping := func() {
		Describe("DeleteTcpRouteMapping", func() {
			var (
				err               error
				routerGroupId     string
				tcpRoute          models.TcpRouteMapping
				tcpRouteWithModel models.TcpRouteMapping
			)
			BeforeEach(func() {
				routerGroupId = newUuid()
				modTag := models.ModificationTag{Guid: "some-tag", Index: 10}
				tcpRoute = models.NewTcpRouteMapping(routerGroupId, 3056, "127.0.0.1", 2990, 5)
				tcpRoute.ModificationTag = modTag
				tcpRouteWithModel, err = models.NewTcpRouteMappingWithModel(tcpRoute)
				Expect(err).ToNot(HaveOccurred())
			})

			JustBeforeEach(func() {
				err = sqlDB.DeleteTcpRouteMapping(tcpRoute)
			})

			Context("when at least one tcp route exists", func() {
				BeforeEach(func() {
					_, err = sqlDB.Client.Create(&tcpRouteWithModel)
					Expect(err).ToNot(HaveOccurred())
				})

				AfterEach(func() {
					_, err = sqlDB.Client.Delete(&tcpRouteWithModel)
					Expect(err).ToNot(HaveOccurred())
				})

				It("deletes the tcp route", func() {
					Expect(err).ToNot(HaveOccurred())

					tcpRoutes, err := sqlDB.ReadTcpRouteMappings()
					Expect(err).ToNot(HaveOccurred())
					Expect(tcpRoutes).ToNot(ContainElement(tcpRoute))
				})

				Context("when multiple tcp routes exist", func() {
					var tcpRouteWithModel2 models.TcpRouteMapping

					BeforeEach(func() {
						modTag := models.ModificationTag{Guid: "some-tag", Index: 10}
						tcpRoute2 := models.NewTcpRouteMapping(routerGroupId, 3057, "127.0.0.1", 2990, 5)
						tcpRoute2.ModificationTag = modTag
						tcpRouteWithModel2, err = models.NewTcpRouteMappingWithModel(tcpRoute2)
						Expect(err).ToNot(HaveOccurred())
						_, err = sqlDB.Client.Create(&tcpRouteWithModel2)
						Expect(err).ToNot(HaveOccurred())
					})

					AfterEach(func() {
						_, err = sqlDB.Client.Delete(&tcpRouteWithModel2)
						Expect(err).ToNot(HaveOccurred())
					})

					It("does not delete everything", func() {
						Expect(err).ToNot(HaveOccurred())

						tcpRoutes, err := sqlDB.ReadTcpRouteMappings()
						Expect(err).ToNot(HaveOccurred())

						Expect(tcpRoutes).To(HaveLen(1))
						Expect(tcpRoutes[0]).To(matchers.MatchTcpRoute(tcpRouteWithModel2))
					})
				})
			})

			Context("when the tcp route doesn't exist", func() {
				It("returns a DB error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).Should(MatchError(db.DeleteRouteError))
					dberr, ok := err.(db.DBError)
					Expect(ok).To(BeTrue())
					Expect(dberr.Type).To(Equal(db.KeyNotFound))
				})
			})
		})
	}

	SaveRoute := func() {
		Describe("SaveRoute", func() {
			var (
				httpRoute models.Route
				err       error
			)

			BeforeEach(func() {
				httpRoute = models.NewRoute("post_here", 7000, "127.0.0.1", "my-guid", "https://rs.com", 5)
			})

			AfterEach(func() {
				_, err = sqlDB.Client.Delete(&httpRoute)
				Expect(err).ToNot(HaveOccurred())
			})

			Context("when the http route already exists", func() {
				BeforeEach(func() {
					err = sqlDB.SaveRoute(httpRoute)
					Expect(err).ToNot(HaveOccurred())
				})

				It("updates the existing route and increments its modification tag", func() {
					err := sqlDB.SaveRoute(httpRoute)
					Expect(err).ToNot(HaveOccurred())

					var dbRoute models.Route
					err = sqlDB.Client.Where("ip = ?", "127.0.0.1").First(&dbRoute)
					Expect(err).ToNot(HaveOccurred())
					Expect(dbRoute).ToNot(BeNil())
					Expect(dbRoute.ModificationTag.Index).To(BeNumerically("==", 1))
				})

				It("refreshes the expiration time of the route", func() {
					var dbRoute models.Route
					var ttl = 9
					err = sqlDB.Client.Where("ip = ?", "127.0.0.1").First(&dbRoute)
					Expect(err).ToNot(HaveOccurred())
					Expect(dbRoute).ToNot(BeNil())
					initialExpiration := dbRoute.ExpiresAt

					httpRoute.TTL = &ttl
					err := sqlDB.SaveRoute(httpRoute)
					Expect(err).ToNot(HaveOccurred())

					err = sqlDB.Client.Where("ip = ?", "127.0.0.1").First(&dbRoute)
					Expect(err).ToNot(HaveOccurred())
					Expect(dbRoute).ToNot(BeNil())
					Expect(initialExpiration).To(BeTemporally("<", dbRoute.ExpiresAt))
				})
			})

			Context("when the http route doesn't exist", func() {
				It("creates a modification tag", func() {
					err := sqlDB.SaveRoute(httpRoute)

					Expect(err).ToNot(HaveOccurred())
					var dbRoute models.Route
					err = sqlDB.Client.Where("ip = ?", "127.0.0.1").First(&dbRoute)
					Expect(err).ToNot(HaveOccurred())
					Expect(dbRoute.ModificationTag.Guid).ToNot(BeEmpty())
					Expect(dbRoute.ModificationTag.Index).To(BeZero())
				})

				It("creates a http route", func() {
					err := sqlDB.SaveRoute(httpRoute)
					Expect(err).ToNot(HaveOccurred())
					var dbRoute models.Route
					err = sqlDB.Client.Where("ip = ?", "127.0.0.1").First(&dbRoute)
					Expect(err).ToNot(HaveOccurred())
					Expect(dbRoute).To(matchers.MatchHttpRoute(httpRoute))
				})
			})

			Context("when there is a connection error", func() {
				BeforeEach(func() {
					fakeClient := &fakes.FakeClient{}
					fakeClient.WhereReturns(fakeClient)
					fakeClient.FindReturns(errors.New("BOOM!"))
					sqlDB.Client = fakeClient
				})

				It("returns an error", func() {
					err := sqlDB.SaveRoute(httpRoute)
					Expect(err).To(HaveOccurred())
				})
			})
		})
	}

	ReadRoute := func() {
		Describe("ReadRoute", func() {
			var (
				err    error
				routes []models.Route
			)

			JustBeforeEach(func() {
				routes, err = sqlDB.ReadRoutes()
			})

			Context("when at least one route exists", func() {
				var (
					route          models.Route
					routeWithModel models.Route
				)

				BeforeEach(func() {
					modTag := models.ModificationTag{Guid: "some-tag", Index: 10}
					route = models.NewRoute("post_here", 7000, "127.0.0.1", "my-guid", "https://rs.com", 5)
					route.ModificationTag = modTag
					routeWithModel, err = models.NewRouteWithModel(route)
					Expect(err).NotTo(HaveOccurred())
					_, err = sqlDB.Client.Create(&routeWithModel)
					Expect(err).ToNot(HaveOccurred())
				})

				AfterEach(func() {
					_, err = sqlDB.Client.Delete(&routeWithModel)
					Expect(err).ToNot(HaveOccurred())
				})

				It("returns the routes", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(routes).To(HaveLen(1))
					Expect(routes[0]).To(matchers.MatchHttpRoute(routeWithModel))
				})

				Context("when http routes have outlived their ttl", func() {
					var (
						expiredRoute          models.Route
						expiredRouteWithModel models.Route
					)

					BeforeEach(func() {
						modTag := models.ModificationTag{Guid: "some-tag", Index: 10}
						expiredRoute = models.NewRoute("post_here", 7001, "127.0.0.1", "my-guid", "https://rs.com", -9)
						expiredRoute.ModificationTag = modTag
						expiredRouteWithModel, err = models.NewRouteWithModel(expiredRoute)
						Expect(err).NotTo(HaveOccurred())
						_, err = sqlDB.Client.Create(&expiredRouteWithModel)
						Expect(err).ToNot(HaveOccurred())
					})

					AfterEach(func() {
						_, err = sqlDB.Client.Delete(&expiredRouteWithModel)
						Expect(err).ToNot(HaveOccurred())
					})

					It("does not return the route", func() {
						Expect(err).ToNot(HaveOccurred())

						var dbRoutes []models.Route
						err := sqlDB.Client.Find(&dbRoutes)
						Expect(err).NotTo(HaveOccurred())
						Expect(dbRoutes).To(HaveLen(2))

						Expect(routes).To(HaveLen(1))
						Expect(routes[0]).To(matchers.MatchHttpRoute(route))
					})
				})
			})

			Context("when the http route doesn't exist", func() {
				It("returns an empty array", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(routes).To(Equal([]models.Route{}))
				})
			})
		})
	}

	DeleteRoute := func() {
		Describe("DeleteRoute", func() {
			var (
				err            error
				route          models.Route
				routeWithModel models.Route
			)
			BeforeEach(func() {
				modTag := models.ModificationTag{Guid: "some-tag", Index: 10}
				route = models.NewRoute("post_here", 7000, "127.0.0.1", "my-guid", "https://rs.com", 100)
				route.ModificationTag = modTag
				routeWithModel, err = models.NewRouteWithModel(route)
				Expect(err).ToNot(HaveOccurred())
			})

			JustBeforeEach(func() {
				err = sqlDB.DeleteRoute(route)
			})

			Context("when at least one route exists", func() {
				BeforeEach(func() {
					_, err = sqlDB.Client.Create(&routeWithModel)
					Expect(err).ToNot(HaveOccurred())
					routes, err := sqlDB.ReadRoutes()
					Expect(err).ToNot(HaveOccurred())
					Expect(routes).To(HaveLen(1))
					Expect(routes).ToNot(ContainElement(route))
				})

				AfterEach(func() {
					_, err = sqlDB.Client.Delete(&routeWithModel)
					Expect(err).ToNot(HaveOccurred())
				})

				It("deletes the route", func() {
					Expect(err).ToNot(HaveOccurred())

					routes, err := sqlDB.ReadRoutes()
					Expect(err).ToNot(HaveOccurred())
					Expect(routes).To(BeEmpty())
				})

				Context("when multiple routes exist", func() {
					var (
						routeWithModel2 models.Route
					)
					BeforeEach(func() {
						modTag := models.ModificationTag{Guid: "some-tag", Index: 10}
						route := models.NewRoute("post_here", 7001, "127.0.0.1", "my-guid", "https://rs.com", 5)
						route.ModificationTag = modTag
						routeWithModel2, err = models.NewRouteWithModel(route)
						Expect(err).ToNot(HaveOccurred())
						_, err = sqlDB.Client.Create(&routeWithModel2)
						Expect(err).ToNot(HaveOccurred())
					})

					AfterEach(func() {
						_, err = sqlDB.Client.Delete(&routeWithModel2)
						Expect(err).ToNot(HaveOccurred())
					})

					It("deletes the specified route", func() {
						Expect(err).ToNot(HaveOccurred())

						routes, err := sqlDB.ReadRoutes()
						Expect(err).ToNot(HaveOccurred())
						Expect(routes).To(HaveLen(1))
						Expect(routes[0]).To(matchers.MatchHttpRoute(routeWithModel2))
					})
				})
			})

			Context("when the route doesn't exist", func() {
				It("returns a DB error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err).Should(MatchError(db.DeleteRouteError))
					dberr, ok := err.(db.DBError)
					Expect(ok).To(BeTrue())
					Expect(dberr.Type).To(Equal(db.KeyNotFound))
				})
			})
		})
	}

	WatcherRouteChanges := func() {
		Describe("WatchChanges with tcp events", func() {
			var (
				err           error
				routerGroupId string
			)

			BeforeEach(func() {
				routerGroupId = newUuid()
			})

			It("does not return an error when canceled", func() {
				_, errors, cancel := sqlDB.WatchChanges(db.TCP_WATCH)
				cancel()
				Consistently(errors).ShouldNot(Receive())
				Eventually(errors).Should(BeClosed())
			})

			Context("with wrong event type", func() {
				It("throws an error", func() {
					event, err, _ := sqlDB.WatchChanges("some-random-key")
					Eventually(err).Should(Receive())
					Eventually(err).Should(BeClosed())

					Consistently(event).ShouldNot(Receive())
					Eventually(event).Should(BeClosed())
				})
			})

			Context("when a tcp route is updated", func() {
				var (
					tcpRoute models.TcpRouteMapping
				)

				BeforeEach(func() {
					tcpRoute = models.NewTcpRouteMapping(routerGroupId, 3057, "127.0.0.1", 2990, 50)
					err = sqlDB.SaveTcpRouteMapping(tcpRoute)
					Expect(err).NotTo(HaveOccurred())
				})

				It("should return an update watch event", func() {
					results, _, _ := sqlDB.WatchChanges(db.TCP_WATCH)

					err = sqlDB.SaveTcpRouteMapping(tcpRoute)
					Expect(err).NotTo(HaveOccurred())

					var event db.Event
					Eventually(results).Should((Receive(&event)))
					Expect(event).NotTo(BeNil())
					Expect(event.Type).To(Equal(db.UpdateEvent))
					Expect(event.Value).To(ContainSubstring(`"port":3057`))
				})
			})

			Context("when a tcp route is created", func() {
				It("should return an create watch event", func() {
					results, _, _ := sqlDB.WatchChanges(db.TCP_WATCH)

					tcpRoute := models.NewTcpRouteMapping(routerGroupId, 3057, "127.0.0.1", 2990, 50)
					err = sqlDB.SaveTcpRouteMapping(tcpRoute)
					Expect(err).NotTo(HaveOccurred())

					var event db.Event
					Eventually(results).Should((Receive(&event)))
					Expect(event).NotTo(BeNil())
					Expect(event.Type).To(Equal(db.CreateEvent))
					Expect(event.Value).To(ContainSubstring(`"port":3057`))
				})
			})

			Context("when a route is deleted", func() {
				It("should return an delete watch event", func() {
					tcpRoute := models.NewTcpRouteMapping(routerGroupId, 3057, "127.0.0.1", 2990, 50)
					err := sqlDB.SaveTcpRouteMapping(tcpRoute)
					Expect(err).NotTo(HaveOccurred())

					results, _, _ := sqlDB.WatchChanges(db.TCP_WATCH)

					err = sqlDB.DeleteTcpRouteMapping(tcpRoute)
					Expect(err).NotTo(HaveOccurred())

					var event db.Event
					Eventually(results).Should((Receive(&event)))
					Expect(event).NotTo(BeNil())
					Expect(event.Type).To(Equal(db.DeleteEvent))
					Expect(event.Value).To(ContainSubstring(`"port":3057`))
				})
			})

			Context("Cancel Watches", func() {
				It("cancels any in-flight watches", func() {
					results, err, _ := sqlDB.WatchChanges(db.TCP_WATCH)
					results2, err2, _ := sqlDB.WatchChanges(db.TCP_WATCH)

					sqlDB.CancelWatches()

					Eventually(err).Should(BeClosed())
					Eventually(results).Should(BeClosed())
					Eventually(err2).Should(BeClosed())
					Eventually(results2).Should(BeClosed())
				})

				It("doesn't panic when called twice", func() {
					sqlDB.CancelWatches()
					sqlDB.CancelWatches()
				})

				It("causes subsequent calls to WatchChanges to error", func() {
					sqlDB.CancelWatches()

					event, err, _ := sqlDB.WatchChanges(db.TCP_WATCH)
					Eventually(err).ShouldNot(Receive())
					Eventually(err).Should(BeClosed())

					Consistently(event).ShouldNot(Receive())
					Eventually(event).Should(BeClosed())

				})
			})
		})

		Describe("WatchChanges with http events", func() {
			var (
				err error
			)

			It("does not return an error when canceled", func() {
				_, errors, cancel := sqlDB.WatchChanges(db.HTTP_WATCH)
				cancel()
				Consistently(errors).ShouldNot(Receive())
				Eventually(errors).Should(BeClosed())
			})

			Context("when a http route is updated", func() {
				var (
					httpRoute models.Route
				)

				BeforeEach(func() {
					httpRoute = models.NewRoute("post_here", 7001, "127.0.0.1", "my-guid", "https://rs.com", 5)
					err = sqlDB.SaveRoute(httpRoute)
					Expect(err).NotTo(HaveOccurred())
				})

				It("should return an update watch event", func() {
					results, _, _ := sqlDB.WatchChanges(db.HTTP_WATCH)

					err = sqlDB.SaveRoute(httpRoute)
					Expect(err).NotTo(HaveOccurred())

					var event db.Event
					Eventually(results).Should((Receive(&event)))
					Expect(event).NotTo(BeNil())
					Expect(event.Type).To(Equal(db.UpdateEvent))
					Expect(event.Value).To(ContainSubstring(`"port":7001`))
				})
			})

			Context("when a http route is created", func() {
				It("should return an create watch event", func() {
					results, _, _ := sqlDB.WatchChanges(db.HTTP_WATCH)

					httpRoute := models.NewRoute("post_here", 7002, "127.0.0.1", "my-guid", "https://rs.com", 5)
					err := sqlDB.SaveRoute(httpRoute)
					Expect(err).NotTo(HaveOccurred())

					var event db.Event
					Eventually(results).Should((Receive(&event)))
					Expect(event).NotTo(BeNil())
					Expect(event.Type).To(Equal(db.CreateEvent))
					Expect(event.Value).To(ContainSubstring(`"port":7002`))
				})
			})

			Context("when a http route is deleted", func() {
				It("should return an delete watch event", func() {
					httpRoute := models.NewRoute("post_here", 7003, "127.0.0.1", "my-guid", "https://rs.com", 5)
					err := sqlDB.SaveRoute(httpRoute)
					Expect(err).NotTo(HaveOccurred())

					results, _, _ := sqlDB.WatchChanges(db.HTTP_WATCH)

					err = sqlDB.DeleteRoute(httpRoute)
					Expect(err).NotTo(HaveOccurred())

					var event db.Event
					Eventually(results).Should((Receive(&event)))
					Expect(event).NotTo(BeNil())
					Expect(event.Type).To(Equal(db.DeleteEvent))
					Expect(event.Value).To(ContainSubstring(`"port":7003`))
				})
			})

			Context("Cancel Watches", func() {
				It("cancels any in-flight watches", func() {
					results, err, _ := sqlDB.WatchChanges(db.HTTP_WATCH)
					results2, err2, _ := sqlDB.WatchChanges(db.HTTP_WATCH)

					sqlDB.CancelWatches()

					Eventually(err).Should(BeClosed())
					Eventually(results).Should(BeClosed())
					Eventually(err2).Should(BeClosed())
					Eventually(results2).Should(BeClosed())
				})

				It("causes subsequent calls to WatchChanges to error", func() {
					sqlDB.CancelWatches()

					event, err, _ := sqlDB.WatchChanges(db.HTTP_WATCH)
					Eventually(err).ShouldNot(Receive())
					Eventually(err).Should(BeClosed())

					Consistently(event).ShouldNot(Receive())
					Eventually(event).Should(BeClosed())

				})
			})
		})
	}

	CleanupRoutes := func() {
		Describe("Cleanup routes", func() {
			var (
				logger  lager.Logger
				signals chan os.Signal
			)

			BeforeEach(func() {
				signals = make(chan os.Signal, 1)
			})

			JustBeforeEach(func() {
				logger = lagertest.NewTestLogger("prune")
				go sqlDB.CleanupRoutes(logger, 100*time.Millisecond, signals)
			})

			AfterEach(func() {
				close(signals)
			})

			Context("when cleanup takes longer than the cleanup interval", func() {
				var (
					fakeClient *fakes.FakeClient
					done       chan bool
					count      int32
				)

				BeforeEach(func() {
					done = make(chan bool, 2)
					fakeClient = &fakes.FakeClient{}
					fakeClient.DeleteStub = func(value interface{}, where ...interface{}) (int64, error) {
						time.Sleep(500 * time.Millisecond)
						c := atomic.AddInt32(&count, 1)
						if c <= 2 {
							done <- true
						}
						return 0, nil
					}
					sqlDB.Client = fakeClient
				})

				AfterEach(func() {
					close(done)
				})

				It("should not cleanup before the previous cleanup is complete", func() {
					Eventually(fakeClient.DeleteCallCount).Should(Equal(2))
					Eventually(done).Should(Receive())
					Eventually(done).Should(Receive())

					Eventually(fakeClient.DeleteCallCount).Should(Equal(4))
				})

			})

			Context("tcp routes", func() {
				var tcpRouteModel models.TcpRouteMapping

				BeforeEach(func() {
					tcpRoute := models.NewTcpRouteMapping("guid", 3555, "127.0.0.1", 7879, 2)
					var err error
					tcpRouteModel, err = models.NewTcpRouteMappingWithModel(tcpRoute)
					Expect(err).ToNot(HaveOccurred())
					err = sqlDB.SaveTcpRouteMapping(tcpRouteModel)
					Expect(err).ToNot(HaveOccurred())

					routes, err := sqlDB.ReadTcpRouteMappings()
					Expect(routes).To(HaveLen(1))
				})

				Context("when db connection is successful", func() {
					Context("when all routes have expired", func() {

						It("should prune the expired routes and log the number of pruned routes", func() {
							Eventually(func() []models.TcpRouteMapping {
								var tcpRoutes []models.TcpRouteMapping
								err := sqlDB.Client.Where("host_ip = ?", "127.0.0.1").Find(&tcpRoutes)
								Expect(err).ToNot(HaveOccurred())
								return tcpRoutes
							}, 5).Should(HaveLen(0))
							Eventually(logger, 2).Should(gbytes.Say(`"prune.successfully-finished-pruning-tcp-routes","log_level":1,"data":{"rowsAffected":1}`))
						})

						It("should emit a ExpireEvent for the pruned route", func() {
							results, _, _ := sqlDB.WatchChanges(db.TCP_WATCH)
							var event db.Event
							Eventually(results, 5).Should((Receive(&event)))
							Expect(event).NotTo(BeNil())
							Expect(event.Type).To(Equal(db.ExpireEvent))
							Expect(event.Value).To(ContainSubstring(`"port":3555`))
						})
					})

					Context("when routes that have not expired exist", func() {
						var tcpRoute models.TcpRouteMapping

						BeforeEach(func() {
							tcpRoute = models.NewTcpRouteMapping("guid", 3556, "127.0.0.1", 7879, 100)
							err := sqlDB.SaveTcpRouteMapping(tcpRoute)
							Expect(err).ToNot(HaveOccurred())

							var routesDB []models.TcpRouteMapping
							err = sqlDB.Client.Find(&routesDB)
							Expect(err).ToNot(HaveOccurred())
							Expect(routesDB).To(HaveLen(2))
						})

						It("should only prune expired routes", func() {
							var tcpRoutes []models.TcpRouteMapping

							Eventually(func() []models.TcpRouteMapping {
								err := sqlDB.Client.Where("host_ip = ?", "127.0.0.1").Find(&tcpRoutes)
								Expect(err).ToNot(HaveOccurred())
								return tcpRoutes
							}, 5).Should(HaveLen(1))

							Expect(tcpRoutes[0]).To(matchers.MatchTcpRoute(tcpRoute))
						})
					})
				})
			})

			Context("http routes", func() {
				BeforeEach(func() {
					httpRoute := models.NewRoute("post_here", 7000, "127.0.0.1", "my-guid", "https://rs.com", 2)
					httpRouteModel, err := models.NewRouteWithModel(httpRoute)
					Expect(err).ToNot(HaveOccurred())
					err = sqlDB.SaveRoute(httpRouteModel)
					Expect(err).ToNot(HaveOccurred())

					routes, err := sqlDB.ReadRoutes()
					Expect(err).ToNot(HaveOccurred())
					Expect(routes).To(HaveLen(1))
				})

				Context("when db connection is successful", func() {
					Context("when all routes have expired", func() {
						It("should prune the expired routes and log the number of pruned routes", func() {
							Eventually(func() []models.Route {
								var httpRoutes []models.Route
								err := sqlDB.Client.Where("ip = ?", "127.0.0.1").Find(&httpRoutes)
								Expect(err).ToNot(HaveOccurred())
								return httpRoutes
							}, 5).Should(HaveLen(0))

							Eventually(logger, 2).Should(gbytes.Say(`prune.successfully-finished-pruning-http-routes","log_level":1,"data":{"rowsAffected":1}`))
						})

						It("should emit a ExpireEvent for the pruned route", func() {
							results, _, _ := sqlDB.WatchChanges(db.HTTP_WATCH)
							var event db.Event
							Eventually(results, 5).Should((Receive(&event)))
							Expect(event).NotTo(BeNil())
							Expect(event.Type).To(Equal(db.ExpireEvent))
							Expect(event.Value).To(ContainSubstring(`"port":7000`))
						})
					})

					Context("when some routes are expired", func() {
						var httpRoute models.Route

						BeforeEach(func() {
							httpRoute = models.NewRoute("post_here", 7001, "127.0.0.1", "my-guid", "https://rs.com", 100)
							err := sqlDB.SaveRoute(httpRoute)
							Expect(err).ToNot(HaveOccurred())

							var dbRoutes []models.Route
							err = sqlDB.Client.Where("ip = ?", "127.0.0.1").Find(&dbRoutes)
							Expect(err).ToNot(HaveOccurred())
							Expect(dbRoutes).To(HaveLen(2))
						})

						It("should prune only expired routes", func() {
							var httpRoutes []models.Route

							Eventually(func() []models.Route {
								err := sqlDB.Client.Where("ip = ?", "127.0.0.1").Find(&httpRoutes)
								Expect(err).ToNot(HaveOccurred())
								return httpRoutes
							}, 5).Should(HaveLen(1))

							Expect(httpRoutes[0]).To(matchers.MatchHttpRoute(httpRoute))
						})
					})
				})
			})

			Context("when db throws an error", func() {

				BeforeEach(func() {
					err := sqlDB.Client.Close()
					Expect(err).ToNot(HaveOccurred())
				})

				AfterEach(func() {
					_, err := db.NewSqlDB(sqlCfg)
					Expect(err).ToNot(HaveOccurred())
				})

				It("logs error message", func() {
					Eventually(logger, 2).Should(gbytes.Say(`failed-to-prune-.*-routes","log_level":2,"data":{"error":"sql: database is closed"}`))
					Eventually(logger, 2).Should(gbytes.Say(`failed-to-prune-.*-routes","log_level":2,"data":{"error":"sql: database is closed"}`))
				})
			})
		})
	}

	Describe("Test with Mysql", func() {
		var (
			err error
		)

		BeforeEach(func() {
			sqlCfg = mysqlCfg
			sqlDB, err = db.NewSqlDB(sqlCfg)
			Expect(err).ToNot(HaveOccurred())
			err = migration.NewV0InitMigration().Run(sqlDB)
			Expect(err).ToNot(HaveOccurred())
		})

		MySQLConnectionString()
		CleanupRoutes()
		WatcherRouteChanges()
		DeleteRoute()
		ReadRoute()
		SaveRoute()
		DeleteTcpRouteMapping()
		ReadTcpRouteMappings()
		ReadFilteredTcpRouteMappings()
		SaveTcpRouteMapping()
		ReadRouterGroup()
		ReadRouterGroupByName()
		ReadRouterGroups()
		SaveRouterGroup()
		DeleteRouterGroup()
		Connection()
	})

	Describe("Test with Postgres", func() {
		var (
			err error
		)

		BeforeEach(func() {
			sqlCfg = postgresCfg
			sqlDB, err = db.NewSqlDB(sqlCfg)
			Expect(err).ToNot(HaveOccurred())
			err = migration.NewV0InitMigration().Run(sqlDB)
			Expect(err).ToNot(HaveOccurred())
		})

		PostgresConnectionString()
		CleanupRoutes()
		WatcherRouteChanges()
		DeleteRoute()
		ReadRoute()
		SaveRoute()
		DeleteTcpRouteMapping()
		ReadTcpRouteMappings()
		ReadFilteredTcpRouteMappings()
		SaveTcpRouteMapping()
		ReadRouterGroup()
		ReadRouterGroupByName()
		ReadRouterGroups()
		SaveRouterGroup()
		DeleteRouterGroup()
		Connection()
	})

})

func newUuid() string {
	u, err := uuid.NewV4()
	Expect(err).ToNot(HaveOccurred())
	return u.String()
}
