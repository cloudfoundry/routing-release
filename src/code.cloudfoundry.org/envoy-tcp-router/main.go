package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/envoy-tcp-router/controlplane"
	"code.cloudfoundry.org/lager/v3"
	routing_api "code.cloudfoundry.org/routing-api"
	"code.cloudfoundry.org/routing-api/uaaclient"
	"code.cloudfoundry.org/tlsconfig"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
)

func main() {
	xdsPort := flag.Int("xds-port", 18000, "xDS server port")
	routingAPIURL := flag.String("routing-api-url", "https://routing-api.service.cf.internal:3001", "Routing API URL")
	nodeID := flag.String("node-id", "envoy-tcp-router", "Node ID for xDS")
	refreshInterval := flag.Duration("refresh-interval", 30*time.Second, "Interval to refresh routes from Routing API")
	caFile := flag.String("routing-api-ca-cert", "", "CA cert file for Routing API mTLS")
	clientCertFile := flag.String("routing-api-client-cert", "", "Client cert file for Routing API mTLS")
	clientKeyFile := flag.String("routing-api-client-key", "", "Client key file for Routing API mTLS")
	uaaClientSecret := flag.String("uaa-client-secret", "", "UAA client secret")
	uaaURL := flag.String("uaa-url", "uaa.service.cf.internal", "UAA hostname")
	uaaPort := flag.Uint("uaa-port", 8443, "UAA TLS port")
	uaaCACert := flag.String("uaa-ca-cert", "", "CA cert file for UAA TLS")
	authDisabled := flag.Bool("routing-api-auth-disabled", false, "Disable UAA auth (dev mode)")
	flag.Parse()

	lgr := lager.NewLogger("envoy-tcp-router")
	lgr.RegisterSink(lager.NewWriterSink(os.Stdout, lager.INFO))

	uaaCfg := uaaclient.Config{
		Port:          uint16(*uaaPort),
		ClientName:    "tcp_router",
		ClientSecret:  *uaaClientSecret,
		CACerts:       *uaaCACert,
		TokenEndpoint: *uaaURL,
	}
	clk := clock.NewClock()
	uaaTokenFetcher, err := uaaclient.NewTokenFetcher(*authDisabled, uaaCfg, clk, 3, 5*time.Second, 30, lgr)
	if err != nil {
		log.Fatalf("Failed to create UAA token fetcher: %v", err)
	}

	cp := controlplane.NewControlPlane(*nodeID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start xDS server
	go func() {
		if err := cp.Run(ctx, *xdsPort); err != nil {
			log.Fatalf("Failed to run control plane: %v", err)
		}
	}()

	// Build mTLS config for Routing API
	tlsCfg, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentityFromFile(*clientCertFile, *clientKeyFile),
	).Client(
		tlsconfig.WithAuthorityFromFile(*caFile),
	)
	if err != nil {
		log.Fatalf("Failed to build TLS config for Routing API: %v", err)
	}

	client := routing_api.NewClientWithTLSConfig(*routingAPIURL, tlsCfg)

	// Periodically refresh routes
	ticker := time.NewTicker(*refreshInterval)
	defer ticker.Stop()

	canUseCachedToken := true

	updateRoutes := func() {
		token, err := uaaTokenFetcher.FetchToken(ctx, !canUseCachedToken)
		if err != nil {
			fmt.Printf("[Main] Error fetching UAA token: %v\n", err)
			canUseCachedToken = false
			return
		}
		client.SetToken(token.AccessToken)

		routes, err := client.TcpRouteMappings()
		if err != nil {
			fmt.Printf("[Main] Error fetching routes: %v\n", err)
			if err.Error() == "unauthorized" {
				canUseCachedToken = false
			}
			return
		}
		canUseCachedToken = true

		fmt.Printf("[Main] Fetched %d TCP routes\n", len(routes))

		snapshot, err := controlplane.Translate(routes)
		if err != nil {
			fmt.Printf("[Main] Error translating routes: %v\n", err)
			return
		}

		if err := cp.SetSnapshot(*nodeID, snapshot); err != nil {
			fmt.Printf("[Main] Error setting snapshot: %v\n", err)
			return
		}
		fmt.Printf("[Main] Updated xDS snapshot (version %s)\n", snapshot.GetVersion(resource.ListenerType))
	}

	// Initial update
	updateRoutes()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			updateRoutes()
		case sig := <-sigCh:
			fmt.Printf("[Main] Received signal %v, shutting down...\n", sig)
			return
		case <-ctx.Done():
			return
		}
	}
}
