package config_test

import (
	"code.cloudfoundry.org/routing-release/cf-tcp-router/config"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Config", func() {

	Context("when a valid config", func() {
		It("loads the config", func() {
			expectedCfg := config.Config{
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
				ReservedSystemComponentPorts: []int{8080, 8081},
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
})
