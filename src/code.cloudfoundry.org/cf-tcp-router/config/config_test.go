package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	tlshelpers "code.cloudfoundry.org/cf-routing-test-helpers/tls"
	"code.cloudfoundry.org/cf-tcp-router/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", Serial, func() {
	caFile := "fixtures/ca.pem"
	certAndKeyFile := "fixtures/cert_and_key.pem"
	mismatchedCertAndKeyFile := "fixtures/mismatched_cert_and_key.pem"

	BeforeEach(func() {
		// Generate a CA and move it into the correct location for the fixture
		tmpCAFile, _ := tlshelpers.GenerateCa()
		caBytes, err := os.ReadFile(tmpCAFile)
		Expect(err).ToNot(HaveOccurred())
		f, err := os.OpenFile(caFile, os.O_RDWR|os.O_CREATE, 0644)
		Expect(err).ToNot(HaveOccurred())
		_, err = f.Write(caBytes)
		Expect(err).ToNot(HaveOccurred())
		err = os.Remove(tmpCAFile)
		Expect(err).ToNot(HaveOccurred())

		// Generate a second trusted CA and add it to the fixture's CA file
		tmpCAFile2, _ := tlshelpers.GenerateCa()
		caBytes, err = os.ReadFile(tmpCAFile2)
		Expect(err).ToNot(HaveOccurred())
		_, err = f.Write(caBytes)
		Expect(err).ToNot(HaveOccurred())
		err = os.Remove(tmpCAFile2)
		Expect(err).ToNot(HaveOccurred())
		err = f.Close()
		Expect(err).ToNot(HaveOccurred())

		// Generate a client cert + key, and move it into the correct location for the fixture
		_, tmpCertFile1, tmpKeyFile1, _ := tlshelpers.GenerateCaAndMutualTlsCerts()
		cert1Bytes, err := os.ReadFile(tmpCertFile1)
		Expect(err).NotTo(HaveOccurred())
		key1Bytes, err := os.ReadFile(tmpKeyFile1)
		Expect(err).NotTo(HaveOccurred())
		os.WriteFile(certAndKeyFile, []byte(fmt.Sprintf("%s%s", string(cert1Bytes), string(key1Bytes))), 0644)
		Expect(err).NotTo(HaveOccurred())

		// Generate a second client cert + key, and move it into the correct location for the fixture
		// used for the invalid key-pair combo to fail if a key and cert do not go together
		_, _, tmpKeyFile2, _ := tlshelpers.GenerateCaAndMutualTlsCerts()
		key2Bytes, err := os.ReadFile(tmpKeyFile2)
		Expect(err).NotTo(HaveOccurred())
		os.WriteFile(mismatchedCertAndKeyFile, []byte(fmt.Sprintf("%s%s", string(cert1Bytes), string(key2Bytes))), 0644)
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() {
		os.Remove(caFile)
		os.Remove(certAndKeyFile)
		os.Remove(mismatchedCertAndKeyFile)
	})

	Context("when a valid config", func() {
		It("loads the config", func() {
			expectedCfg := config.Config{
				DrainWaitDuration: 40 * time.Second,
				OAuth: config.OAuthConfig{
					TokenEndpoint:     "uaa.service.cf.internal",
					ClientName:        "someclient",
					ClientSecret:      "somesecret",
					Port:              8443,
					SkipSSLValidation: true,
					CACerts:           "some-ca-cert",
				},
				RoutingAPI: config.RoutingAPIConfig{
					URI:          "http://routing-api.service.cf.internal",
					Port:         3000,
					AuthDisabled: false,

					ClientCertificatePath: "/a/client_cert",
					ClientPrivateKeyPath:  "/b/private_key",
					CACertificatePath:     "/c/ca_cert",
				},
				HaProxyPidFile:               "/path/to/pid/file",
				IsolationSegments:            []string{"foo-iso-seg"},
				ReservedSystemComponentPorts: []uint16{8080, 8081},
				BackendTLS: config.BackendTLSConfig{
					Enabled:              true,
					CACertificatePath:    caFile,
					ClientCertAndKeyPath: certAndKeyFile,
				},
			}
			cfg, err := config.New("fixtures/valid_config.yml")
			Expect(err).NotTo(HaveOccurred())
			Expect(*cfg).To(Equal(expectedCfg))
		})
	})

	Context("when given an invalid config", func() {
		Context("non existing config", func() {
			It("return error", func() {
				_, err := config.New("fixtures/non_existing_config.yml")
				Expect(err).To(HaveOccurred())
			})
		})
		Context("malformed YAML config", func() {
			It("return error", func() {
				_, err := config.New("fixtures/malformed_config.yml")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("When backend_tls is enabled", func() {
		Context("when the CA path is not a valid CA", func() {
			It("returns an error", func() {
				_, err := config.New("fixtures/bad_ca_config.yml")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Invalid PEM block found in file"))
			})
		})

		Context("when the Client Cert/key pair are not valid", func() {
			It("returns an error", func() {
				_, err := config.New("fixtures/bad_client_cert_config.yml")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Invalid PEM CERTIFICATE found in file"))
			})
		})

		Context("when the Client Cert/key pair are mismatched", func() {
			It("returns an error", func() {
				_, err := config.New("fixtures/mismatched_client_cert_config.yml")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Unable to validate backend TLS client cert + key in file"))
				Expect(err.Error()).To(ContainSubstring("tls: private key does not match public key"))
			})
		})
		Context("when CA path is not specified", func() {
			It("returns an error", func() {
				_, err := config.New("fixtures/no_ca.yml")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("Backend TLS was enabled but no CA certificates were specified"))
			})
		})
	})

	Context("when backend_tls is disabled", func() {
		It("does not set any of the backend_tls cert/ca values", func() {
			cfg, err := config.New("fixtures/disabled_tls.yml")
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.BackendTLS).To(Equal(config.BackendTLSConfig{
				Enabled: false,
			}))
		})
	})

	Context("when haproxy pid file is missing", func() {
		It("return error", func() {
			_, err := config.New("fixtures/no_haproxy.yml")
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when oauth section is  missing", func() {
		It("loads only routing api section", func() {
			expectedCfg := config.Config{
				RoutingAPI: config.RoutingAPIConfig{
					URI:  "http://routing-api.service.cf.internal",
					Port: 3000,
				},
				HaProxyPidFile: "/path/to/pid/file",
			}
			cfg, err := config.New("fixtures/no_oauth.yml")
			Expect(err).NotTo(HaveOccurred())
			Expect(*cfg).To(Equal(expectedCfg))
		})
	})

	Context("when oauth section has some missing fields", func() {
		It("loads config and defaults missing fields", func() {
			expectedCfg := config.Config{
				OAuth: config.OAuthConfig{
					TokenEndpoint:     "uaa.service.cf.internal",
					ClientName:        "",
					ClientSecret:      "",
					Port:              8443,
					SkipSSLValidation: true,
				},
				RoutingAPI: config.RoutingAPIConfig{
					URI:  "http://routing-api.service.cf.internal",
					Port: 3000,
				},
				HaProxyPidFile: "/path/to/pid/file",
			}
			cfg, err := config.New("fixtures/missing_oauth_fields.yml")
			Expect(err).NotTo(HaveOccurred())
			Expect(*cfg).To(Equal(expectedCfg))
		})
	})
	Context("when drain_wait is a negative number", func() {
		It("defaults to 20s", func() {
			cfg, err := config.New("fixtures/negative_drain_wait.yml")
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.DrainWaitDuration).To(Equal(20 * time.Second))

		})
	})

	Context("When frontend_tls is enabled", func() {
		var (
			tmpDir string
			cfg    *config.Config
			err    error
		)

		BeforeEach(func() {
			tmpDir = GinkgoT().TempDir()
			os.Setenv("FRONTEND_TLS_BASE_PATH", tmpDir)
		})

		AfterEach(func() {
			os.Unsetenv("FRONTEND_TLS_BASE_PATH")
		})

		Context("with valid cert and key", func() {
			BeforeEach(func() {
				cfg, err = config.New("fixtures/valid_frontend_cert.yml")
			})

			It("loads config without error", func() {
				Expect(err).NotTo(HaveOccurred())
			})

			It("adds the certs and keys to the expected directories", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.FrontendTLS).To(HaveLen(2))

				Expect(cfg.FrontendTLS[0]).To(Equal(config.FrontendTLSConfig{
					Enabled:  true,
					CertPath: filepath.Join(tmpDir, "prod"),
				}))

				Expect(cfg.FrontendTLS[1]).To(Equal(config.FrontendTLSConfig{
					Enabled:  true,
					CertPath: filepath.Join(tmpDir, "dev"),
				}))
			})

			It("writes the correct cert and key files", func() {
				for i, name := range []string{"prod", "dev"} {
					certPath := filepath.Join(tmpDir, name, name+".cert.pem")
					keyPath := filepath.Join(tmpDir, name, name+".key.pem")

					Expect(certPath).To(BeAnExistingFile())
					Expect(keyPath).To(BeAnExistingFile())

					certData, certErr := os.ReadFile(certPath)
					Expect(certErr).NotTo(HaveOccurred())
					Expect(string(certData)).To(Equal(cfg.FrontendTLSJob[i].CertChain))

					keyData, keyErr := os.ReadFile(keyPath)
					Expect(keyErr).NotTo(HaveOccurred())
					Expect(string(keyData)).To(Equal(cfg.FrontendTLSJob[i].PrivateKey))
				}
			})
		})

		Context("with invalid frontend_tls config", func() {
			It("should fail if cert_chain is missing SAN information", func() {
				_, err := config.New("fixtures/invalid_frontend_cert.yml")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("frontend_tls[0].cert_chain must include a subjectAltName extension"))
			})

		})
	})

})
