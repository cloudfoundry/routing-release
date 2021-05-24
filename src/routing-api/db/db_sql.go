package db

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"code.cloudfoundry.org/eventhub"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/routing-release/routing-api/config"
	"code.cloudfoundry.org/routing-release/routing-api/models"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/postgres"
)

//go:generate counterfeiter -o fakes/fake_db.go . DB
type DB interface {
	ReadRoutes() ([]models.Route, error)
	SaveRoute(route models.Route) error
	DeleteRoute(route models.Route) error

	ReadTcpRouteMappings() ([]models.TcpRouteMapping, error)
	ReadFilteredTcpRouteMappings(columnName string, values []string) ([]models.TcpRouteMapping, error)
	SaveTcpRouteMapping(tcpMapping models.TcpRouteMapping) error
	DeleteTcpRouteMapping(tcpMapping models.TcpRouteMapping) error

	ReadRouterGroups() (models.RouterGroups, error)
	ReadRouterGroup(guid string) (models.RouterGroup, error)
	DeleteRouterGroup(guid string) error
	ReadRouterGroupByName(name string) (models.RouterGroup, error)
	SaveRouterGroup(routerGroup models.RouterGroup) error

	CancelWatches()
	WatchChanges(watchType string) (<-chan Event, <-chan error, context.CancelFunc)

	LockRouterGroupReads()
	LockRouterGroupWrites()
	UnlockRouterGroupReads()
	UnlockRouterGroupWrites()
}

const (
	TCP_MAPPING_BASE_KEY  string = "/v1/tcp_routes/router_groups"
	HTTP_ROUTE_BASE_KEY   string = "/routes"
	ROUTER_GROUP_BASE_KEY string = "/v1/router_groups"
	defaultDialTimeout           = 30 * time.Second
	maxRetries                   = 3
	TCP_WATCH             string = "tcp-watch"
	HTTP_WATCH            string = "http-watch"
	ROUTER_GROUP_WATCH    string = "router-group-watch"
)

const backupError = "Database unavailable due to backup or restore"

type rwLocker struct {
	readLock  uint32
	writeLock uint32
}

func (l *rwLocker) isReadLocked() bool {
	return atomic.LoadUint32(&l.readLock) != 0
}

func (l *rwLocker) isWriteLocked() bool {
	return atomic.LoadUint32(&l.writeLock) != 0
}

func (l *rwLocker) lockReads() {
	atomic.StoreUint32(&l.readLock, 1)
}

func (l *rwLocker) lockWrites() {
	atomic.StoreUint32(&l.writeLock, 1)
}

func (l *rwLocker) unlockReads() {
	atomic.StoreUint32(&l.readLock, 0)
}

func (l *rwLocker) unlockWrites() {
	atomic.StoreUint32(&l.writeLock, 0)
}

type SqlDB struct {
	Client       Client
	tcpEventHub  eventhub.Hub
	httpEventHub eventhub.Hub
	locker       *rwLocker
}

var DeleteRouteError = DBError{Type: KeyNotFound, Message: "Delete Fails: Route does not exist"}
var DeleteRouterGroupError = DBError{Type: KeyNotFound, Message: "Delete Fails: Router Group does not exist"}

func NewSqlDB(cfg *config.SqlDB) (*SqlDB, error) {
	if cfg == nil {
		return nil, errors.New("SQL configuration cannot be nil")
	}

	if cfg.Type != "mysql" && cfg.Type != "postgres" {
		return &SqlDB{}, errors.New(fmt.Sprintf("Unknown type %s", cfg.Type))
	}

	connStr, err := ConnectionString(cfg)
	if err != nil {
		return nil, err
	}

	db, err := gorm.Open(cfg.Type, connStr)
	if err != nil {
		return nil, err
	}

	db.DB().SetMaxIdleConns(cfg.MaxIdleConns)
	db.DB().SetMaxOpenConns(cfg.MaxOpenConns)
	connMaxLifetime := time.Duration(cfg.ConnMaxLifetime) * time.Second
	db.DB().SetConnMaxLifetime(connMaxLifetime)

	tcpEventHub := eventhub.NewNonBlocking(1024)
	httpEventHub := eventhub.NewNonBlocking(1024)

	return &SqlDB{
		Client:       NewGormClient(db),
		tcpEventHub:  tcpEventHub,
		httpEventHub: httpEventHub,
		locker:       &rwLocker{},
	}, nil
}

