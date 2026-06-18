package testrunner

import (
	"code.cloudfoundry.org/routing-api/config"
	"crypto/tls"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"gopkg.in/yaml.v3"
	"net/http"
	"os"
	"strings"
)

func WriteConfigToTempFile(conf *config.Config) string {
	bytes, err := yaml.Marshal(conf)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	tempFile, err := os.CreateTemp("", "routing_api_config.yml")
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	defer func() {
		gomega.Expect(tempFile.Close()).To(gomega.Succeed())
	}()

	_, err = tempFile.Write(bytes)
	gomega.Expect(err).ToNot(gomega.HaveOccurred())

	return tempFile.Name()
}

func getServerPort(url string) string {
	endpoints := strings.Split(url, ":")
	gomega.Expect(endpoints).To(gomega.HaveLen(3))
	return endpoints[2]
}

func SetupOauthServer(uaaServerCert tls.Certificate) (*ghttp.Server, string) {
	oAuthServer := ghttp.NewUnstartedServer()

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{uaaServerCert},
	}

	oAuthServer.HTTPTestServer.TLS = tlsConfig
	oAuthServer.AllowUnhandledRequests = true
	oAuthServer.UnhandledRequestStatusCode = http.StatusOK
	oAuthServer.HTTPTestServer.StartTLS()

	oAuthServerPort := getServerPort(oAuthServer.URL())

	return oAuthServer, oAuthServerPort
}
