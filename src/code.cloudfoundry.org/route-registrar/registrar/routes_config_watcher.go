package registrar

import (
	"os"
	"path/filepath"
	"reflect"
	"time"

	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/route-registrar/config"
	"gopkg.in/yaml.v3"
)

type RoutesConfigSchema struct {
	Routes []config.RouteSchema `json:"routes"`
}

type routesConfigWatcher struct {
	globs               []string
	host                string
	logger              lager.Logger
	watchInterval       time.Duration
	discoveredRoutes    map[string][]config.Route
	routeDiscoveredChan chan config.Route
	routeRemovedChan    chan config.Route
}

func NewRoutesConfigWatcher(logger lager.Logger, watchInterval time.Duration, globs []string, host string, routeDiscoveredChan chan config.Route, routeRemovedChan chan config.Route) *routesConfigWatcher {
	return &routesConfigWatcher{
		globs:               globs,
		host:                host,
		logger:              logger.Session("routes-config-watcher"),
		watchInterval:       watchInterval,
		routeDiscoveredChan: routeDiscoveredChan,
		routeRemovedChan:    routeRemovedChan,
		discoveredRoutes:    map[string][]config.Route{},
	}
}

func (r *routesConfigWatcher) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	close(ready)

	timer := time.NewTicker(r.watchInterval)
	for {
		select {
		case <-timer.C:
			err := r.discoverRoutesFromConfigFiles()
			if err != nil {
				r.logger.Error("failed-to-discover-config-files", err)
				return err
			}

		case s := <-signals:
			r.logger.Info("caught-signal", lager.Data{"signal": s})
			return nil
		}
	}
}

func (r *routesConfigWatcher) discoverRoutesFromConfigFiles() error {
	allFiles := map[string]bool{}

	for _, glob := range r.globs {
		files, err := filepath.Glob(glob)
		if err != nil {
			r.logger.Error("failed-to-glob-config-files", err, lager.Data{"glob": glob})
			return err
		}

		for _, f := range files {
			allFiles[f] = true

			r.registerNewRoutesFromConfigFile(f)
		}
	}

	for f := range r.discoveredRoutes {
		if _, ok := allFiles[f]; !ok {
			r.logger.Info("removing-routes-from-config-file", lager.Data{"file": f})
			for _, route := range r.discoveredRoutes[f] {
				r.routeRemovedChan <- route
			}
			delete(r.discoveredRoutes, f)
		}
	}

	return nil
}

func (r *routesConfigWatcher) registerNewRoutesFromConfigFile(configFile string) {
	b, err := os.ReadFile(configFile)
	if err != nil {
		r.logger.Error("failed-to-read-macthed-file", err)
		return
	}
	var routesConfig RoutesConfigSchema
	err = yaml.Unmarshal(b, &routesConfig)
	if err != nil {
		r.logger.Error("failed-to-parse-file", err)
		return
	}

	if _, ok := r.discoveredRoutes[configFile]; !ok {
		r.discoveredRoutes[configFile] = []config.Route{}
	}

	configRoutes := []config.Route{}

	for i, routeSchema := range routesConfig.Routes {
		route, err := config.RouteFromSchema(routeSchema, i, r.host)
		if err != nil {
			r.logger.Error("failed-to-parse-route", err)
			continue
		}

		if route != nil {
			configRoutes = append(configRoutes, *route)

			if !containsRoute(r.discoveredRoutes[configFile], *route) {
				r.discoveredRoutes[configFile] = append(r.discoveredRoutes[configFile], *route)
				r.routeDiscoveredChan <- *route
			}
		}
	}

	keptRoutes := make([]config.Route, 0, len(r.discoveredRoutes[configFile]))
	for _, route := range r.discoveredRoutes[configFile] {
		if containsRoute(configRoutes, route) {
			keptRoutes = append(keptRoutes, route)
			continue
		}
		r.routeRemovedChan <- route
	}
	r.discoveredRoutes[configFile] = keptRoutes
}

func containsRoute(routes []config.Route, route config.Route) bool {
	for _, r := range routes {
		if reflect.DeepEqual(r, route) {
			return true
		}
	}

	return false
}

type noopRoutesConfigWatcher struct{}

func NewNoopRoutesConfigWatcher() *noopRoutesConfigWatcher {
	return &noopRoutesConfigWatcher{}
}

func (r *noopRoutesConfigWatcher) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	close(ready)

	<-signals
	return nil
}