func (s *SqlDB) CleanupRoutes(logger lager.Logger, pruningInterval time.Duration, signals <-chan os.Signal) {
	var tcpInFlight, httpInFlight int32
	pruningTicker := time.NewTicker(pruningInterval)
	for {
		select {
		case <-pruningTicker.C:
			if atomic.CompareAndSwapInt32(&tcpInFlight, 0, 1) {
				go func() {
					var tcpRoutes []models.TcpRouteMapping
					err := s.Client.Find(&tcpRoutes, "expires_at < ?", time.Now())
					if err != nil {
						logger.Error("failed-to-prune-tcp-routes", err)
						return
					}
					guids := make([]string, len(tcpRoutes))
					for _, route := range tcpRoutes {
						guids = append(guids, route.Guid)
					}
					rowsAffected, err := s.Client.Delete(models.TcpRouteMapping{}, "guid in (?)", guids)
					if err != nil {
						logger.Error("failed-to-prune-tcp-routes", err)
						return
					}
					for _, route := range tcpRoutes {
						err = s.emitEvent(ExpireEvent, route)
						if err != nil {
							logger.Error("failed-to-emit-expire-tcp-event", err)
						}
					}

					logger.Info("successfully-finished-pruning-tcp-routes", lager.Data{"rowsAffected": rowsAffected})
					atomic.StoreInt32(&tcpInFlight, 0)
				}()
			}

			if atomic.CompareAndSwapInt32(&httpInFlight, 0, 1) {
				go func() {
					var httpRoutes []models.Route
					err := s.Client.Find(&httpRoutes, "expires_at < ?", time.Now())
					if err != nil {
						logger.Error("failed-to-prune-http-routes", err)
						return
					}
					guids := make([]string, len(httpRoutes))
					for _, route := range httpRoutes {
						guids = append(guids, route.Guid)
					}
					rowsAffected, err := s.Client.Delete(models.Route{}, "guid in (?)", guids)
					if err != nil {
						logger.Error("failed-to-prune-http-routes", err)
						return
					}
					for _, route := range httpRoutes {
						err = s.emitEvent(ExpireEvent, route)
						if err != nil {
							logger.Error("failed-to-emit-expire-http-event", err)
						}
					}

					logger.Info("successfully-finished-pruning-http-routes", lager.Data{"rowsAffected": rowsAffected})
					atomic.StoreInt32(&httpInFlight, 0)
				}()
			}
		case <-signals:
			return
		}
	}
}

func (s *SqlDB) ReadRouterGroups() (models.RouterGroups, error) {
	if s.locker.isReadLocked() {
		return models.RouterGroups{}, errors.New(backupError)
	}
	routerGroupsDB := models.RouterGroupsDB{}
	routerGroups := models.RouterGroups{}
	err := s.Client.Find(&routerGroupsDB)
	if err == nil {
		routerGroups = routerGroupsDB.ToRouterGroups()
	}

	return routerGroups, err
}

func (s *SqlDB) ReadRouterGroup(guid string) (models.RouterGroup, error) {
	if s.locker.isReadLocked() {
		return models.RouterGroup{}, errors.New(backupError)
	}
	routerGroupDB := models.RouterGroupDB{}
	routerGroup := models.RouterGroup{}
	err := s.Client.Where("guid = ?", guid).First(&routerGroupDB)
	if err == nil {
		routerGroup = routerGroupDB.ToRouterGroup()
	}

	if recordNotFound(err) {
		err = nil
	}
	return routerGroup, err
}

func (s *SqlDB) ReadRouterGroupByName(name string) (models.RouterGroup, error) {
	if s.locker.isReadLocked() {
		return models.RouterGroup{}, errors.New(backupError)
	}
	routerGroupDB := models.RouterGroupDB{}
	routerGroup := models.RouterGroup{}
	err := s.Client.Where("name = ?", name).First(&routerGroupDB)
	if err == nil {
		routerGroup = routerGroupDB.ToRouterGroup()
	}

	if recordNotFound(err) {
		err = nil
	}
	return routerGroup, err
}

