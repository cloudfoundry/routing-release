package db_test

import (
	"crypto/ecdsa"
	"crypto/x509"

	"code.cloudfoundry.org/routing-release/routing-api/config"
	"code.cloudfoundry.org/routing-release/routing-api/db"
	"code.cloudfoundry.org/routing-release/routing-api/db/fakes"
	"code.cloudfoundry.org/routing-release/routing-api/test_helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MySQLConnectionStringBuilder", func() {
	Describe("Build", func() {
		var (
			mySQLAdapter      *fakes.MySQLAdapter
			connStringBuilder *db.MySQLConnectionStringBuilder
			cfg               *config.SqlDB
		)

		BeforeEach(func() {
			mySQLAdapter = &fakes.MySQLAdapter{}

			connStringBuilder = &db.MySQLConnectionStringBuilder{MySQLAdapter: mySQLAdapter}

			cfg = &config.SqlDB{
				Username: "foo",
				Password: "bar",
				Host:     "some-host",
				Port:     12345,
				Schema:   "routing_api",
			}
		})

		Context("when skipping hostname validation", func() {
			BeforeEach(func() {
				dbCACert, _, err := test_helpers.CreateCA()
				Expect(err).ToNot(HaveOccurred())

				cfg.CACert = string(dbCACert.Raw)
				cfg.SkipHostnameValidation = true
			})
			It("builds a tls config with VerifyPeerCertificate set", func() {
				connectionString, err := connStringBuilder.Build(cfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(connectionString).To(Equal("foo:bar@tcp(some-host:12345)/routing_api?parseTime=true&tls=dbTLSCertVerify"))

				tlsConfigName, tlsConfig := mySQLAdapter.RegisterTLSConfigArgsForCall(0)
				Expect(tlsConfigName).To(Equal("dbTLSCertVerify"))
				Expect(tlsConfig.InsecureSkipVerify).To(BeTrue())
				Expect(tlsConfig.VerifyPeerCertificate).NotTo(BeNil())
			})
		})
	})

	Describe("VerifyCertificatesIgnoreHostname", func() {
		var (
			caCertPool   *x509.CertPool
			dbCACert     *x509.Certificate
			dbPrivateKey *ecdsa.PrivateKey
		)

		BeforeEach(func() {
			caCertPool = x509.NewCertPool()
			var err error
			dbCACert, dbPrivateKey, err = test_helpers.CreateCA()
			Expect(err).ToNot(HaveOccurred())
			caCertPool.AddCert(dbCACert)
		})

		It("verifies that provided certificates are valid", func() {
			dbServerCert, err := test_helpers.CreateCertificate(dbCACert, dbPrivateKey, test_helpers.IsServer)
			Expect(err).ToNot(HaveOccurred())

			err = db.VerifyCertificatesIgnoreHostname(dbServerCert.Certificate, caCertPool)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when raw certs are not parsable", func() {
			It("returns an error", func() {
				err := db.VerifyCertificatesIgnoreHostname([][]byte{
					[]byte("foo"),
					[]byte("bar"),
				}, nil)
				Expect(err.Error()).To(ContainSubstring("tls: failed to parse certificate from server: asn1: structure error: tags don't match"))
			})
		})

		Context("when verifying an expired cert", func() {
			It("returns an error", func() {
				dbOtherCACert, dbOtherPrivateKey, err := test_helpers.CreateCA()
				Expect(err).ToNot(HaveOccurred())
				caCertPool.AddCert(dbOtherCACert)
				dbOtherServerCert, err := test_helpers.CreateExpiredCertificate(dbOtherCACert, dbOtherPrivateKey, test_helpers.IsServer)
				Expect(err).ToNot(HaveOccurred())

				err = db.VerifyCertificatesIgnoreHostname(dbOtherServerCert.Certificate, caCertPool)

				Expect(err.Error()).To(ContainSubstring("x509: certificate has expired or is not yet valid"))
			})
		})
	})
})
