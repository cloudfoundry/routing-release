package testrunner

import (
	"fmt"
	"os"

	loggingclient "code.cloudfoundry.org/diego-logging-client"
	"code.cloudfoundry.org/locket/cmd/locket/config"
	locketrunner "code.cloudfoundry.org/locket/cmd/locket/testrunner"
	"github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
	ginkgomon "github.com/tedsuo/ifrit/ginkgomon_v2"
)

func StartLocket(
	locketPort uint16,
	locketBinPath string,
	databaseName string,
	caCert string,
	logConfig loggingclient.Config,
) ifrit.Process {
	locketAddress := fmt.Sprintf("localhost:%d", locketPort)

	locketRunner := locketrunner.NewLocketRunner(locketBinPath, func(cfg *config.LocketConfig) {
		switch Database {
		case Postgres:
			cfg.DatabaseConnectionString = fmt.Sprintf(
				"user=%s password=%s host=%s dbname=%s",
				PostgresUsername,
				PostgresPassword,
				Host,
				databaseName,
			)
			cfg.DatabaseDriver = Postgres
		default:
			cfg.DatabaseConnectionString = fmt.Sprintf("%s:%s@/%s", MySQLUserName, MySQLPassword, databaseName)
			cfg.DatabaseDriver = MySQL
		}
		if caCert != "" {
			caFile, err := os.CreateTemp("", "")
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			gomega.Expect(os.WriteFile(caFile.Name(), []byte(caCert), 0400)).To(gomega.Succeed())
			cfg.SQLCACertFile = caFile.Name()
		}
		cfg.ListenAddress = locketAddress
		cfg.LoggregatorConfig = logConfig
	})

	return ginkgomon.Invoke(locketRunner)
}

func StopLocket(locketProcess ifrit.Process) {
	ginkgomon.Interrupt(locketProcess)
	locketProcess.Wait()
}
