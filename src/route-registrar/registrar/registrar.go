package registrar

import (
	"crypto/tls"
	"fmt"
	"os"
	"time"

	"code.cloudfoundry.org/routing-release/route-registrar/commandrunner"
	"code.cloudfoundry.org/routing-release/route-registrar/messagebus"
	"code.cloudfoundry.org/tlsconfig"
	uuid "github.com/nu7hatch/gouuid"

	"code.cloudfoundry.org/routing-release/route-registrar/config"
	"code.cloudfoundry.org/routing-release/route-registrar/healthchecker"

	"code.cloudfoundry.org/lager"
)

type Registrar interface {
	Run(signals <-chan os.Signal, ready chan<- struct{}) error
}

type api interface {
	RegisterRoute(route config.Route) error
	UnregisterRoute(route config.Route) error
}

type registrar struct {
	logger            lager.Logger
	config            config.Config
	healthChecker     healthchecker.HealthChecker
	messageBus        messagebus.MessageBus
	routingAPI        api
	privateInstanceId string
}

func NewRegistrar(
	clientConfig config.Config,
	healthChecker healthchecker.HealthChecker,
	logger lager.Logger,
	messageBus messagebus.MessageBus,
	routingAPI api,
) Registrar {
	aUUID, err := uuid.NewV4()
	if err != nil {
		panic(err)
	}
	return &registrar{
		config:            clientConfig,
		logger:            logger,
		privateInstanceId: aUUID.String(),
		healthChecker:     healthChecker,
		messageBus:        messageBus,
		routingAPI:        routingAPI,
	}
}

func (r *registrar) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	var err error
	var tlsConfig *tls.Config

	if r.config.NATSmTLSConfig.Enabled {
		tlsConfig, err = tlsconfig.Build(
			tlsconfig.WithInternalServiceDefaults(),
			tlsconfig.WithIdentityFromFile(r.config.NATSmTLSConfig.CertPath, r.config.NATSmTLSConfig.KeyPath),
		).Client(
			tlsconfig.WithAuthorityFromFile(r.config.NATSmTLSConfig.CAPath),
		)

		if err != nil {
			return fmt.Errorf("failed building NATS mTLS config: %s", err)
		}
	}

	if len(r.config.MessageBusServers) > 0 {
		err = r.messageBus.Connect(r.config.MessageBusServers, tlsConfig)
		if err != nil {
			return err
		}
		defer r.messageBus.Close()
	}
	close(ready)

	nohealthcheckChan := make(chan config.Route, len(r.config.Routes))
	errChan := make(chan config.Route, len(r.config.Routes))
	healthyChan := make(chan config.Route, len(r.config.Routes))
	unhealthyChan := make(chan config.Route, len(r.config.Routes))

	periodicHealthcheckCloseChans := make([]chan struct{}, len(r.config.Routes))

	for i := range periodicHealthcheckCloseChans {
		periodicHealthcheckCloseChans[i] = make(chan struct{}, len(r.config.Routes))
	}

	for i, route := range r.config.Routes {
		go r.periodicallyDetermineHealth(
			route,
			nohealthcheckChan,
			errChan,
			healthyChan,
			unhealthyChan,
			periodicHealthcheckCloseChans[i],
		)
	}

	for {
		select {
		case route := <-nohealthcheckChan:
			r.logger.Info("no healthchecker found for route", lager.Data{"route": route})

			err := r.registerRoutes(route)
			if err != nil {
				return err
			}
		case route := <-errChan:
			r.logger.Info("healthchecker errored for route", lager.Data{"route": route})

			err := r.unregisterRoutes(route)
			if err != nil {
				return err
			}
		case route := <-healthyChan:
			r.logger.Info("healthchecker returned healthy for route", lager.Data{"route": route})

			err := r.registerRoutes(route)
			if err != nil {
				return err
			}
		case route := <-unhealthyChan:
			r.logger.Info("healthchecker returned unhealthy for route", lager.Data{"route": route})

			err := r.unregisterRoutes(route)
			if err != nil {
				return err
			}
		case <-signals:
			r.logger.Info("Received signal; shutting down")

			for _, c := range periodicHealthcheckCloseChans {
				close(c)
			}

			for _, route := range r.config.Routes {
				err := r.unregisterRoutes(route)
				if err != nil {
					return err
				}
			}
			return nil
		}
	}
}

func (r registrar) periodicallyDetermineHealth(
	route config.Route,
	nohealthcheckChan chan<- config.Route,
	errChan chan<- config.Route,
	healthyChan chan<- config.Route,
	unhealthyChan chan<- config.Route,
	closeChan chan struct{},
) {
	ticker := time.NewTicker(route.RegistrationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if route.HealthCheck == nil || route.HealthCheck.ScriptPath == "" {
				nohealthcheckChan <- route
			} else {
				runner := commandrunner.NewRunner(route.HealthCheck.ScriptPath)
				healthy, err := r.healthChecker.Check(runner, route.HealthCheck.ScriptPath, route.HealthCheck.Timeout)
				if err != nil {
					errChan <- route
				} else if healthy {
					healthyChan <- route
				} else {
					unhealthyChan <- route
				}
			}
		case <-closeChan:
			return
		}
	}
}

func (r registrar) registerRoutes(route config.Route) error {
	r.logger.Info("Registering route", lager.Data{"route": route})

	var err error
	if route.Type == "tcp" {
		err = r.routingAPI.RegisterRoute(route)
	} else {
		err = r.messageBus.SendMessage("router.register", r.config.Host, route, r.privateInstanceId)
	}
	if err != nil {
		return err
	}

	r.logger.Info("Registered routes successfully")

	return nil
}

func (r registrar) unregisterRoutes(route config.Route) error {
	r.logger.Info("Unregistering route", lager.Data{"route": route})

	var err error
	if route.Type == "tcp" {
		err = r.routingAPI.UnregisterRoute(route)
	} else {
		err = r.messageBus.SendMessage("router.unregister", r.config.Host, route, r.privateInstanceId)
	}
	if err != nil {
		return err
	}

	r.logger.Info("Unregistered routes successfully")

	return nil
}