func (s *SqlDB) SaveRouterGroup(routerGroup models.RouterGroup) error {
	if s.locker.isWriteLocked() {
		return errors.New(backupError)
	}
	existingRouterGroup, err := s.ReadRouterGroup(routerGroup.Guid)
	if err != nil {
		return err
	}

	routerGroupDB := models.NewRouterGroupDB(routerGroup)
	if existingRouterGroup.Guid == routerGroup.Guid {
		updateRouterGroup(&existingRouterGroup, &routerGroup)
		routerGroupDB = models.NewRouterGroupDB(existingRouterGroup)
		_, err = s.Client.Save(&routerGroupDB)
	} else {
		_, err = s.Client.Create(&routerGroupDB)
	}

	return err
}

func (s *SqlDB) DeleteRouterGroup(guid string) error {
	if s.locker.isWriteLocked() {
		return errors.New(backupError)
	}
	routerGroup, err := s.ReadRouterGroup(guid)
	if err != nil {
		return err
	}
	if routerGroup == (models.RouterGroup{}) {
		return DeleteRouterGroupError
	}

	_, err = s.Client.Delete(&routerGroup)
	if err != nil {
		return err
	}
	return nil
}

func (s *SqlDB) LockRouterGroupReads() {
	s.locker.lockReads()
}

func (s *SqlDB) LockRouterGroupWrites() {
	s.locker.lockWrites()
}

func (s *SqlDB) UnlockRouterGroupReads() {
	s.locker.unlockReads()
}

func (s *SqlDB) UnlockRouterGroupWrites() {
	s.locker.unlockWrites()
}

func updateRouterGroup(existingRouterGroup, currentRouterGroup *models.RouterGroup) {
	if currentRouterGroup.Type != "" {
		existingRouterGroup.Type = currentRouterGroup.Type
	}
	if currentRouterGroup.Name != "" {
		existingRouterGroup.Name = currentRouterGroup.Name
	}
	existingRouterGroup.ReservablePorts = currentRouterGroup.ReservablePorts
}

func updateTcpRouteMapping(existingTcpRouteMapping models.TcpRouteMapping, currentTcpRouteMapping models.TcpRouteMapping) models.TcpRouteMapping {
	existingTcpRouteMapping.ModificationTag.Increment()
	if currentTcpRouteMapping.TTL != nil {
		existingTcpRouteMapping.TTL = currentTcpRouteMapping.TTL
	}
	existingTcpRouteMapping.IsolationSegment = currentTcpRouteMapping.IsolationSegment

	existingTcpRouteMapping.ExpiresAt = time.Now().
		Add(time.Duration(*existingTcpRouteMapping.TTL) * time.Second)
	return existingTcpRouteMapping
}

func updateRoute(existingRoute, currentRoute models.Route) models.Route {
	existingRoute.ModificationTag.Increment()
	if currentRoute.TTL != nil {
		existingRoute.TTL = currentRoute.TTL
	}

	if currentRoute.LogGuid != "" {
		existingRoute.LogGuid = currentRoute.LogGuid
	}

	existingRoute.ExpiresAt = time.Now().
		Add(time.Duration(*existingRoute.TTL) * time.Second)

	return existingRoute
}

func notImplementedError() error {
	pc, _, _, _ := runtime.Caller(1)
	fnName := runtime.FuncForPC(pc).Name()
	return errors.New(fmt.Sprintf("function not implemented: %s", fnName))
}

func (s *SqlDB) ReadRoutes() ([]models.Route, error) {
	var routes []models.Route
	now := time.Now()
	err := s.Client.Where("expires_at > ?", now).Find(&routes)
	if err != nil {
		return nil, err
	}
	return routes, err
}

