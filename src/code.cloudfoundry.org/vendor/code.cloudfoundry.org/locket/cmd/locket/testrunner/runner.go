package testrunner

import (
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"time"

	"code.cloudfoundry.org/durationjson"
	"code.cloudfoundry.org/lager/lagerflags"
	"code.cloudfoundry.org/locket"
	"code.cloudfoundry.org/locket/cmd/locket/certauthority"
	"code.cloudfoundry.org/locket/cmd/locket/config"
	"code.cloudfoundry.org/tlsconfig"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit/ginkgomon"
)

var (
	caCertFile, certFile, keyFile string
)

func init() {
	certDepot, err := ioutil.TempDir("", "cert-depot")
	if err != nil {
		panic(err)
	}
	certAuthority, err := certauthority.NewCertAuthority(certDepot, "ca")
	if err != nil {
		panic(err)
	}
	_, caCertFile = certAuthority.CAAndKey()
	keyFile, certFile, err = certAuthority.GenerateSelfSignedCertAndKey("locket", []string{"localhost"}, false)
	if err != nil {
		panic(err)
	}
}

func NewLocketRunner(locketBinPath string, fs ...func(cfg *config.LocketConfig)) *ginkgomon.Runner {

	cfg := &config.LocketConfig{
		LagerConfig: lagerflags.LagerConfig{
			LogLevel:   lagerflags.INFO,
			TimeFormat: lagerflags.FormatUnixEpoch,
		},
		DatabaseDriver: "mysql",
		ReportInterval: durationjson.Duration(1 * time.Minute),
		CaFile:         caCertFile,
		CertFile:       certFile,
		KeyFile:        keyFile,
	}

	for _, f := range fs {
		f(cfg)
	}

	locketConfig, err := ioutil.TempFile("", "locket-config")
	Expect(err).NotTo(HaveOccurred())

	locketConfigFilePath := locketConfig.Name()

	encoder := json.NewEncoder(locketConfig)
	err = encoder.Encode(cfg)
	Expect(err).NotTo(HaveOccurred())
	Expect(locketConfig.Close()).To(Succeed())

	return ginkgomon.New(ginkgomon.Config{
		Name:              "locket",
		StartCheck:        "locket.started",
		StartCheckTimeout: 10 * time.Second,
		Command:           exec.Command(locketBinPath, "-config="+locketConfigFilePath),
		Cleanup: func() {
			os.RemoveAll(locketConfigFilePath)
		},
	})
}

func LocketClientTLSConfig() *tls.Config {
	tlsConfig, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentityFromFile(certFile, keyFile),
	).Client(tlsconfig.WithAuthorityFromFile(caCertFile))
	Expect(err).NotTo(HaveOccurred())
	return tlsConfig
}

func ClientLocketConfig() locket.ClientLocketConfig {
	return locket.ClientLocketConfig{
		LocketCACertFile:     caCertFile,
		LocketClientCertFile: certFile,
		LocketClientKeyFile:  keyFile,
	}
}
