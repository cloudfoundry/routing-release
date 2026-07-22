package routing_table

import (
	"context"
	"errors"
	"sync"
	"time"

	"code.cloudfoundry.org/cf-tcp-router/configurer"
	"code.cloudfoundry.org/cf-tcp-router/models"
	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/lager/v3"
	routing_api "code.cloudfoundry.org/routing-api"
	apimodels "code.cloudfoundry.org/routing-api/models"
	"code.cloudfoundry.org/routing-api/uaaclient"
)

//go:generate counterfeiter -o fakes/fake_updater.go . Updater
type Updater interface {
	HandleEvent(event routing_api.TcpEvent) error
	Sync()
	Syncing() bool
	PruneStaleRoutes()
	Drain() error
}

type updater struct {
	logger            lager.Logger
	routingTable      *models.RoutingTable
	configurer        configurer.RouterConfigurer
	syncing           bool
	routingAPIClient  routing_api.Client
	uaaTokenFetcher   uaaclient.TokenFetcher
	cachedEvents      []routing_api.TcpEvent
	lock              *sync.Mutex
	klock             clock.Clock
	defaultTTL        int
	drainWaitDuration time.Duration
	isDraining        bool
}

func NewUpdater(logger lager.Logger, routingTable *models.RoutingTable, configurer configurer.RouterConfigurer,
	routingAPIClient routing_api.Client, uaaTokenFetcher uaaclient.TokenFetcher, klock clock.Clock, defaultTTL int, drainWaitDuration time.Duration) Updater {
	return &updater{
		logger:            logger,
		routingTable:      routingTable,
		configurer:        configurer,
		lock:              new(sync.Mutex),
		syncing:           false,
		routingAPIClient:  routingAPIClient,
		uaaTokenFetcher:   uaaTokenFetcher,
		cachedEvents:      nil,
		klock:             klock,
		defaultTTL:        defaultTTL,
		drainWaitDuration: drainWaitDuration,
	}
}

func (u *updater) PruneStaleRoutes() {
	logger := u.logger.Session("prune-stale-routes")
	logger.Debug("starting")

	defer func() {
		u.lock.Unlock()
		logger.Debug("completed")
	}()

	u.lock.Lock()
	u.routingTable.PruneEntries(u.defaultTTL)
}

func (u *updater) Sync() {
	logger := u.logger.Session("bulk-sync")
	logger.Debug("starting")

	tableChanged := false
	defer func() {
		u.lock.Lock()
		if u.applyCachedEvents(logger) {
			tableChanged = true
		}
		if tableChanged {
			_ = u.configurer.Configure(*u.routingTable, u.isDraining)
			logger.Debug("applied-fetched-routes-to-routing-table", lager.Data{"size": u.routingTable.Size()})
		}
		u.syncing = false
		u.cachedEvents = nil
		u.lock.Unlock()
		logger.Debug("completed")
	}()

	u.lock.Lock()
	u.syncing = true
	u.cachedEvents = []routing_api.TcpEvent{}
	u.lock.Unlock()

	useCachedToken := true
	var err error
	var tcpRouteMappings []apimodels.TcpRouteMapping
	for count := 0; count < 2; count++ {
		token, tokenErr := u.uaaTokenFetcher.FetchToken(context.Background(), !useCachedToken)
		if tokenErr != nil {
			logger.Error("error-fetching-token", tokenErr)
			return
		}
		u.routingAPIClient.SetToken(token.AccessToken)
		tcpRouteMappings, err = u.routingAPIClient.TcpRouteMappings()
		if err != nil {
			logger.Error("error-fetching-routes", err)
			if err.Error() == "unauthorized" {
				useCachedToken = false
				logger.Info("retrying-sync")
			} else {
				return
			}
		} else {
			break
		}
	}
	logger.Debug("fetched-tcp-routes", lager.Data{"num-routes": len(tcpRouteMappings)})

	if err == nil {
		freshRoutingTable := models.NewRoutingTableWithSession(logger, "fresh-routing-table")

		for _, routeMapping := range tcpRouteMappings {
			routingKey, backendServerInfo := u.toRoutingTableEntry(logger, routeMapping)
			logger.Debug("creating-routing-table-entry", lager.Data{"key": routingKey, "value": backendServerInfo})
			if u.routingTable.UpsertBackendServerKey(routingKey, backendServerInfo) {
				tableChanged = true
				logger.Debug("change-detected-for-endpoint", lager.Data{"key": routingKey, "value": backendServerInfo})
			}
			freshRoutingTable.UpsertBackendServerKey(routingKey, backendServerInfo)
		}

		if freshRoutingTable.Size() != u.routingTable.Size() {
			tableChanged = true
			logger.Debug("routing-table-size-discrepency", lager.Data{"old-table-entries": u.routingTable.Size(), "new-table-entries": freshRoutingTable.Size()})
			u.routingTable.Entries = freshRoutingTable.Entries
		}
	}
}