func (s *SqlDB) readRoute(route models.Route) (models.Route, error) {
	var routes []models.Route
	err := s.Client.Where("route = ? and ip = ? and port = ? and route_service_url = ?",
		route.Route, route.IP, route.Port, route.RouteServiceUrl).Find(&routes)

	if err != nil {
		return route, err
	}
	count := len(routes)
	if count > 1 || count < 0 {
		return route, errors.New("Have duplicate routes")
	}
	if count == 1 {
		return routes[0], nil
	}
	return models.Route{}, nil
}

func (s *SqlDB) SaveRoute(route models.Route) error {
	existingRoute, err := s.readRoute(route)
	if err != nil {
		return err
	}

	if existingRoute != (models.Route{}) {
		newRoute := updateRoute(existingRoute, route)
		_, err = s.Client.Save(&newRoute)
		if err != nil {
			return err
		}
		return s.emitEvent(UpdateEvent, newRoute)
	}

	newRoute, err := models.NewRouteWithModel(route)
	if err != nil {
		return err
	}

	tag, err := models.NewModificationTag()
	if err != nil {
		return err
	}
	newRoute.ModificationTag = tag

	_, err = s.Client.Create(&newRoute)
	if err != nil {
		return err
	}
	return s.emitEvent(CreateEvent, newRoute)
}

func (s *SqlDB) DeleteRoute(route models.Route) error {
	route, err := s.readRoute(route)
	if err != nil {
		return err
	}
	if route == (models.Route{}) {
		return DeleteRouteError
	}

	_, err = s.Client.Delete(&route)
	if err != nil {
		return err
	}
	return s.emitEvent(DeleteEvent, route)
}

func (s *SqlDB) ReadTcpRouteMappings() ([]models.TcpRouteMapping, error) {
	var tcpRoutes []models.TcpRouteMapping
	now := time.Now()
	err := s.Client.Where("expires_at > ?", now).Find(&tcpRoutes)
	if err != nil {
		return nil, err
	}
	return tcpRoutes, nil
}

func (s *SqlDB) ReadFilteredTcpRouteMappings(columnName string, values []string) ([]models.TcpRouteMapping, error) {
	var tcpRoutes []models.TcpRouteMapping
	now := time.Now()
	err := s.Client.Where(columnName+" in (?)", values).Where("expires_at > ?", now).Find(&tcpRoutes)
	if err != nil {
		return nil, err
	}
	return tcpRoutes, nil
}

func (s *SqlDB) readTcpRouteMapping(tcpMapping models.TcpRouteMapping) (models.TcpRouteMapping, error) {
	var routes []models.TcpRouteMapping
	var tcpRoute models.TcpRouteMapping
	err := s.Client.Where("router_group_guid = ? and host_ip = ? and host_port = ? and external_port = ?",
		tcpMapping.RouterGroupGuid, tcpMapping.HostIP, tcpMapping.HostPort, tcpMapping.ExternalPort).Find(&routes)

	if err != nil {
		return tcpRoute, err
	}
	count := len(routes)
	if count > 1 || count < 0 {
		return tcpRoute, errors.New("Have duplicate tcp route mappings")
	}
	if count == 1 {
		tcpRoute = routes[0]
	}

	return tcpRoute, err
}

func (s *SqlDB) emitEvent(eventType EventType, obj interface{}) error {
	event, err := NewEventFromInterface(eventType, obj)
	if err != nil {
		return err
	}

	switch obj.(type) {
	case models.Route:
		s.httpEventHub.Emit(event)
	case models.TcpRouteMapping:
		s.tcpEventHub.Emit(event)
	default:
		return errors.New("Unknown event type")
	}
	return nil
}

func (s *SqlDB) SaveTcpRouteMapping(tcpRouteMapping models.TcpRouteMapping) error {
	existingTcpRouteMapping, err := s.readTcpRouteMapping(tcpRouteMapping)
	if err != nil {
		return err
	}

	if existingTcpRouteMapping != (models.TcpRouteMapping{}) {
		newTcpRouteMapping := updateTcpRouteMapping(existingTcpRouteMapping, tcpRouteMapping)
		_, err = s.Client.Save(&newTcpRouteMapping)
		if err != nil {
			return err
		}
		return s.emitEvent(UpdateEvent, newTcpRouteMapping)
	}

	tcpMapping, err := models.NewTcpRouteMappingWithModel(tcpRouteMapping)
	if err != nil {
		return err
	}

	tag, err := models.NewModificationTag()
	if err != nil {
		return err
	}
	tcpMapping.ModificationTag = tag

	_, err = s.Client.Create(&tcpMapping)
	if err != nil {
		return err
	}

	return s.emitEvent(CreateEvent, tcpMapping)
}

