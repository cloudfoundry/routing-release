package controlplane

import (
	"fmt"
	"sort"
	"time"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	tcp_proxy_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"code.cloudfoundry.org/routing-api/models"
)

// portSniKey identifies a unique (external port, SNI hostname) group.
// Routes sharing the same key are load-balanced together in one cluster.
type portSniKey struct {
	port uint16
	sni  string // empty string when no SNI
}

// Translate converts CF TCP route mappings into an Envoy xDS snapshot.
// Routes on the same external port (and optional SNI hostname) are grouped
// into a single Envoy cluster whose LbEndpoints Envoy round-robins across,
// giving true load balancing when an app is scaled to multiple instances.
func Translate(routes []models.TcpRouteMapping) (*cache.Snapshot, error) {
	clusters := make([]types.Resource, 0)
	listeners := make([]types.Resource, 0)

	// Group routes by (port, sni) — each group becomes one cluster.
	routesByKey := make(map[portSniKey][]models.TcpRouteMapping)
	for _, r := range routes {
		sni := ""
		if r.SniHostname != nil {
			sni = *r.SniHostname
		}
		key := portSniKey{port: r.ExternalPort, sni: sni}
		routesByKey[key] = append(routesByKey[key], r)
	}

	// Collect unique external ports for listener construction.
	portSet := make(map[uint16]struct{})
	for key := range routesByKey {
		portSet[key.port] = struct{}{}
	}

	for port := range portSet {
		filterChains := make([]*listener.FilterChain, 0)

		for key, keyRoutes := range routesByKey {
			if key.port != port {
				continue
			}

			clusterName := fmt.Sprintf("cluster_port_%d", port)
			if key.sni != "" {
				clusterName = fmt.Sprintf("cluster_port_%d_sni_%s", port, key.sni)
			}

			// Build one LbEndpoint per backend in this group.
			lbEndpoints := make([]*endpoint.LbEndpoint, 0, len(keyRoutes))
			for _, r := range keyRoutes {
				lbEndpoints = append(lbEndpoints, &endpoint.LbEndpoint{
					HostIdentifier: &endpoint.LbEndpoint_Endpoint{
						Endpoint: &endpoint.Endpoint{
							Address: &core.Address{
								Address: &core.Address_SocketAddress{
									SocketAddress: &core.SocketAddress{
										Address: r.HostIP,
										PortSpecifier: &core.SocketAddress_PortValue{
											PortValue: uint32(r.HostTLSPort),
										},
									},
								},
							},
						},
					},
				})
			}

			c := &cluster.Cluster{
				Name:                 clusterName,
				ConnectTimeout:       durationpb.New(5 * time.Second),
				ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_STATIC},
				LoadAssignment: &endpoint.ClusterLoadAssignment{
					ClusterName: clusterName,
					Endpoints: []*endpoint.LocalityLbEndpoints{
						{LbEndpoints: lbEndpoints},
					},
				},
			}
			clusters = append(clusters, c)

			tcpProxyCfg := &tcp_proxy_v3.TcpProxy{
				StatPrefix: clusterName,
				ClusterSpecifier: &tcp_proxy_v3.TcpProxy_Cluster{
					Cluster: clusterName,
				},
			}
			tcpProxyAny, err := anypb.New(tcpProxyCfg)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal tcp_proxy config for %s: %w", clusterName, err)
			}

			fcMatch := &listener.FilterChainMatch{}
			if key.sni != "" {
				fcMatch.ServerNames = []string{key.sni}
			}

			filterChains = append(filterChains, &listener.FilterChain{
				FilterChainMatch: fcMatch,
				Filters: []*listener.Filter{
					{
						Name: "envoy.filters.network.tcp_proxy",
						ConfigType: &listener.Filter_TypedConfig{
							TypedConfig: tcpProxyAny,
						},
					},
				},
			})
		}

		l := &listener.Listener{
			Name: fmt.Sprintf("listener_%d", port),
			Address: &core.Address{
				Address: &core.Address_SocketAddress{
					SocketAddress: &core.SocketAddress{
						Address: "0.0.0.0",
						PortSpecifier: &core.SocketAddress_PortValue{
							PortValue: uint32(port),
						},
					},
				},
			},
			FilterChains: filterChains,
		}
		listeners = append(listeners, l)
	}

	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].(*cluster.Cluster).Name < clusters[j].(*cluster.Cluster).Name
	})
	sort.Slice(listeners, func(i, j int) bool {
		return listeners[i].(*listener.Listener).Name < listeners[j].(*listener.Listener).Name
	})

	version := time.Now().String()
	snapshot, err := cache.NewSnapshot(version, map[resource.Type][]types.Resource{
		resource.ClusterType:  clusters,
		resource.ListenerType: listeners,
	})
	if err != nil {
		return nil, err
	}

	return snapshot, nil
}
