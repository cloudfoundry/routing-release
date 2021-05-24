package testrunner

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"code.cloudfoundry.org/routing-release/routing-api/db"

	"code.cloudfoundry.org/routing-release/routing-api/config"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	. "github.com/onsi/ginkgo"
)

type DbAllocator interface {
	Create() (*config.SqlDB, error)
	Reset() error
	Delete() error
	minConfig() *config.SqlDB
}

type mysqlAllocator struct {
	sqlDB      *sql.DB
	schemaName string
}

type postgresAllocator struct {
	sqlDB      *sql.DB
	schemaName string
}

func randSchemaName() string {
	return fmt.Sprintf("test%d%d", time.Now().UnixNano(), GinkgoParallelNode())
}

func NewPostgresAllocator() DbAllocator {
	return &postgresAllocator{schemaName: randSchemaName()}
}

func (a *postgresAllocator) minConfig() *config.SqlDB {
	return &config.SqlDB{
		Username:          "postgres",
		Password:          "",
		Host:              "localhost",
		Port:              5432,
		Type:              "postgres",
		CACert:            os.Getenv("SQL_SERVER_CA_CERT"),
		SkipSSLValidation: os.Getenv("DB_SKIP_SSL_VALIDATION") == "true",
	}
}

func (a *postgresAllocator) Create() (*config.SqlDB, error) {
	var (
		err error
		cfg *config.SqlDB
	)

	cfg = a.minConfig()
	connStr, err := db.ConnectionString(cfg)
	if err != nil {
		return nil, err
	}
	a.sqlDB, err = sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}
	err = a.sqlDB.Ping()
	if err != nil {
		return nil, err
	}

	for i := 0; i < 5; i++ {
		dbExists, err := a.sqlDB.Exec(fmt.Sprintf("SELECT * FROM pg_database WHERE datname='%s'", a.schemaName))
		rowsAffected, err := dbExists.RowsAffected()
		if err != nil {
			return nil, err
		}
		if rowsAffected == 0 {
			_, err = a.sqlDB.Exec(fmt.Sprintf("CREATE DATABASE %s", a.schemaName))
			if err != nil {
				return nil, err
			}
			cfg.Schema = a.schemaName
			return cfg, nil
		} else {
			a.schemaName = randSchemaName()
		}
	}
	return nil, errors.New("Failed to create unique database ")
}

func (a *postgresAllocator) Reset() error {
	_, err := a.sqlDB.Exec(fmt.Sprintf(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity
	WHERE datname = '%s'`, a.schemaName))
	_, err = a.sqlDB.Exec(fmt.Sprintf("DROP DATABASE %s", a.schemaName))
	if err != nil {
		return err
	}

	_, err = a.sqlDB.Exec(fmt.Sprintf("CREATE DATABASE %s", a.schemaName))
	return err
}

func (a *postgresAllocator) Delete() error {
	defer func() {
		_ = a.sqlDB.Close()
	}()
	_, err := a.sqlDB.Exec(fmt.Sprintf(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity
	WHERE datname = '%s'`, a.schemaName))
	if err != nil {
		return err
	}
	_, err = a.sqlDB.Exec(fmt.Sprintf("DROP DATABASE %s", a.schemaName))
	return err
}

func NewMySQLAllocator() DbAllocator {
	return &mysqlAllocator{schemaName: randSchemaName()}
}

func (a *mysqlAllocator) minConfig() *config.SqlDB {
	return &config.SqlDB{
		Username:          "root",
		Password:          "password",
		Host:              "localhost",
		Port:              3306,
		Type:              "mysql",
		CACert:            os.Getenv("SQL_SERVER_CA_CERT"),
		SkipSSLValidation: os.Getenv("DB_SKIP_SSL_VALIDATION") == "true",
	}
}

func (a *mysqlAllocator) Create() (*config.SqlDB, error) {
	var (
		err error
		cfg *config.SqlDB
	)

	cfg = a.minConfig()
	connStr, err := db.ConnectionString(cfg)
	if err != nil {
		return nil, err
	}
	a.sqlDB, err = sql.Open("mysql", connStr)
	if err != nil {
		return nil, err
	}
	err = a.sqlDB.Ping()
	if err != nil {
		return nil, err
	}

	for i := 0; i < 5; i++ {
		dbExists, err := a.sqlDB.Exec(fmt.Sprintf("SHOW DATABASES LIKE '%s'", a.schemaName))
		rowsAffected, err := dbExists.RowsAffected()
		if err != nil {
			return nil, err
		}
		if rowsAffected == 0 {
			_, err = a.sqlDB.Exec(fmt.Sprintf("CREATE DATABASE %s", a.schemaName))
			if err != nil {
				return nil, err
			}
			cfg.Schema = a.schemaName
			return cfg, nil
		} else {
			a.schemaName = randSchemaName()
		}
	}
	return nil, errors.New("Failed to create unique database ")
}

func (a *mysqlAllocator) Reset() error {
	_, err := a.sqlDB.Exec(fmt.Sprintf("DROP DATABASE %s", a.schemaName))
	if err != nil {
		return err
	}

	_, err = a.sqlDB.Exec(fmt.Sprintf("CREATE DATABASE %s", a.schemaName))
	return err
}

func (a *mysqlAllocator) Delete() error {
	defer func() {
		_ = a.sqlDB.Close()
	}()
	_, err := a.sqlDB.Exec(fmt.Sprintf("DROP DATABASE %s", a.schemaName))
	return err
}
