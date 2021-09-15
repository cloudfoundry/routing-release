package locket

import (
	"net"
	"time"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/locket/models"
	"code.cloudfoundry.org/tlsconfig"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

type ClientLocketConfig struct {
	LocketAddress        string `json:"locket_address,omitempty" yaml:"locket_address,omitempty"`
	LocketCACertFile     string `json:"locket_ca_cert_file,omitempty" yaml:"locket_ca_cert_file,omitempty"`
	LocketClientCertFile string `json:"locket_client_cert_file,omitempty" yaml:"locket_client_cert_file,omitempty"`
	LocketClientKeyFile  string `json:"locket_client_key_file,omitempty" yaml:"locket_client_key_file,omitempty"`
}

func NewClientSkipCertVerify(logger lager.Logger, config ClientLocketConfig) (models.LocketClient, error) {
	return newClientInternal(logger, config, true)
}

func NewClient(logger lager.Logger, config ClientLocketConfig) (models.LocketClient, error) {
	return newClientInternal(logger, config, false)
}

func newClientInternal(logger lager.Logger, config ClientLocketConfig, skipCertVerify bool) (models.LocketClient, error) {
	if config.LocketAddress == "" {
		logger.Fatal("invalid-locket-config", nil)
	}

	locketTLSConfig, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentityFromFile(config.LocketClientCertFile, config.LocketClientKeyFile),
	).Client(tlsconfig.WithAuthorityFromFile(config.LocketCACertFile))
	if err != nil {
		logger.Error("failed-to-open-tls-config", err, lager.Data{"keypath": config.LocketClientKeyFile, "certpath": config.LocketClientCertFile, "capath": config.LocketCACertFile})
		return nil, err
	}
	locketTLSConfig.InsecureSkipVerify = skipCertVerify

	// TODO: test the following code when the following change is released:
	// 1. https://go-review.googlesource.com/c/go/+/115855
	// 2. https://github.com/golang/go/issues/12503
	//
	// We will need the mentioned change in order to mock the dns resolver to
	// return a list of addresses. We will also need to add a new NewClient
	// method that accepts a dialer in order to mock the ipsec (blocking) issue
	// we ran into in https://www.pivotaltracker.com/story/show/158104990
	conn, err := grpc.Dial(
		config.LocketAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(locketTLSConfig)),
		grpc.WithDialer(func(addr string, _ time.Duration) (net.Conn, error) {
			return net.DialTimeout("tcp", addr, 10*time.Second) // give at least 2 seconds per ip address (assuming there are at most 5)
		}),
		grpc.WithBlock(),
		grpc.WithTimeout(10*time.Second), // ensure that grpc won't keep retrying forever
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time: 10 * time.Second,
		}),
	)
	if err != nil {
		return nil, err
	}
	return models.NewLocketClient(conn), nil
}
