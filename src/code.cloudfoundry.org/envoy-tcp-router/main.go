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

	"code.cloudfoundry.org/envoy-tcp-router/controlplane"
	routing_api "code.cloudfoundry.org/routing-api"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
)

func main() {
	xdsPort := flag.Int("xds-port", 18000, "xDS server port")
	routingAPIURL := flag.String("routing-api-url", "https://routing-api.service.cf.internal:8080", "Routing API URL")
	nodeID := flag.String("node-id", "envoy-tcp-router", "Node ID for xDS")
	filterPath := flag.String("filter-path", "/var/vcap/packages/envoy-tcp-router/filter.so", "Path to the Go filter shared library")
	refreshInterval := flag.Duration("refresh-interval", 30*time.Second, "Interval to refresh routes from Routing API")
	flag.Parse()

	cp := controlplane.NewControlPlane(*nodeID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start xDS server
	go func() {
		if err := cp.Run(ctx, *xdsPort); err != nil {
			log.Fatalf("Failed to run control plane: %v", err)
		}
	}()

	// Initialize Routing API client
	// Note: In a real BOSH deployment, we'd need TLS config and auth token
	client := routing_api.NewClient(*routingAPIURL, true)

	// Periodically refresh routes
	ticker := time.NewTicker(*refreshInterval)
	defer ticker.Stop()

	updateRoutes := func() {
		routes, err := client.TcpRouteMappings()
		if err != nil {
			fmt.Printf("[Main] Error fetching routes: %v\n", err)
			return
		}

		fmt.Printf("[Main] Fetched %d TCP routes\n", len(routes))

		snapshot, err := controlplane.Translate(routes, *filterPath)
		if err != nil {
			fmt.Printf("[Main] Error translating routes: %v\n", err)
			return
		}

		if err := cp.SetSnapshot(*nodeID, snapshot); err != nil {
			fmt.Printf("[Main] Error setting snapshot: %v\n", err)
			return
		}
		fmt.Printf("[Main] Updated xDS snapshot (version %s)\n", snapshot.GetVersion(resource.ClusterType))
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
