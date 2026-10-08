package registrar

import (
	"crypto/tls"
	"fmt"
	"os"
	"time"

	"code.cloudfoundry.org/tlsconfig"
	uuid "github.com/nu7hatch/gouuid"
	"github.com/tedsuo/ifrit"

	"code.cloudfoundry.org/route-registrar/commandrunner"
	"code.cloudfoundry.org/route-registrar/config"
	"code.cloudfoundry.org/route-registrar/healthchecker"
	"code.cloudfoundry.org/route-registrar/messagebus"

	"code.cloudfoundry.org/lager/v3"
)

type Registrar interface {
	Run(signals <-chan os.Signal, ready chan<- struct{}) error
}

type api interface {
	RegisterRoute(route config.Route) error
	UnregisterRoute(route config.Route) error
}

type registrar struct {
	logger                         lager.Logger
	config                         config.Config
	healthChecker                  healthchecker.HealthChecker
	messageBus                     messagebus.MessageBus
	routingAPI                     api
	privateInstanceId              string
	dynamicConfigDiscoveryInterval time.Duration
}

func NewRegistrar(
	clientConfig config.Config,
	healthChecker healthchecker.HealthChecker,
	logger lager.Logger,
	messageBus messagebus.MessageBus,
	routingAPI api,
	dynamicConfigDiscoveryInterval time.Duration,
) Registrar {
	aUUID, err := uuid.NewV4()
	if err != nil {
		panic(err)
	}
	return &registrar{
		config:                         clientConfig,
		logger:                         logger,
		privateInstanceId:              aUUID.String(),
		healthChecker:                  healthChecker,
		messageBus:                     messageBus,
		routingAPI:                     routingAPI,
		dynamicConfigDiscoveryInterval: dynamicConfigDiscoveryInterval,
	}
}

func (r *registrar) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	var err error
	var tlsConfig *tls.Config

	tlsConfig, err = tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentityFromFile(r.config.NATSmTLSConfig.CertPath, r.config.NATSmTLSConfig.KeyPath),
	).Client(
		tlsconfig.WithAuthorityFromFile(r.config.NATSmTLSConfig.CAPath),
	)

	if err != nil {
		return fmt.Errorf("failed building NATS mTLS config: %s", err)
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

	periodicHealthcheckCloseChans := &PeriodicHealthcheckCloseChans{}

	for _, route := range r.config.Routes {
		closeChan := periodicHealthcheckCloseChans.Add(route)

		go r.periodicallyDetermineHealth(
			route,
			nohealthcheckChan,
			errChan,
			healthyChan,
			unhealthyChan,
			closeChan,
		)
	}

	routeDiscovered := make(chan config.Route)
	routeRemoved := make(chan config.Route)

	var routesConfigWatcher ifrit.Runner
	if len(r.config.DynamicConfigGlobs) > 0 {
		routesConfigWatcher = NewRoutesConfigWatcher(r.logger, r.dynamicConfigDiscoveryInterval, r.config.DynamicConfigGlobs, r.config.Host, routeDiscovered, routeRemoved)
	} else {
		routesConfigWatcher = NewNoopRoutesConfigWatcher()
	}

	routesConfigWatcherProcess := ifrit.Background(routesConfigWatcher)

	unregistrationCount := map[string]int{}

	routesConfigWatcherChannel := routesConfigWatcherProcess.Wait()

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

			routeKey := generateRouteKey(route)
			if unregistrationCount[routeKey] < r.config.UnregistrationMessageLimit {
				err := r.unregisterRoutes(route)
				if err != nil {
					return err
				}

				unregistrationCount[routeKey]++
			}
		case route := <-healthyChan:
			r.logger.Info("healthchecker returned healthy for route", lager.Data{"route": route})

			routeKey := generateRouteKey(route)

			err := r.registerRoutes(route)
			if err != nil {
				return err
			}

			unregistrationCount[routeKey] = 0
		case route := <-unhealthyChan:
			r.logger.Info("healthchecker returned unhealthy for route", lager.Data{"route": route})

			routeKey := generateRouteKey(route)
			if unregistrationCount[routeKey] < r.config.UnregistrationMessageLimit {
				err := r.unregisterRoutes(route)
				if err != nil {
					return err
				}

				unregistrationCount[routeKey]++
			}
		case route := <-routeDiscovered:
			r.logger.Info("discovered route", lager.Data{"route": route})

			closeChan := periodicHealthcheckCloseChans.Add(route)

			go r.periodicallyDetermineHealth(
				route,
				nohealthcheckChan,
				errChan,
				healthyChan,
				unhealthyChan,
				closeChan,
			)

		case route := <-routeRemoved:
			r.logger.Info("route removed", lager.Data{"route": route})
			periodicHealthcheckCloseChans.CloseForRoute(route)

			routeKey := generateRouteKey(route)
			if unregistrationCount[routeKey] < r.config.UnregistrationMessageLimit {
				err := r.unregisterRoutes(route)
				if err != nil {
					return err
				}

				unregistrationCount[routeKey]++
			}

		case err := <-routesConfigWatcherChannel:
			if err != nil {
				r.logger.Error("config watcher failed", err)
				return err
			}

		case s := <-signals:
			r.logger.Info("Received signal; shutting down")

			routesConfigWatcherProcess.Signal(s)

			periodicHealthcheckCloseChans.CloseAll()

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

	// fire ticker on process startup
	r.determineHealth(route, nohealthcheckChan, errChan, healthyChan, unhealthyChan)
	for {
		select {
		case <-ticker.C:
			r.determineHealth(route, nohealthcheckChan, errChan, healthyChan, unhealthyChan)
		case <-closeChan:
			return
		}
	}
}

func (r registrar) determineHealth(route config.Route, nohealthcheckChan chan<- config.Route, errChan chan<- config.Route, healthyChan chan<- config.Route, unhealthyChan chan<- config.Route) {
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
}

func (r registrar) registerRoutes(route config.Route) error {
	r.logger.Info("Registering route", lager.Data{"route": route})

	var err error
	if route.Type == "tcp" {
		err = r.routingAPI.RegisterRoute(route)
	} else {
		err = r.messageBus.SendMessage("router.register", route, r.privateInstanceId)
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
		err = r.messageBus.SendMessage("router.unregister", route, r.privateInstanceId)
	}
	if err != nil {
		return err
	}

	r.logger.Info("Unregistered routes successfully")

	return nil
}

func generateRouteKey(route config.Route) string {
	routeKey := fmt.Sprintf("%v", route)
	return routeKey
}
