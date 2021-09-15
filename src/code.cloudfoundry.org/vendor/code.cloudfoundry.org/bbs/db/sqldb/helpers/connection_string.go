package helpers

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"io/ioutil"
	"strconv"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx"
	"github.com/jackc/pgx/stdlib"
	_ "github.com/jackc/pgx/stdlib"
)

// MYSQL group_concat_max_len system variable
// defines the length of the result returned by GROUP_CONCAT() function
// default value 1024 only allows 282 instance indexes to be concatenated
// this will allow 10_000_000 instance indexes
const MYSQL_GROUP_CONCAT_MAX_LEN = 78888889

func Connect(
	logger lager.Logger,
	driverName,
	databaseConnectionString,
	sqlCACertFile string,
	sqlEnableIdentityVerification bool,
) (*sql.DB, error) {
	connString := addTLSParams(logger, driverName, databaseConnectionString, sqlCACertFile, sqlEnableIdentityVerification)

	if driverName == "postgres" {
		driverName = "pgx"
	}

	return sql.Open(driverName, connString)
}

// addTLSParams appends necessary extra parameters to the
// connection string if tls verifications is enabled.  If
// sqlEnableIdentityVerification is true, turn on hostname/identity
// verification, otherwise only ensure that the server certificate is signed by
// one of the CAs in sqlCACertFile.
func addTLSParams(
	logger lager.Logger,
	driverName,
	databaseConnectionString,
	sqlCACertFile string,
	sqlEnableIdentityVerification bool,
) string {

	switch driverName {
	case "mysql":
		cfg, err := mysql.ParseDSN(databaseConnectionString)
		if err != nil {
			logger.Fatal("invalid-db-connection-string", err, lager.Data{"connection-string": databaseConnectionString})
		}

		tlsConfig := generateTLSConfig(logger, sqlCACertFile, sqlEnableIdentityVerification)
		if tlsConfig != nil {
			err = mysql.RegisterTLSConfig("bbs-tls", tlsConfig)
			if err != nil {
				logger.Fatal("cannot-register-tls-config", err)
			}
			cfg.TLSConfig = "bbs-tls"
		}

		cfg.Timeout = 10 * time.Minute
		cfg.ReadTimeout = 10 * time.Minute
		cfg.WriteTimeout = 10 * time.Minute
		cfg.Params = map[string]string{
			"group_concat_max_len": strconv.Itoa(MYSQL_GROUP_CONCAT_MAX_LEN),
		}
		databaseConnectionString = cfg.FormatDSN()
	case "postgres":
		config, err := pgx.ParseConnectionString(databaseConnectionString)
		if err != nil {
			logger.Fatal("invalid-db-connection-string", err, lager.Data{"connection-string": databaseConnectionString})
		}

		tlsConfig := generateTLSConfig(logger, sqlCACertFile, sqlEnableIdentityVerification)
		config.TLSConfig = tlsConfig

		driverConfig := &stdlib.DriverConfig{ConnConfig: config}
		stdlib.RegisterDriverConfig(driverConfig)
		return driverConfig.ConnectionString(databaseConnectionString)

	default:
		logger.Fatal("invalid-driver-name", nil, lager.Data{"driver-name": driverName})
	}

	return databaseConnectionString
}

func generateTLSConfig(logger lager.Logger, sqlCACertPath string, sqlEnableIdentityVerification bool) *tls.Config {
	var tlsConfig *tls.Config

	if sqlCACertPath == "" {
		return tlsConfig
	}

	certBytes, err := ioutil.ReadFile(sqlCACertPath)
	if err != nil {
		logger.Fatal("failed-to-read-sql-ca-file", err)
	}

	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(certBytes); !ok {
		logger.Fatal("failed-to-parse-sql-ca", err)
	}

	if sqlEnableIdentityVerification {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: false,
			RootCAs:            caCertPool,
		}
	} else {
		tlsConfig = &tls.Config{
			InsecureSkipVerify:    true,
			RootCAs:               caCertPool,
			VerifyPeerCertificate: generateCustomTLSVerificationFunction(caCertPool),
		}
	}

	return tlsConfig
}

func generateCustomTLSVerificationFunction(caCertPool *x509.CertPool) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		opts := x509.VerifyOptions{
			Roots:         caCertPool,
			CurrentTime:   time.Now(),
			DNSName:       "",
			Intermediates: x509.NewCertPool(),
		}

		certs := make([]*x509.Certificate, len(rawCerts))
		for i, rawCert := range rawCerts {
			cert, err := x509.ParseCertificate(rawCert)
			if err != nil {
				return err
			}
			certs[i] = cert
		}

		for i, cert := range certs {
			if i == 0 {
				continue
			}

			opts.Intermediates.AddCert(cert)
		}

		_, err := certs[0].Verify(opts)
		if err != nil {
			return err
		}

		return nil
	}
}
