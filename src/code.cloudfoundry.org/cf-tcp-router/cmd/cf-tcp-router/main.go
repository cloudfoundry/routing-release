package main

import (
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"code.cloudfoundry.org/cf-tcp-router/config"
	"code.cloudfoundry.org/cf-tcp-router/configurer"
	"code.cloudfoundry.org/cf-tcp-router/configurer/haproxy"
	"code.cloudfoundry.org/cf-tcp-router/metrics_reporter"
	"code.cloudfoundry.org/cf-tcp-router/metrics_reporter/haproxy_client"
	"code.cloudfoundry.org/cf-tcp-router/models"
	"code.cloudfoundry.org/cf-tcp-router/monitor"
	"code.cloudfoundry.org/cf-tcp-router/router_group_port_checker"
	"code.cloudfoundry.org/cf-tcp-router/routing_table"
	"code.cloudfoundry.org/cf-tcp-router/syncer"
	"code.cloudfoundry.org/cf-tcp-router/watcher"
	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/debugserver"
	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagerflags"
	routing_api "code.cloudfoundry.org/routing-api"
	"code.cloudfoundry.org/routing-api/uaaclient"
	"code.cloudfoundry.org/tlsconfig"
	"github.com/cloudfoundry/dropsonde"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/sigmon"
)

const (
	defaultTokenFetchRetryInterval = 5 * time.Second
	defaultTokenFetchNumRetries    = uint(3)
)

var tcpLoadBalancer = flag.String(
	"tcpLoadBalancer",
	configurer.HaProxyConfigurer,
	"The tcp load balancer to use.",
)

var tcpLoadBalancerBaseCfg = flag.String(
	"tcpLoadBalancerBaseConfig",
	"",
	"The tcp load balancer base configuration file name. This contains the basic header information.",
)

var tcpLoadBalancerCfg = flag.String(
	"tcpLoadBalancerConfig",
	"",
	"The tcp load balancer configuration file name.",
)

var tcpLoadBalancerStatsUnixSocket = flag.String(
	"tcpLoadBalancerStatsUnixSocket",
	"/var/vcap/jobs/haproxy/config/haproxy.sock",
	"Unix domain socket for tcp load balancer",
)

var subscriptionRetryInterval = flag.Int(
	"subscriptionRetryInterval",
	5,
	"Retry interval between retries to subscribe for tcp events from routing api (in seconds)",
)

var configFile = flag.String(
	"config",
	"/var/vcap/jobs/tcp_router/config/tcp_router.yml",
	"The Router configurer yml config.",
)

var routingGroupCheckExit = flag.Bool(
	"routingGroupCheckExit",
	false,
	"Whether to exit if routing groups have conflicting ports",
)

var haproxyReloader = flag.String(
	"haproxyReloader",
	"/var/vcap/jobs/tcp_router/bin/haproxy_reloader",
	"Path to a script that reloads HAProxy.",
)

var syncInterval = flag.Duration(
	"syncInterval",
	time.Minute,
	"The interval between syncs of the routing table from routing api.",
)

var tokenFetchMaxRetries = flag.Uint(
	"tokenFetchMaxRetries",
	defaultTokenFetchNumRetries,
	"Maximum number of retries the Token Fetcher will use every time FetchToken is called",
)

var tokenFetchRetryInterval = flag.Duration(
	"tokenFetchRetryInterval",
	defaultTokenFetchRetryInterval,
	"interval to wait before TokenFetcher retries to fetch a token",
)

var tokenFetchExpirationBufferTime = flag.Uint64(
	"tokenFetchExpirationBufferTime",
	30,
	"Buffer time in seconds before the actual token expiration time, when TokenFetcher consider a token expired",
)

var statsCollectionInterval = flag.Duration(
	"statsCollectionInterval",
	time.Minute,
	"The interval between collection of stats from tcp load balancer.",
)

var dropsondePort = flag.Int(
	"dropsondePort",
	3457,
	"Port the local metron agent is listening on",
)