func (u *updater) applyCachedEvents(logger lager.Logger) bool {
	logger.Debug("applying-cached-events", lager.Data{"cache_size": len(u.cachedEvents)})
	defer logger.Debug("applied-cached-events")
	tableChanged := false
	for _, e := range u.cachedEvents {
		changeTriggered, _ := u.handleEvent(logger, e)
		if changeTriggered {
			tableChanged = true
		}
	}
	return tableChanged
}

func (u *updater) Syncing() bool {
	u.lock.Lock()
	defer u.lock.Unlock()
	return u.syncing
}

func (u *updater) IsDraining() bool {
	u.lock.Lock()
	defer u.lock.Unlock()
	return u.isDraining
}

func (u *updater) HandleEvent(event routing_api.TcpEvent) error {
	u.lock.Lock()
	defer u.lock.Unlock()

	if u.syncing {
		u.logger.Debug("caching-events")
		u.cachedEvents = append(u.cachedEvents, event)
	} else {
		_, err := u.handleEvent(u.logger, event)
		return err
	}
	return nil
}

func (u *updater) Drain() error {

	for u.Syncing() {
		u.logger.Debug("waiting-for-sync-to-finish-before-starting-drain")
		time.Sleep(100 * time.Millisecond)
	}

	if u.IsDraining() {
		u.logger.Debug("drain-already-in-progress")
		return nil
	}

	u.lock.Lock()
	u.isDraining = true
	u.lock.Unlock()

	u.logger.Info("drain-started")
	err := u.configurer.Configure(*u.routingTable, u.IsDraining())
	if err != nil {
		// if we couldn't reconfigure haproxy to gracefully drain
		// we may as well just give up and do a hard exit
		return err
	}

	u.logger.Debug("starting-drain-wait", lager.Data{"drain-wait-period": u.drainWaitDuration})
	time.Sleep(u.drainWaitDuration)
	u.logger.Debug("finished-drain-wait")

	return nil
}

func (u *updater) handleEvent(l lager.Logger, event routing_api.TcpEvent) (bool, error) {
	logger := l.Session("handle-event", lager.Data{"event": event})
	logger.Debug("starting")
	defer logger.Debug("finished")
	action := event.Action
	switch action {
	case "Upsert":
		return u.handleUpsert(logger, event.TcpRouteMapping)
	case "Delete":
		return u.handleDelete(logger, event.TcpRouteMapping)
	default:
		logger.Info("unknown-event-action")
		return false, errors.New("unknown-event-action:" + action)
	}
}

func (u *updater) toRoutingTableEntry(logger lager.Logger, routeMapping apimodels.TcpRouteMapping) (models.RoutingKey, models.BackendServerInfo) {
	logger.Debug("converting-tcp-route-mapping", lager.Data{"tcp-route": routeMapping})

	var hostname string
	if routeMapping.SniHostname != nil {
		hostname = *routeMapping.SniHostname
	}

	var sniRewriteHostname string
	if routeMapping.SniRewriteHostname != nil {
		sniRewriteHostname = *routeMapping.SniRewriteHostname
	}

	routingKey := models.RoutingKey{
		Port:        routeMapping.ExternalPort,
		SniHostname: models.SniHostname(hostname),
	}

	var ttl int
	if routeMapping.TTL != nil {
		ttl = *routeMapping.TTL
	}

	backendServerInfo := models.BackendServerInfo{
		Address:              routeMapping.HostIP,
		Port:                 routeMapping.HostPort,
		TLSPort:              routeMapping.HostTLSPort,
		InstanceID:           routeMapping.InstanceId,
		ModificationTag:      routeMapping.ModificationTag,
		TTL:                  ttl,
		TerminateFrontendTLS: routeMapping.TerminateFrontendTLS,
		ALPNs:                routeMapping.ALPNs,
		SniRewriteHostname:   sniRewriteHostname,
		EnableBackendMTLS:    routeMapping.EnableBackendMTLS,
	}
	return routingKey, backendServerInfo
}

func (u *updater) handleUpsert(logger lager.Logger, routeMapping apimodels.TcpRouteMapping) (bool, error) {
	routingKey, backendServerInfo := u.toRoutingTableEntry(logger, routeMapping)
	tableChanged := u.routingTable.UpsertBackendServerKey(routingKey, backendServerInfo)
	if tableChanged && !u.syncing {
		logger.Debug("calling-configurer")
		return true, u.configurer.Configure(*u.routingTable, u.isDraining) // called from HandleEvent which already has a lock, so don't need to use IsDraining() here
	}

	return tableChanged, nil
}

func (u *updater) handleDelete(logger lager.Logger, routeMapping apimodels.TcpRouteMapping) (bool, error) {
	routingKey, backendServerInfo := u.toRoutingTableEntry(logger, routeMapping)

	tableChanged := u.routingTable.DeleteBackendServerKey(routingKey, backendServerInfo)
	if tableChanged && !u.syncing {
		logger.Debug("calling-configurer")
		return true, u.configurer.Configure(*u.routingTable, u.isDraining) // called from HandleEvent which already has a lock, so don't need to use IsDraining() here
	}

	return tableChanged, nil
}
