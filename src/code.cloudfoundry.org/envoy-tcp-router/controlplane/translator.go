package controlplane

import (
	"fmt"
	"sort"
	"time"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	tcp_proxy "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"code.cloudfoundry.org/routing-api/models"
)

func Translate(routes []models.TcpRouteMapping, filterLibraryPath string) (*cache.Snapshot, error) {
	clusters := make([]types.Resource, 0)
	listeners := make([]types.Resource, 0)

	// Group routes by ExternalPort for Listeners
	routesByPort := make(map[uint16][]models.TcpRouteMapping)
	for _, r := range routes {
		routesByPort[r.ExternalPort] = append(routesByPort[r.ExternalPort], r)
	}

	// Create Clusters and Listeners
	for port, portRoutes := range routesByPort {
		filterChains := make([]*listener.FilterChain, 0)

		for _, r := range portRoutes {
			clusterName := fmt.Sprintf("cluster_%s_%d", r.HostIP, r.HostPort)

			// Create Cluster
			c := &cluster.Cluster{
				Name:                 clusterName,
				ConnectTimeout:       durationpb.New(5 * time.Second),
				ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_STATIC},
				LoadAssignment: &endpoint.ClusterLoadAssignment{
					ClusterName: clusterName,
					Endpoints: []*endpoint.LocalityLbEndpoints{
						{
							LbEndpoints: []*endpoint.LbEndpoint{
								{
									HostIdentifier: &endpoint.LbEndpoint_Endpoint{
										Endpoint: &endpoint.Endpoint{
											Address: &core.Address{
												Address: &core.Address_SocketAddress{
													SocketAddress: &core.SocketAddress{
														Address: r.HostIP,
														PortSpecifier: &core.SocketAddress_PortValue{
															PortValue: uint32(r.HostPort),
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			clusters = append(clusters, c)

			// Create Filter Chain for this route
			tcpProxyConfig, _ := anypb.New(&tcp_proxy.TcpProxy{
				StatPrefix: "tcp_proxy",
				ClusterSpecifier: &tcp_proxy.TcpProxy_Cluster{
					Cluster: clusterName,
				},
			})

			// TODO: Add Go Filter here once the proto is available in go-control-plane
			// The Go filter should be placed before the tcp_proxy filter.

			filters := []*listener.Filter{
				{
					Name: "envoy.filters.network.tcp_proxy",
					ConfigType: &listener.Filter_TypedConfig{
						TypedConfig: tcpProxyConfig,
					},
				},
			}

			fcMatch := &listener.FilterChainMatch{}
			if r.SniHostname != nil && *r.SniHostname != "" {
				fcMatch.ServerNames = []string{*r.SniHostname}
			}

			filterChains = append(filterChains, &listener.FilterChain{
				FilterChainMatch: fcMatch,
				Filters:          filters,
			})
		}

		// Create Listener
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

	// Sort resources for consistency
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
