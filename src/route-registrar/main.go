package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagerflags"
	"code.cloudfoundry.org/routing-release/route-registrar/config"
	"code.cloudfoundry.org/routing-release/route-registrar/healthchecker"
	"code.cloudfoundry.org/routing-release/route-registrar/messagebus"
	"code.cloudfoundry.org/routing-release/route-registrar/registrar"
	"code.cloudfoundry.org/routing-release/route-registrar/routingapi"
	"code.cloudfoundry.org/routing-release/routing-api"
	"code.cloudfoundry.org/tlsconfig"
	uaaclient "code.cloudfoundry.org/uaa-go-client"
	uaaconfig "code.cloudfoundry.org/uaa-go-client/config"

	"github.com/tedsuo/ifrit"
)

func main() {
	var configPath string
	flags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	pidfile := flags.String("pidfile", "", "Path to pid file")
	lagerflags.AddFlags(flags)

	flags.StringVar(&configPath, "configPath", "", "path to configuration file with json encoded content")
	flags.Set("configPath", "registrar_settings.yml")

	flags.Parse(os.Args[1:])

	logger, _ := lagerflags.New("Route Registrar")

	logger.Info("Initializing")

	configSchema, err := config.NewConfigSchemaFromFile(configPath)
	if err != nil {
		logger.Fatal("error parsing file: %s\n", err)
	}

	c, err := configSchema.ToConfig()
	if err != nil {
		log.Fatalln(err)
	}

	hc := healthchecker.NewHealthChecker(logger)

	logger.Info("creating nats connection")
	messageBus := messagebus.NewMessageBus(logger)

	var routingAPI *routingapi.RoutingAPI
	if c.RoutingAPI.APIURL != "" {
		logger.Info("creating routing API connection")
		clk := clock.NewClock()
		uaaConf := &uaaconfig.Config{
			UaaEndpoint:           c.RoutingAPI.OAuthURL,
			SkipVerification:      c.RoutingAPI.SkipSSLValidation,
			ClientName:            c.RoutingAPI.ClientID,
			ClientSecret:          c.RoutingAPI.ClientSecret,
			MaxNumberOfRetries:    3,
			RetryInterval:         500 * time.Millisecond,
			ExpirationBufferInSec: 30,
			CACerts:               c.RoutingAPI.CACerts,
		}

		uaaClient, err := uaaclient.NewClient(logger, uaaConf, clk)
		if err != nil {
			log.Fatalln(err)
		}

		apiClient, err := newAPIClient(c)

		if err != nil {
			logger.Fatal("failed-to-create-tls-config", err)
		}

		routingAPI = routingapi.NewRoutingAPI(logger, uaaClient, apiClient)
	}

	r := registrar.NewRegistrar(*c, hc, logger, messageBus, routingAPI)

	if *pidfile != "" {
		pid := strconv.Itoa(os.Getpid())
		err := ioutil.WriteFile(*pidfile, []byte(pid), 0644)
		logger.Info("Writing pid", lager.Data{"pid": pid, "file": *pidfile})
		if err != nil {
			logger.Fatal(
				"error writing pid to pidfile",
				err,
				lager.Data{
					"pid":     pid,
					"pidfile": *pidfile,
				},
			)
		}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)

	logger.Info("Running")

	process := ifrit.Invoke(r)
	for {
		select {
		case s := <-sigChan:
			logger.Info("Caught signal", lager.Data{"signal": s})
			process.Signal(s)
		case err := <-process.Wait():
			if err != nil {
				logger.Fatal("Exiting with error", err)
			}
			logger.Info("Exiting without error")
			os.Exit(0)
		}
	}
}

func newAPIClient(c *config.Config) (routing_api.Client, error) {
	apiURL, err := url.Parse(c.RoutingAPI.APIURL)
	if err != nil {
		return nil, err
	}

	var client routing_api.Client

	if apiURL.Scheme == "https" {
		routingAPITLSConfig, err := tlsconfig.Build(
			tlsconfig.WithInternalServiceDefaults(),
			tlsconfig.WithIdentityFromFile(c.RoutingAPI.ClientCertificatePath, c.RoutingAPI.ClientPrivateKeyPath),
		).Client(
			tlsconfig.WithAuthorityFromFile(c.RoutingAPI.ServerCACertificatePath),
		)
		if err != nil {
			return nil, err
		}

		client = routing_api.NewClientWithTLSConfig(c.RoutingAPI.APIURL, routingAPITLSConfig)
	} else {
		client = routing_api.NewClient(c.RoutingAPI.APIURL, c.RoutingAPI.SkipSSLValidation)
	}

	return client, nil
}