var staleRouteCheckInterval = flag.Duration(
	"staleRouteCheckInterval",
	30*time.Second,
	"The interval at which router checks for expired routes",
)

var defaultRouteExpiry = flag.Duration(
	"defaultRouteExpiry",
	2*time.Minute,
	"The default ttl for a route",
)

const (
	dropsondeOrigin        = "tcp-router"
	statsConnectionTimeout = 10 * time.Second
)

func main() {
	debugserver.AddFlags(flag.CommandLine)
	lagerflags.AddFlags(flag.CommandLine)
	flag.Parse()

	logger, reconfigurableSink := lagerflags.New("tcp-router")
	logger.Info("starting")
	clock := clock.NewClock()

	initializeDropsonde(logger)

	cfg, err := config.New(*configFile)
	if err != nil {
		logger.Error("failed-to-unmarshal-config-file", err)
		os.Exit(1)
	}
	if len(cfg.IsolationSegments) > 0 {
		logger.Info("retrieved-isolation-segments", map[string]interface{}{"isolation_segments": fmt.Sprintf("[%s]", strings.Join(cfg.IsolationSegments, ","))})
	}

	monitor := monitor.New(cfg.HaProxyPidFile, logger)

	routingTable := models.NewRoutingTable(logger)
	reloaderRunner := haproxy.CreateCommandRunner(*haproxyReloader, logger)
	configurer := configurer.NewConfigurer(
		logger,
		*tcpLoadBalancer,
		*tcpLoadBalancerBaseCfg,
		*tcpLoadBalancerCfg,
		monitor,
		reloaderRunner,
		cfg.BackendTLS,
	)

	// Reap child processes to prevent zombies when running in a container (BPM)
	go func() {
		signalChannel := make(chan os.Signal, 1)
		signal.Notify(signalChannel, syscall.SIGCHLD)
		for {
			sig := <-signalChannel
			if sig == syscall.SIGCHLD {
				_, err := syscall.Wait4(-1, nil, syscall.WNOHANG, nil)
				if err != nil {
					logger.Error("failed-reaping-child-process", err)
				}

			}
		}
	}()

	if defaultRouteExpiry.Seconds() > 65535 {
		logger.Error("invalid-route-expiry", errors.New("route expiry cannot be greater than 65535"))
		os.Exit(1)
	}

	if staleRouteCheckInterval.Seconds() > defaultRouteExpiry.Seconds() {
		logger.Error("invalid-stale-route-check-interval", errors.New("stale route check interval cannot be greater than route expiry"))
		os.Exit(1)
	}

	uaaConfig := uaaclient.Config{
		Port:              cfg.OAuth.Port,
		SkipSSLValidation: cfg.OAuth.SkipSSLValidation,
		ClientName:        cfg.OAuth.ClientName,
		ClientSecret:      cfg.OAuth.ClientSecret,
		CACerts:           cfg.OAuth.CACerts,
		TokenEndpoint:     cfg.OAuth.TokenEndpoint,
	}

	if *tokenFetchExpirationBufferTime > math.MaxInt64 {
		logger.Fatal("initialize-token-fetcher", fmt.Errorf("token fetch expiration buffer time exceeds MaxInt64 value"))
	}
	// #nosec G115 - overflow guarded against above
	uaaTokenFetcher, err := uaaclient.NewTokenFetcher(cfg.RoutingAPI.AuthDisabled, uaaConfig, clock, uint(*tokenFetchMaxRetries), *tokenFetchRetryInterval, int64(*tokenFetchExpirationBufferTime), logger)
	if err != nil {
		logger.Fatal("initialize-token-fetcher", err)
	}

	// Check UAA connectivity
	_, err = uaaTokenFetcher.FetchKey()
	if err != nil {
		logger.Error("failed-connecting-to-uaa", err)
		os.Exit(1)
	}

	routingAPIAddress := fmt.Sprintf("%s:%d", cfg.RoutingAPI.URI, cfg.RoutingAPI.Port)

	var routingAPIClient routing_api.Client

	tlsConfig, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentityFromFile(cfg.RoutingAPI.ClientCertificatePath, cfg.RoutingAPI.ClientPrivateKeyPath),
	).Client(
		tlsconfig.WithAuthorityFromFile(cfg.RoutingAPI.CACertificatePath),
	)
	if err != nil {
		logger.Fatal("failed-to-create-tls-config", err)
	}
	routingAPIClient = routing_api.NewClientWithTLSConfig(routingAPIAddress, tlsConfig)

	logger.Debug("creating-routing-api-client", lager.Data{"api-location": routingAPIAddress})
	portChecker := router_group_port_checker.NewPortChecker(routingAPIClient, uaaTokenFetcher)
	checkPorts(logger, portChecker, cfg)

	updater := routing_table.NewUpdater(logger, &routingTable, configurer, routingAPIClient, uaaTokenFetcher, clock, int(defaultRouteExpiry.Seconds()), cfg.DrainWaitDuration)

	ticker := clock.NewTicker(*staleRouteCheckInterval)

	go startRoutePruner(ticker, updater)

	syncChannel := make(chan struct{})
	syncRunner := syncer.New(clock, *syncInterval, syncChannel, logger)
	watcher := watcher.New(routingAPIClient, updater, uaaTokenFetcher, *subscriptionRetryInterval, syncChannel, logger)

	haproxyClient := haproxy_client.NewClient(logger, *tcpLoadBalancerStatsUnixSocket, statsConnectionTimeout)
	metricsEmitter := metrics_reporter.NewMetricsEmitter()
	metricsReporter := metrics_reporter.NewMetricsReporter(clock, haproxyClient, metricsEmitter, *statsCollectionInterval, logger)

	members := grouper.Members{
		{Name: "syncer", Runner: syncRunner},
		{Name: "metricsReporter", Runner: metricsReporter},
		{Name: "monitor", Runner: monitor},
		{Name: "watcher", Runner: watcher},
	}

	if dbgAddr := debugserver.DebugAddress(flag.CommandLine); dbgAddr != "" {
		members = append(grouper.Members{
			{Name: "debug-server", Runner: debugserver.Runner(dbgAddr, reconfigurableSink)},
		}, members...)
	}

	group := grouper.NewOrdered(os.Interrupt, members)

	process := ifrit.Invoke(sigmon.New(group, syscall.SIGUSR2))
	watcher.SetProcess(process)

	logger.Info("started")

	err = <-process.Wait()
	if err != nil {
		logger.Error("exited-with-failure", err)
		os.Exit(1)
	}

	logger.Info("exited")
}

func checkPorts(logger lager.Logger, portChecker router_group_port_checker.PortChecker, config *config.Config) {
	shouldExit, err := portChecker.Check(config.ReservedSystemComponentPorts)
	if err != nil {
		if shouldExit && *routingGroupCheckExit {
			logger.Error("router-group-port-checker-failure: Exiting now. ", err)
			os.Exit(1)
		} else if shouldExit && !*routingGroupCheckExit {
			logger.Error("router-group-port-checker-failure: WARNING! In the future this will cause tcp_router to not start.", err)
		} else {
			// this would occur if routing-api or uaa were unreachable
			logger.Error("router-group-port-checker-error:", err)
		}
	} else {
		logger.Info("router-group-port-checker-success: No conflicting router group ports.")
	}
}

func startRoutePruner(ticker clock.Ticker, updater routing_table.Updater) {
	for {
		<-ticker.C()
		updater.PruneStaleRoutes()
	}
}

func initializeDropsonde(logger lager.Logger) {
	dropsondeDestination := fmt.Sprintf("localhost:%d", *dropsondePort)
	err := dropsonde.Initialize(dropsondeDestination, dropsondeOrigin)
	if err != nil {
		logger.Error("failed-to-initialize-dropsonde", err)
	}
}