func (s *SqlDB) DeleteTcpRouteMapping(tcpMapping models.TcpRouteMapping) error {
	tcpMapping, err := s.readTcpRouteMapping(tcpMapping)
	if err != nil {
		return err
	}
	if tcpMapping == (models.TcpRouteMapping{}) {
		return DeleteRouteError
	}

	_, err = s.Client.Delete(&tcpMapping)
	if err != nil {
		return err
	}
	return s.emitEvent(DeleteEvent, tcpMapping)
}

func (s *SqlDB) Connect() error {
	return notImplementedError()
}

func (s *SqlDB) CancelWatches() {
	// This only errors if the eventhub was closed.
	_ = s.tcpEventHub.Close()
	_ = s.httpEventHub.Close()
}

func (s *SqlDB) WatchChanges(watchType string) (<-chan Event, <-chan error, context.CancelFunc) {
	var (
		sub eventhub.Source
		err error
	)
	events := make(chan Event)
	errors := make(chan error, 1)
	cancelFunc := func() {}

	switch watchType {
	case TCP_WATCH:
		sub, err = s.tcpEventHub.Subscribe()
		if err != nil {
			errors <- err
			close(events)
			close(errors)
			return events, errors, cancelFunc
		}
	case HTTP_WATCH:
		sub, err = s.httpEventHub.Subscribe()
		if err != nil {
			errors <- err
			close(events)
			close(errors)
			return events, errors, cancelFunc
		}
	default:
		err := fmt.Errorf("Invalid watch type: %s", watchType)
		errors <- err
		close(events)
		close(errors)
		return events, errors, cancelFunc
	}

	cancelFunc = func() {
		_ = sub.Close()
	}

	go dispatchWatchEvents(sub, events, errors)

	return events, errors, cancelFunc
}

func dispatchWatchEvents(sub eventhub.Source, events chan<- Event, errors chan<- error) {
	defer close(events)
	defer close(errors)
	for {
		event, err := sub.Next()
		if err != nil {
			if err == eventhub.ErrReadFromClosedSource {
				return
			}
			errors <- err
			return
		}
		watchEvent, ok := event.(Event)
		if !ok {
			errors <- fmt.Errorf("Incoming event is not a db.Event: %#v", event)
		}

		events <- watchEvent
	}
}

func recordNotFound(err error) bool {
	if err == gorm.ErrRecordNotFound {
		return true
	}
	return false
}

func ConnectionString(cfg *config.SqlDB) (string, error) {
	var connectionString string
	switch cfg.Type {
	case "mysql":
		connStringBuilder := &MySQLConnectionStringBuilder{MySQLAdapter: &MySQLAdapter{}}
		return connStringBuilder.Build(cfg)

	case "postgres":
		var queryString string
		if cfg.CACert == "" {
			queryString = "?sslmode=disable"
		} else {
			if cfg.SkipSSLValidation {
				queryString = "?sslmode=require"
			} else {
				tempDir, err := ioutil.TempDir("", "")
				if err != nil {
					return "", err
				}
				certPath := filepath.Join(tempDir, "postgres_cert.pem")
				err = ioutil.WriteFile(certPath, []byte(cfg.CACert), 0400)
				if err != nil {
					return "", err
				}
				queryString = fmt.Sprintf("?sslmode=verify-full&sslrootcert=%s", certPath)
			}
		}
		connectionString = fmt.Sprintf(
			"postgres://%s:%s@%s:%d/%s%s",
			cfg.Username,
			cfg.Password,
			cfg.Host,
			cfg.Port,
			cfg.Schema,
			queryString,
		)
	}

	return connectionString, nil
}
