package migration

import (
	"os"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/routing-release/routing-api/db"
	"github.com/jinzhu/gorm"
)

const MigrationKey = "routing-api-migration"

type MigrationData struct {
	MigrationKey   string `gorm:"primary_key"`
	CurrentVersion int
	TargetVersion  int
}

type Runner struct {
	sqlDB  *db.SqlDB
	logger lager.Logger
}

func NewRunner(
	sqlDB *db.SqlDB,
	logger lager.Logger,
) *Runner {
	return &Runner{
		sqlDB:  sqlDB,
		logger: logger,
	}
}
func (r *Runner) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	err2 := RunAllMigration(r.sqlDB, r.logger)
	if err2 != nil {
		return err2
	}

	close(ready)

	select {
	case sig := <-signals:
		r.logger.Info("received signal", lager.Data{"signal": sig})
	}

	return nil
}

func RunAllMigration(sqlDB *db.SqlDB, logger lager.Logger) error {
	migrations := InitializeMigrations()

	logger.Info("starting-migration")
	err := RunMigrations(sqlDB, migrations, logger)
	if err != nil {
		logger.Error("migrations-failed", err)
		return err
	}
	logger.Info("finished-migration")
	return nil
}

//go:generate counterfeiter -o fakes/fake_migration.go . Migration
type Migration interface {
	Run(*db.SqlDB) error
	Version() int
}

func InitializeMigrations() []Migration {
	migrations := []Migration{}
	var migration Migration

	migration = NewV0InitMigration()
	migrations = append(migrations, migration)

	migration = NewV2UpdateRgMigration()
	migrations = append(migrations, migration)

	migration = NewV3UpdateTcpRouteMigration()
	migrations = append(migrations, migration)

	migration = NewV4AddRgUniqIdxTCPRouteMigration()
	migrations = append(migrations, migration)

	migration = NewV5SniHostnameMigration()
	migrations = append(migrations, migration)

	return migrations
}

func RunMigrations(sqlDB *db.SqlDB, migrations []Migration, logger lager.Logger) error {
	if len(migrations) == 0 {
		return nil
	}

	if sqlDB == nil {
		return nil
	}

	lastMigrationVersion := migrations[len(migrations)-1].Version()
	err := sqlDB.Client.AutoMigrate(&MigrationData{})
	if err != nil {
		return err
	}

	tx := sqlDB.Client.Begin()
	existingVersion := &MigrationData{}

	err = tx.Where("migration_key = ?", MigrationKey).First(existingVersion)
	if err != nil && err != gorm.ErrRecordNotFound {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			logger.Error("rollback-error", rollbackErr)
		}
		return err
	}

	if err == gorm.ErrRecordNotFound {
		existingVersion = &MigrationData{
			MigrationKey:   MigrationKey,
			CurrentVersion: -1,
			TargetVersion:  lastMigrationVersion,
		}

		logger.Info("creating-migration-version", lager.Data{"version": existingVersion})
		_, err = tx.Create(existingVersion)
	} else {
		if existingVersion.TargetVersion >= lastMigrationVersion {
			return tx.Commit()
		}

		existingVersion.TargetVersion = lastMigrationVersion
		logger.Info("updating-migration-version", lager.Data{"version": existingVersion})
		_, err = tx.Save(existingVersion)
	}

	if err != nil {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil {
			logger.Error("rollback-error", rollbackErr)
		}
		return err
	}

	err = tx.Commit()
	if err != nil {
		logger.Error("commit-error", err)
		return err
	}

	currentVersion := existingVersion.CurrentVersion
	for _, m := range migrations {
		if m != nil && m.Version() > currentVersion {
			err = m.Run(sqlDB)
			if err != nil {
				return err
			}
			currentVersion = m.Version()
			existingVersion.CurrentVersion = currentVersion
			_, err = sqlDB.Client.Save(existingVersion)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
