package models_test

import (
	"code.cloudfoundry.org/lager"
	"time"

	"code.cloudfoundry.org/routing-release/cf-tcp-router/models"
	"code.cloudfoundry.org/routing-release/cf-tcp-router/testutil"
	"code.cloudfoundry.org/lager/lagertest"
	routing_api_models "code.cloudfoundry.org/routing-release/routing-api/models"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("RoutingTable", func() {
	var (
		backendServerKey models.BackendServerKey
		routingTable     models.RoutingTable
		modificationTag  routing_api_models.ModificationTag
		logger           lager.Logger
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("routing-table-test")
		routingTable = models.NewRoutingTable(logger)
		modificationTag = routing_api_models.ModificationTag{Guid: "abc", Index: 1}
	})

	Describe("Set", func() {
		var (
			routingKey           models.RoutingKey
			routingTableEntry    models.RoutingTableEntry
			backendServerDetails models.BackendServerDetails
			now                  time.Time
		)

		BeforeEach(func() {
			routingKey = models.RoutingKey{Port: 12}
			backendServerKey = models.BackendServerKey{Address: "some-ip-1", Port: 1234}
			now = time.Now()
			backendServerDetails = models.BackendServerDetails{ModificationTag: modificationTag, TTL: 120, UpdatedTime: now}
			backends := map[models.BackendServerKey]models.BackendServerDetails{
				backendServerKey: backendServerDetails,
			}
			routingTableEntry = models.RoutingTableEntry{Backends: backends}
		})

		Context("when a new entry is added", func() {
			It("adds the entry", func() {
				ok := routingTable.Set(routingKey, routingTableEntry)
				Expect(ok).To(BeTrue())
				Expect(routingTable.Get(routingKey)).To(Equal(routingTableEntry))
				Expect(routingTable.Size()).To(Equal(1))
			})
		})

		Context("when setting pre-existing routing key", func() {
			var (
				existingRoutingTableEntry models.RoutingTableEntry
				newBackendServerKey       models.BackendServerKey
			)

			BeforeEach(func() {
				newBackendServerKey = models.BackendServerKey{
					Address: "some-ip-2",
					Port:    1234,
				}
				existingRoutingTableEntry = models.RoutingTableEntry{
					Backends: map[models.BackendServerKey]models.BackendServerDetails{
						backendServerKey:    backendServerDetails,
						newBackendServerKey: models.BackendServerDetails{ModificationTag: modificationTag, TTL: 120, UpdatedTime: now},
					},
				}
				ok := routingTable.Set(routingKey, existingRoutingTableEntry)
				Expect(ok).To(BeTrue())
				Expect(routingTable.Size()).To(Equal(1))
			})

			Context("with different value", func() {
				verifyChangedValue := func(routingTableEntry models.RoutingTableEntry) {
					ok := routingTable.Set(routingKey, routingTableEntry)
					Expect(ok).To(BeTrue())
					Expect(routingTable.Get(routingKey)).Should(Equal(routingTableEntry))
				}

				Context("when number of backends are different", func() {
					It("overwrites the value", func() {
						routingTableEntry := models.RoutingTableEntry{
							Backends: map[models.BackendServerKey]models.BackendServerDetails{
								models.BackendServerKey{
									Address: "some-ip-1",
									Port:    1234,
								}: models.BackendServerDetails{ModificationTag: modificationTag, UpdatedTime: now},
							},
						}
						verifyChangedValue(routingTableEntry)
					})
				})

				Context("when at least one backend server info is different", func() {
					It("overwrites the value", func() {
						routingTableEntry := models.RoutingTableEntry{
							Backends: map[models.BackendServerKey]models.BackendServerDetails{
								models.BackendServerKey{Address: "some-ip-1", Port: 1234}: models.BackendServerDetails{ModificationTag: modificationTag, UpdatedTime: now},
								models.BackendServerKey{Address: "some-ip-2", Port: 2345}: models.BackendServerDetails{ModificationTag: modificationTag, UpdatedTime: now},
							},
						}
						verifyChangedValue(routingTableEntry)
					})
				})

				Context("when all backend servers info are different", func() {
					It("overwrites the value", func() {
						routingTableEntry := models.RoutingTableEntry{
							Backends: map[models.BackendServerKey]models.BackendServerDetails{
								models.BackendServerKey{Address: "some-ip-1", Port: 3456}: models.BackendServerDetails{ModificationTag: modificationTag, UpdatedTime: now},
								models.BackendServerKey{Address: "some-ip-2", Port: 2345}: models.BackendServerDetails{ModificationTag: modificationTag, UpdatedTime: now},
							},
						}
						verifyChangedValue(routingTableEntry)
					})
				})

				Context("when modificationTag is different", func() {
					It("overwrites the value", func() {
						routingTableEntry := models.RoutingTableEntry{
							Backends: map[models.BackendServerKey]models.BackendServerDetails{
								models.BackendServerKey{Address: "some-ip-1", Port: 1234}: models.BackendServerDetails{ModificationTag: routing_api_models.ModificationTag{Guid: "different-guid"}, UpdatedTime: now},
								models.BackendServerKey{Address: "some-ip-2", Port: 1234}: models.BackendServerDetails{ModificationTag: routing_api_models.ModificationTag{Guid: "different-guid"}, UpdatedTime: now},
							},
						}
						verifyChangedValue(routingTableEntry)
					})
				})

				Context("when TTL is different", func() {
					It("overwrites the value", func() {
						routingTableEntry := models.RoutingTableEntry{
							Backends: map[models.BackendServerKey]models.BackendServerDetails{
								models.BackendServerKey{Address: "some-ip-1", Port: 1234}: models.BackendServerDetails{ModificationTag: modificationTag, TTL: 110, UpdatedTime: now},
								models.BackendServerKey{Address: "some-ip-2", Port: 1234}: models.BackendServerDetails{ModificationTag: modificationTag, TTL: 110, UpdatedTime: now},
							},
						}
						verifyChangedValue(routingTableEntry)
					})
				})
			})

			Context("with same value", func() {
				It("returns false", func() {
					routingTableEntry := models.RoutingTableEntry{
						Backends: map[models.BackendServerKey]models.BackendServerDetails{
							backendServerKey:    models.BackendServerDetails{ModificationTag: modificationTag, TTL: 120, UpdatedTime: now},
							newBackendServerKey: models.BackendServerDetails{ModificationTag: modificationTag, TTL: 120, UpdatedTime: now},
						},
					}
					ok := routingTable.Set(routingKey, routingTableEntry)
					Expect(ok).To(BeFalse())
					testutil.RoutingTableEntryMatches(routingTable.Get(routingKey), existingRoutingTableEntry)
				})
			})
		})
	})

	Describe("UpsertBackendServerKey", func() {
		var (
			existingRoutingKey        models.RoutingKey
			existingBackendServerInfo models.BackendServerInfo
			existingRoutingTableEntry models.RoutingTableEntry
		)

		BeforeEach(func() {
			existingRoutingKey = models.RoutingKey{Port: 80}
			existingBackendServerInfo = models.BackendServerInfo{
				Address:         "host-1.internal",
				Port:            8080,
				ModificationTag: routing_api_models.ModificationTag{Guid: "11111111-1111-1111-1111-111111111111", Index: 2},
				TTL:             60,
			}
			existingRoutingTableEntry = models.NewRoutingTableEntry([]models.BackendServerInfo{existingBackendServerInfo})
		})

		Context("when the routing key does not exist in the table", func() {
			Context("when the table has no existing entries", func() {
				It("adds the new entry and indicates the config needs to be reloaded", func() {
					reloadConfig := routingTable.UpsertBackendServerKey(existingRoutingKey, existingBackendServerInfo)

					Expect(reloadConfig).To(BeTrue())
					Expect(logger).To(gbytes.Say("routing-key-not-found"))

					Expect(routingTable.Size()).To(Equal(1))
					testutil.RoutingTableEntryMatches(routingTable.Get(existingRoutingKey), existingRoutingTableEntry)
				})
			})

			Context("when the table has existing entries", func() {
				BeforeEach(func() {
					routingTable.Entries = map[models.RoutingKey]models.RoutingTableEntry{
						existingRoutingKey: existingRoutingTableEntry,
					}
				})

				Context("and the new routing key port does not exist", func() {
					It("adds the new entry and indicates the config needs to be reloaded", func() {
						newRoutingKey := models.RoutingKey{Port: 90}
						reloadConfig := routingTable.UpsertBackendServerKey(newRoutingKey, existingBackendServerInfo)

						Expect(reloadConfig).To(BeTrue())
						Expect(logger).To(gbytes.Say("routing-key-not-found"))

						Expect(routingTable.Size()).To(Equal(2))
						testutil.RoutingTableEntryMatches(routingTable.Entries[existingRoutingKey], existingRoutingTableEntry)
						testutil.RoutingTableEntryMatches(routingTable.Entries[newRoutingKey], existingRoutingTableEntry)
					})
				})

				Context("and the new routing key SNI hostname does not exist", func() {
					It("adds the new entry and indicates the config needs to be reloaded", func() {
						newRoutingKey := models.RoutingKey{Port: 80, SniHostname: "host-2.example.com"}
						reloadConfig := routingTable.UpsertBackendServerKey(newRoutingKey, existingBackendServerInfo)

						Expect(reloadConfig).To(BeTrue())
						Expect(logger).To(gbytes.Say("routing-key-not-found"))

						Expect(routingTable.Size()).To(Equal(2))
						testutil.RoutingTableEntryMatches(routingTable.Entries[existingRoutingKey], existingRoutingTableEntry)
						testutil.RoutingTableEntryMatches(routingTable.Entries[newRoutingKey], existingRoutingTableEntry)
					})
				})
			})
		})

		Context("when the routing key already exists in the table", func() {
			var oldBackendServerDetails models.BackendServerDetails

			BeforeEach(func() {
				routingTable.Entries = map[models.RoutingKey]models.RoutingTableEntry{
					existingRoutingKey: existingRoutingTableEntry,
				}
				oldBackendServerDetails = existingRoutingTableEntry.Backends[models.BackendServerKey{
					Address: existingBackendServerInfo.Address,
					Port:    existingBackendServerInfo.Port,
				}]
			})

			validateBackendServersUpdated := func(newBackendServerInfo models.BackendServerInfo, shouldBeNewer bool) {
				actualBackendServerDetails := routingTable.
					Entries[existingRoutingKey].
					Backends[models.BackendServerKey{
					Address: newBackendServerInfo.Address,
					Port:    newBackendServerInfo.Port,
				}]

				if shouldBeNewer {
					Expect(actualBackendServerDetails.UpdatedTime.After(oldBackendServerDetails.UpdatedTime)).To(BeTrue())
				} else {
					Expect(actualBackendServerDetails.UpdatedTime).To(Equal(oldBackendServerDetails.UpdatedTime))
				}
			}

			Context("when that key's backends do not match", func() {
				Context("because the backend hostname does not match", func() {
					It("adds the new entry and indicates the config needs to be reloaded", func() {
						newBackendServerInfo := models.BackendServerInfo{
							Address:         "host-2.internal",
							Port:            existingBackendServerInfo.Port,
							ModificationTag: routing_api_models.ModificationTag{Guid: "22222222-2222-2222-2222-222222222222", Index: 1},
						}

						reloadConfig := routingTable.UpsertBackendServerKey(existingRoutingKey, newBackendServerInfo)

						Expect(reloadConfig).To(BeTrue())
						Expect(logger).To(gbytes.Say("applying-change-to-table"))
						Expect(routingTable.Size()).To(Equal(1))

						validateBackendServersUpdated(newBackendServerInfo, true)
						testutil.RoutingTableEntryMatches(
							routingTable.Entries[existingRoutingKey],
							models.RoutingTableEntry{
								Backends: map[models.BackendServerKey]models.BackendServerDetails{
									{
										Address: existingBackendServerInfo.Address,
										Port:    existingBackendServerInfo.Port,
									}: {
										ModificationTag: existingBackendServerInfo.ModificationTag,
										TTL:             existingBackendServerInfo.TTL,
									},
									{
										Address: newBackendServerInfo.Address,
										Port:    newBackendServerInfo.Port,
									}: {
										ModificationTag: newBackendServerInfo.ModificationTag,
									},
								},
							},
						)
					})
				})

				Context("because the backend port does not match", func() {
					It("adds the new entry and indicates the config needs to be reloaded", func() {
						newBackendServerInfo := models.BackendServerInfo{
							Address: existingBackendServerInfo.Address,
							Port:    9090,
						}

						reloadConfig := routingTable.UpsertBackendServerKey(existingRoutingKey, newBackendServerInfo)

						Expect(reloadConfig).To(BeTrue())
						Expect(logger).To(gbytes.Say("applying-change-to-table"))
						Expect(routingTable.Size()).To(Equal(1))

						validateBackendServersUpdated(newBackendServerInfo, true)
						testutil.RoutingTableEntryMatches(
							routingTable.Entries[existingRoutingKey],
							models.RoutingTableEntry{
								Backends: map[models.BackendServerKey]models.BackendServerDetails{
									{
										Address: existingBackendServerInfo.Address,
										Port:    existingBackendServerInfo.Port,
									}: {
										ModificationTag: existingBackendServerInfo.ModificationTag,
										TTL:             existingBackendServerInfo.TTL,
									},
									{
										Address: newBackendServerInfo.Address,
										Port:    newBackendServerInfo.Port,
									}: {
										ModificationTag: newBackendServerInfo.ModificationTag,
									},
								},
							},
						)
					})
				})
			})

			Context("when that key's backends do match", func() {
				Context("and the current routing table has a more recent record of the backend", func() {
					It("does not update the existing backend and indicates config does not need to be reloaded", func() {
						newBackendServerInfo := models.BackendServerInfo{
							Address: existingBackendServerInfo.Address,
							Port:    existingBackendServerInfo.Port,
							TTL:     99,
							ModificationTag: routing_api_models.ModificationTag{
								Guid:  existingBackendServerInfo.ModificationTag.Guid,
								Index: existingBackendServerInfo.ModificationTag.Index - 1, // Earlier than existing backend server
							},
						}
						reloadConfig := routingTable.UpsertBackendServerKey(existingRoutingKey, newBackendServerInfo)

						Expect(reloadConfig).To(BeFalse())
						Expect(logger).To(gbytes.Say("skipping-stale-event"))
						Expect(routingTable.Size()).To(Equal(1))

						validateBackendServersUpdated(newBackendServerInfo, false)
						testutil.RoutingTableEntryMatches(
							routingTable.Entries[existingRoutingKey],
							models.RoutingTableEntry{
								Backends: map[models.BackendServerKey]models.BackendServerDetails{
									{
										Address: existingBackendServerInfo.Address,
										Port:    existingBackendServerInfo.Port,
									}: {
										ModificationTag: existingBackendServerInfo.ModificationTag,
										TTL:             existingBackendServerInfo.TTL,
									},
								},
							},
						)
					})
				})

				Context("and the current routing table has the same record of the backend", func() {
					It("does not add the new entry and indicates the config does not need to be reloaded", func() {
						newBackendServerInfo := models.BackendServerInfo{
							Address: existingBackendServerInfo.Address,
							Port:    existingBackendServerInfo.Port,
							TTL:     99,
							ModificationTag: routing_api_models.ModificationTag{
								Guid:  existingBackendServerInfo.ModificationTag.Guid,
								Index: existingBackendServerInfo.ModificationTag.Index, // Identical to existing backend
							},
						}
						reloadConfig := routingTable.UpsertBackendServerKey(existingRoutingKey, newBackendServerInfo)

						Expect(reloadConfig).To(BeFalse())
						Expect(logger).To(gbytes.Say("skipping-stale-event"))
						Expect(routingTable.Size()).To(Equal(1))

						validateBackendServersUpdated(newBackendServerInfo, false)
						testutil.RoutingTableEntryMatches(
							routingTable.Entries[existingRoutingKey],
							models.RoutingTableEntry{
								Backends: map[models.BackendServerKey]models.BackendServerDetails{
									{
										Address: existingBackendServerInfo.Address,
										Port:    existingBackendServerInfo.Port,
									}: {
										ModificationTag: existingBackendServerInfo.ModificationTag,
										TTL:             existingBackendServerInfo.TTL,
									},
								},
							},
						)
					})
				})

				Context("and the current routing table has an older record of the backend", func() {
					It("adds the new entry and indicates the config does not need to be reloaded", func() {
						newBackendServerInfo := models.BackendServerInfo{
							Address: existingBackendServerInfo.Address,
							Port:    existingBackendServerInfo.Port,
							TTL:     99,
							ModificationTag: routing_api_models.ModificationTag{
								Guid:  existingBackendServerInfo.ModificationTag.Guid,
								Index: existingBackendServerInfo.ModificationTag.Index + 1, // Newer than existing backend
							},
						}
						reloadConfig := routingTable.UpsertBackendServerKey(existingRoutingKey, newBackendServerInfo)

						Expect(reloadConfig).To(BeFalse())
						Expect(logger).To(gbytes.Say("applying-change-to-table"))
						Expect(routingTable.Size()).To(Equal(1))

						validateBackendServersUpdated(newBackendServerInfo, true)
						testutil.RoutingTableEntryMatches(
							routingTable.Entries[existingRoutingKey],
							models.RoutingTableEntry{
								Backends: map[models.BackendServerKey]models.BackendServerDetails{
									{
										Address: newBackendServerInfo.Address,
										Port:    newBackendServerInfo.Port,
									}: {
										ModificationTag: newBackendServerInfo.ModificationTag,
										TTL:             newBackendServerInfo.TTL,
									},
								},
							},
						)
					})
				})
			})
		})
	})

	Describe("DeleteBackendServerKey", func() {
		var (
			routingKey                models.RoutingKey
			existingRoutingTableEntry models.RoutingTableEntry
			backendServerInfo1        models.BackendServerInfo
			backendServerInfo2        models.BackendServerInfo
		)
		BeforeEach(func() {
			routingKey = models.RoutingKey{Port: 12, SniHostname: "host-1.example.com"}
			backendServerInfo1 = models.BackendServerInfo{Address: "some-ip", Port: 1234, ModificationTag: modificationTag}
		})

		Context("when no routing keys exist", func() {
			It("it does not causes any changes or errors", func() {
				updated := routingTable.DeleteBackendServerKey(routingKey, backendServerInfo1)
				Expect(updated).To(BeFalse())
			})
		})

		Context("when other routing keys exist", func() {
			var (
				survivingKey               models.RoutingKey
				survivingRoutingTableEntry models.RoutingTableEntry
			)
			Context("when a non-SNI routing key shares the same port as the deleted one", func() {
				BeforeEach(func() {
					survivingKey = models.RoutingKey{Port: 12, SniHostname: ""}
					survivingRoutingTableEntry = models.NewRoutingTableEntry([]models.BackendServerInfo{backendServerInfo1})
					routingTable.Entries[survivingKey] = survivingRoutingTableEntry
				})

				It("retains the non-SNI route", func() {
					updated := routingTable.DeleteBackendServerKey(routingKey, backendServerInfo1)
					Expect(updated).To(BeFalse())

					Expect(len(routingTable.Entries)).To(Equal(1))
					testutil.RoutingTableEntryMatches(routingTable.Entries[survivingKey], survivingRoutingTableEntry)
				})
			})

			Context("when another SNI routing key shares the same port as the deleted one", func() {
				BeforeEach(func() {
					survivingKey = models.RoutingKey{Port: 12, SniHostname: "other-host.example.com"}
					survivingRoutingTableEntry = models.NewRoutingTableEntry([]models.BackendServerInfo{backendServerInfo1})
					routingTable.Entries[survivingKey] = survivingRoutingTableEntry
				})

				It("retains the other SNI route", func() {
					updated := routingTable.DeleteBackendServerKey(routingKey, backendServerInfo1)
					Expect(updated).To(BeFalse())

					Expect(len(routingTable.Entries)).To(Equal(1))
					testutil.RoutingTableEntryMatches(routingTable.Entries[survivingKey], survivingRoutingTableEntry)
				})
			})

			Context("when another SNI routing key shares the same hostname as the deleted one", func() {
				BeforeEach(func() {
					survivingKey = models.RoutingKey{Port: 34, SniHostname: "host.example.com"}
					survivingRoutingTableEntry = models.NewRoutingTableEntry([]models.BackendServerInfo{backendServerInfo1})
					routingTable.Entries[survivingKey] = survivingRoutingTableEntry
				})

				It("retains the other SNI route", func() {
					updated := routingTable.DeleteBackendServerKey(routingKey, backendServerInfo1)
					Expect(updated).To(BeFalse())

					Expect(len(routingTable.Entries)).To(Equal(1))
					testutil.RoutingTableEntryMatches(routingTable.Entries[survivingKey], survivingRoutingTableEntry)
				})
			})
		})

		Context("when the routing key does exist", func() {
			BeforeEach(func() {
				backendServerInfo1 = models.BackendServerInfo{Address: "some-other-ip", Port: 1235, ModificationTag: modificationTag}
				existingRoutingTableEntry = models.NewRoutingTableEntry([]models.BackendServerInfo{backendServerInfo1, backendServerInfo2})
				updated := routingTable.Set(routingKey, existingRoutingTableEntry)
				Expect(updated).To(BeTrue())
			})

			Context("and the backend does not exist ", func() {
				It("does not causes any changes or errors", func() {
					backendServerInfo1 = models.BackendServerInfo{Address: "some-missing-ip", Port: 1236, ModificationTag: modificationTag}
					ok := routingTable.DeleteBackendServerKey(routingKey, backendServerInfo1)
					Expect(ok).To(BeFalse())
					Expect(routingTable.Get(routingKey)).Should(Equal(existingRoutingTableEntry))
				})
			})

			Context("and the backend does exist", func() {
				It("deletes the backend", func() {
					updated := routingTable.DeleteBackendServerKey(routingKey, backendServerInfo1)
					Expect(updated).To(BeTrue())
					Expect(logger).To(gbytes.Say("removing-from-table"))
					expectedRoutingTableEntry := models.NewRoutingTableEntry([]models.BackendServerInfo{backendServerInfo2})
					testutil.RoutingTableEntryMatches(routingTable.Get(routingKey), expectedRoutingTableEntry)
				})

				Context("when a modification tag has the same guid but current index is greater", func() {
					BeforeEach(func() {
						backendServerInfo1.ModificationTag.Index--
					})

					It("does not deletes the backend", func() {
						updated := routingTable.DeleteBackendServerKey(routingKey, backendServerInfo1)
						Expect(updated).To(BeFalse())
						Expect(logger).To(gbytes.Say("skipping-stale-event"))
						Expect(routingTable.Get(routingKey)).Should(Equal(existingRoutingTableEntry))
					})
				})

				Context("when a modification tag has different guid", func() {
					var expectedRoutingTableEntry models.RoutingTableEntry

					BeforeEach(func() {
						expectedRoutingTableEntry = models.NewRoutingTableEntry([]models.BackendServerInfo{backendServerInfo2})
						backendServerInfo1.ModificationTag = routing_api_models.ModificationTag{Guid: "def"}
					})

					It("deletes the backend", func() {
						updated := routingTable.DeleteBackendServerKey(routingKey, backendServerInfo1)
						Expect(updated).To(BeTrue())
						Expect(logger).To(gbytes.Say("removing-from-table"))
						testutil.RoutingTableEntryMatches(routingTable.Get(routingKey), expectedRoutingTableEntry)
					})
				})

				Context("when there are no more backends left", func() {
					BeforeEach(func() {
						updated := routingTable.DeleteBackendServerKey(routingKey, backendServerInfo1)
						Expect(updated).To(BeTrue())
					})

					It("deletes the entry", func() {
						updated := routingTable.DeleteBackendServerKey(routingKey, backendServerInfo2)
						Expect(updated).To(BeTrue())
						Expect(routingTable.Size()).Should(Equal(0))
					})
				})
			})
		})
	})

	Describe("PruneEntries", func() {
		var (
			defaultTTL  int
			routingKey1 models.RoutingKey
			routingKey2 models.RoutingKey
		)
		BeforeEach(func() {
			routingKey1 = models.RoutingKey{Port: 12}
			backendServerKey := models.BackendServerKey{Address: "some-ip-1", Port: 1234}
			backendServerDetails := models.BackendServerDetails{ModificationTag: modificationTag, UpdatedTime: time.Now().Add(-10 * time.Second)}
			backendServerKey2 := models.BackendServerKey{Address: "some-ip-2", Port: 1235}
			backendServerDetails2 := models.BackendServerDetails{ModificationTag: modificationTag, UpdatedTime: time.Now().Add(-3 * time.Second)}
			backends := map[models.BackendServerKey]models.BackendServerDetails{
				backendServerKey:  backendServerDetails,
				backendServerKey2: backendServerDetails2,
			}
			routingTableEntry := models.RoutingTableEntry{Backends: backends}
			updated := routingTable.Set(routingKey1, routingTableEntry)
			Expect(updated).To(BeTrue())

			routingKey2 = models.RoutingKey{Port: 13}
			backendServerKey = models.BackendServerKey{Address: "some-ip-3", Port: 1234}
			backendServerDetails = models.BackendServerDetails{ModificationTag: modificationTag, UpdatedTime: time.Now().Add(-10 * time.Second)}
			backendServerKey2 = models.BackendServerKey{Address: "some-ip-4", Port: 1235}
			backendServerDetails2 = models.BackendServerDetails{ModificationTag: modificationTag, UpdatedTime: time.Now()}
			backends = map[models.BackendServerKey]models.BackendServerDetails{
				backendServerKey:  backendServerDetails,
				backendServerKey2: backendServerDetails2,
			}
			routingTableEntry = models.RoutingTableEntry{Backends: backends}
			updated = routingTable.Set(routingKey2, routingTableEntry)
			Expect(updated).To(BeTrue())
		})

		JustBeforeEach(func() {
			routingTable.PruneEntries(defaultTTL)
		})

		Context("when it has expired entries", func() {
			BeforeEach(func() {
				defaultTTL = 5
			})

			It("prunes the expired entries", func() {
				Expect(routingTable.Entries).To(HaveLen(2))
				Expect(routingTable.Get(routingKey1).Backends).To(HaveLen(1))
				Expect(routingTable.Get(routingKey2).Backends).To(HaveLen(1))
			})

			Context("when all the backends expire for given routing key", func() {
				BeforeEach(func() {
					defaultTTL = 2
				})

				It("prunes the expired entries and deletes the routing key", func() {
					Expect(routingTable.Entries).To(HaveLen(1))
					Expect(routingTable.Get(routingKey2).Backends).To(HaveLen(1))
				})
			})
		})

		Context("when it has no expired entries", func() {
			BeforeEach(func() {
				defaultTTL = 20
			})

			It("does not prune entries", func() {
				Expect(routingTable.Entries).To(HaveLen(2))
				Expect(routingTable.Get(routingKey1).Backends).To(HaveLen(2))
				Expect(routingTable.Get(routingKey2).Backends).To(HaveLen(2))
			})
		})
	})

	Describe("BackendServerDetails", func() {
		var (
			now        = time.Now()
			defaultTTL = 20
		)

		Context("when backend details have TTL", func() {
			It("returns true if updated time is past expiration time", func() {
				backendDetails := models.BackendServerDetails{TTL: 1, UpdatedTime: now.Add(-2 * time.Second)}
				Expect(backendDetails.Expired(defaultTTL)).To(BeTrue())
			})

			It("returns false if updated time is not past expiration time", func() {
				backendDetails := models.BackendServerDetails{TTL: 1, UpdatedTime: now}
				Expect(backendDetails.Expired(defaultTTL)).To(BeFalse())
			})
		})

		Context("when backend details do not have TTL", func() {
			It("returns true if updated time is past expiration time", func() {
				backendDetails := models.BackendServerDetails{TTL: 0, UpdatedTime: now.Add(-25 * time.Second)}
				Expect(backendDetails.Expired(defaultTTL)).To(BeTrue())
			})

			It("returns false if updated time is not past expiration time", func() {
				backendDetails := models.BackendServerDetails{TTL: 0, UpdatedTime: now}
				Expect(backendDetails.Expired(defaultTTL)).To(BeFalse())
			})
		})
	})

	Describe("RoutingTableEntry", func() {
		var (
			routingTableEntry models.RoutingTableEntry
			defaultTTL        int
		)

		BeforeEach(func() {
			backendServerKey := models.BackendServerKey{Address: "some-ip-1", Port: 1234}
			backendServerDetails := models.BackendServerDetails{ModificationTag: modificationTag, UpdatedTime: time.Now().Add(-10 * time.Second)}
			backendServerKey2 := models.BackendServerKey{Address: "some-ip-2", Port: 1235}
			backendServerDetails2 := models.BackendServerDetails{ModificationTag: modificationTag, UpdatedTime: time.Now()}
			backends := map[models.BackendServerKey]models.BackendServerDetails{
				backendServerKey:  backendServerDetails,
				backendServerKey2: backendServerDetails2,
			}
			routingTableEntry = models.RoutingTableEntry{Backends: backends}
		})

		JustBeforeEach(func() {
			routingTableEntry.PruneBackends(defaultTTL)
		})

		Context("when it has expired backends", func() {
			BeforeEach(func() {
				defaultTTL = 5
			})

			It("prunes expired backends", func() {
				Expect(routingTableEntry.Backends).To(HaveLen(1))
			})
		})

		Context("when it does not have any expired backends", func() {
			BeforeEach(func() {
				defaultTTL = 15
			})

			It("prunes expired backends", func() {
				Expect(routingTableEntry.Backends).To(HaveLen(2))
			})
		})
	})
})
