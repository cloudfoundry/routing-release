package models_test

import (
	. "code.cloudfoundry.org/routing-release/cf-tcp-router/models"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("HAProxyConfig", func() {
	Describe("NewHAProxyConfig", func() {
		var (
			logger       lager.Logger
			routingTable RoutingTable
		)

		BeforeEach(func() {
			logger = lagertest.NewTestLogger("haproxy-config-test")
			routingTable = NewRoutingTable(logger)
		})

		Context("when a frontend is invalid", func() {
			validRoutingTableEntry := RoutingTableEntry{
				Backends: map[BackendServerKey]BackendServerDetails{
					BackendServerKey{Address: "valid-host.internal", Port: 1111}: {},
				},
			}

			Context("because it contains an invalid port", func() {
				It("retains only valid frontends", func() {
					routingTable.Entries[RoutingKey{Port: 0}] = validRoutingTableEntry
					routingTable.Entries[RoutingKey{Port: 80}] = validRoutingTableEntry

					Expect(NewHAProxyConfig(routingTable, logger)).To(Equal(HAProxyConfig{
						80: {
							"": {
								{Address: "valid-host.internal", Port: 1111},
							},
						},
					}))
				})
			})

			Context("because it contains an invalid SNI hostname", func() {
				It("retains only valid frontends", func() {
					routingTable.Entries[RoutingKey{Port: 80, SniHostname: "valid-host.example.com"}] = validRoutingTableEntry
					routingTable.Entries[RoutingKey{Port: 90, SniHostname: "!invalid-host.example.com"}] = validRoutingTableEntry
					routingTable.Entries[RoutingKey{Port: 100, SniHostname: "ünvalid-host.example.com"}] = validRoutingTableEntry

					Expect(NewHAProxyConfig(routingTable, logger)).To(Equal(HAProxyConfig{
						80: {
							"valid-host.example.com": {
								{Address: "valid-host.internal", Port: 1111},
							},
						},
					}))
				})
			})

			Context("because it contains no backends", func() {
				It("retains only valid frontends", func() {
					routingTable.Entries[RoutingKey{Port: 80}] = validRoutingTableEntry
					routingTable.Entries[RoutingKey{Port: 90}] = RoutingTableEntry{
						Backends: map[BackendServerKey]BackendServerDetails{},
					}

					Expect(NewHAProxyConfig(routingTable, logger)).To(Equal(HAProxyConfig{
						80: {
							"": {
								{Address: "valid-host.internal", Port: 1111},
							},
						},
					}))
				})
			})

			Context("because a backend is invalid", func() {
				Context("because it contains an invalid address", func() {
					It("retains only valid backends", func() {
						routingTable.Entries[RoutingKey{Port: 80}] = RoutingTableEntry{
							Backends: map[BackendServerKey]BackendServerDetails{
								BackendServerKey{Address: "valid-host.internal", Port: 1111}:    {},
								BackendServerKey{Address: "!invalid-host.internal", Port: 2222}: {},
							},
						}

						Expect(NewHAProxyConfig(routingTable, logger)).To(Equal(HAProxyConfig{
							80: {
								"": {
									{Address: "valid-host.internal", Port: 1111},
								},
							},
						}))
					})
				})

				Context("because it contains an invalid port", func() {
					It("retains only valid backends", func() {
						routingTable.Entries[RoutingKey{Port: 80}] = RoutingTableEntry{
							Backends: map[BackendServerKey]BackendServerDetails{
								BackendServerKey{Address: "valid-host-1.example.com", Port: 1111}: {},
								BackendServerKey{Address: "valid-host-2.example.com", Port: 0}:    {},
							},
						}

						Expect(NewHAProxyConfig(routingTable, logger)).To(Equal(HAProxyConfig{
							80: {
								"": {
									{Address: "valid-host-1.example.com", Port: 1111},
								},
							},
						}))
					})
				})
			})
		})

		Context("when multiple valid frontends exist", func() {
			It("includes all frontends", func() {
				routingTable.Entries[RoutingKey{Port: 80}] = RoutingTableEntry{
					Backends: map[BackendServerKey]BackendServerDetails{
						BackendServerKey{Address: "valid-host-1.internal", Port: 2222}: {},
						BackendServerKey{Address: "valid-host-1.internal", Port: 1111}: {},
					},
				}
				routingTable.Entries[RoutingKey{Port: 90, SniHostname: "valid-host.example.com"}] = RoutingTableEntry{
					Backends: map[BackendServerKey]BackendServerDetails{
						BackendServerKey{Address: "valid-host-4.internal", Port: 8888}: {},
						BackendServerKey{Address: "valid-host-4.internal", Port: 7777}: {},
						BackendServerKey{Address: "valid-host-3.internal", Port: 6666}: {},
						BackendServerKey{Address: "valid-host-3.internal", Port: 5555}: {},
					},
				}
				routingTable.Entries[RoutingKey{Port: 90}] = RoutingTableEntry{
					Backends: map[BackendServerKey]BackendServerDetails{
						BackendServerKey{Address: "valid-host-2.internal", Port: 4444}: {},
						BackendServerKey{Address: "valid-host-2.internal", Port: 3333}: {},
					},
				}

				Expect(NewHAProxyConfig(routingTable, logger)).To(Equal(HAProxyConfig{
					80: {
						"": {
							{Address: "valid-host-1.internal", Port: 1111},
							{Address: "valid-host-1.internal", Port: 2222},
						},
					},
					90: {
						"": {
							{Address: "valid-host-2.internal", Port: 3333},
							{Address: "valid-host-2.internal", Port: 4444},
						},
						"valid-host.example.com": {
							{Address: "valid-host-3.internal", Port: 5555},
							{Address: "valid-host-3.internal", Port: 6666},
							{Address: "valid-host-4.internal", Port: 7777},
							{Address: "valid-host-4.internal", Port: 8888},
						},
					},
				}))
			})
		})
	})
})
