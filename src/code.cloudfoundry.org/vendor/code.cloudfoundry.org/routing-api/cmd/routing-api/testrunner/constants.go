package testrunner

import (
	"os"

	"code.cloudfoundry.org/routing-api/config"
)

const (
	RoutingAPIIP                    = "127.0.0.1"
	Host                            = "localhost"
	Postgres                        = "postgres"
	PostgresUsername                = "postgres"
	PostgresPassword                = ""
	PostgresPort                    = 5432
	MySQL                           = "mysql"
	MySQLUserName                   = "root"
	MySQLPassword                   = "password"
	MySQLPort                       = 3306
	SystemDomain                    = "example.com"
	MetricsReportingIntervalString  = "500ms"
	StatsdClientFlushIntervalString = "10ms"
	StatsdPort                      = 8125
)

var (
	Database     = os.Getenv("DB")
	MetronConfig = config.MetronConfig{
		Address: "1.2.3.4",
		Port:    "4567",
	}
)
