package registry_test

import (
	"io"
	"log/slog"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/gorouter/config"
	"code.cloudfoundry.org/gorouter/metrics"
	"code.cloudfoundry.org/gorouter/registry"
	"code.cloudfoundry.org/gorouter/route"
)

var _ = Describe("Instance lookup security", func() {
	type securitySettings struct {
		service  string
		scope    string
		policies []string
	}

	const uri = route.Uri("app.example/path")
	var r *registry.RouteRegistry

	endpoint := func(index string, port uint16, security securitySettings) *route.Endpoint {
		// HTTP NATS registrations have empty modification tags.
		return route.NewEndpoint(&route.EndpointOpts{
			AppId: "app-id", Host: "127.0.0.1", Port: port,
			PrivateInstanceId: "instance-" + index, PrivateInstanceIndex: index,
			Tags:            map[string]string{"process_id": "process-id"},
			RouteServiceUrl: security.service, RoutePolicyScope: security.scope,
			RoutePolicies: security.policies,
		})
	}
	expectSecurity := func(pool *route.EndpointPool, expected securitySettings) {
		Expect(pool).NotTo(BeNil())
		Expect(pool.RouteServiceUrl()).To(Equal(expected.service))
		Expect(pool.RoutePolicyScope()).To(Equal(expected.scope))
		Expect(pool.RoutePolicies()).To(Equal(expected.policies))
	}

	BeforeEach(func() {
		cfg, err := config.DefaultConfig()
		Expect(err).NotTo(HaveOccurred())
		r = registry.NewRouteRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)), cfg, metrics.MultiMetricReporter{})
	})

	for _, selector := range []string{"app instance", "process instance", "all process instances"} {
		Context(selector, func() {
			lookup := func() *route.EndpointPool {
				switch selector {
				case "app instance":
					return r.LookupWithAppInstance(uri, "app-id", "1")
				case "process instance":
					return r.LookupWithProcessInstance(uri, "process-id", "1")
				default:
					return r.LookupWithProcessInstance(uri, "process-id", "")
				}
			}

			DescribeTable("preserves the parent route's security during registration convergence",
				func(old, current securitySettings) {
					r.Register(uri, endpoint("0", 8080, old))
					stale := endpoint("1", 8081, old)
					r.Register(uri, stale)
					// Update the earlier entry so that a process-wide lookup encounters
					// the stale entry last, independently of registration recency.
					updated := endpoint("0", 8080, current)
					r.Register(uri, updated)
					// Repeating stale registration must not change the parent's settings.
					r.Register(uri, endpoint("1", 8081, old))

					parent := r.Lookup(uri)
					expectSecurity(parent, current)
					selected := lookup()
					expectSecurity(selected, current)

					var selectedEndpoints []*route.Endpoint
					selected.Each(func(e *route.Endpoint) { selectedEndpoints = append(selectedEndpoints, e) })
					if selector == "all process instances" {
						Expect(selectedEndpoints).To(ConsistOf(updated, stale))
					} else {
						Expect(selectedEndpoints).To(ConsistOf(stale))
					}
					Expect(selected.Host()).To(Equal(parent.Host()))
					Expect(selected.ContextPath()).To(Equal(parent.ContextPath()))
					Expect(parent.NumEndpoints()).To(Equal(2))
					Expect(stale.RouteServiceUrl).To(Equal(old.service))
					Expect(stale.RoutePolicyScope).To(Equal(old.scope))
					Expect(stale.RoutePolicies).To(Equal(old.policies))

					r.Register(uri, endpoint("1", 8081, current))
					expectSecurity(lookup(), current)
				},
				Entry("binding a route service", securitySettings{}, securitySettings{service: "https://gate.example"}),
				Entry("unbinding a route service", securitySettings{service: "https://gate.example"}, securitySettings{}),
				Entry("replacing a route service", securitySettings{service: "https://old.example"}, securitySettings{service: "https://new.example"}),
				Entry("restricting policies", securitySettings{scope: "any", policies: []string{"cf:any"}}, securitySettings{scope: "any", policies: []string{"cf:app:allowed-app"}}),
				Entry("removing all policies", securitySettings{scope: "any", policies: []string{"cf:any"}}, securitySettings{scope: "any"}),
				Entry("tightening scope", securitySettings{scope: "any", policies: []string{"cf:any"}}, securitySettings{scope: "org", policies: []string{"cf:any"}}),
				Entry("enabling enforcement", securitySettings{}, securitySettings{scope: "space", policies: []string{"cf:space:allowed-space"}}),
				Entry("disabling enforcement", securitySettings{scope: "org", policies: []string{"cf:org:allowed-org"}}, securitySettings{}),
			)

			It("does not share policy slice storage with the parent or registered endpoints", func() {
				current := securitySettings{scope: "any", policies: []string{"cf:app:allowed-app"}}
				registered := endpoint("1", 8081, current)
				r.Register(uri, registered)
				selected := lookup()
				selected.RoutePolicies()[0] = "cf:any"

				expectSecurity(r.Lookup(uri), securitySettings{scope: "any", policies: []string{"cf:app:allowed-app"}})
				Expect(registered.RoutePolicies).To(Equal([]string{"cf:app:allowed-app"}))
			})

			It("keeps a coherent security snapshot while registrations change", func() {
				states := []securitySettings{
					{service: "https://first.example", scope: "org", policies: []string{"cf:org:first"}},
					{service: "https://second.example", scope: "space", policies: []string{"cf:space:second"}},
				}
				r.Register(uri, endpoint("0", 8080, states[0]))
				r.Register(uri, endpoint("1", 8081, states[0]))

				var workers sync.WaitGroup
				workers.Add(1)
				go func() {
					defer GinkgoRecover()
					defer workers.Done()
					for i := 0; i < 200; i++ {
						r.Register(uri, endpoint("0", 8080, states[i%2]))
					}
				}()
				defer workers.Wait()
				for i := 0; i < 200; i++ {
					selected := lookup()
					actual := securitySettings{selected.RouteServiceUrl(), selected.RoutePolicyScope(), selected.RoutePolicies()}
					Expect(actual).To(BeElementOf(states))
				}
			})
		})
	}
})
