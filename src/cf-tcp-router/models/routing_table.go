package models

import (
	"fmt"
	"reflect"
	"time"

	"code.cloudfoundry.org/lager"
	routing_api_models "code.cloudfoundry.org/routing-release/routing-api/models"
)

type SniHostname string

type RoutingKey struct {
	Port        uint16
	SniHostname SniHostname
}

type BackendServerInfo struct {
	Address         string
	Port            uint16
	ModificationTag routing_api_models.ModificationTag
	TTL             int
}

type BackendServerKey struct {
	Address string
	Port    uint16
}

type BackendServerDetails struct {
	ModificationTag routing_api_models.ModificationTag
	TTL             int
	UpdatedTime     time.Time
}

type RoutingTableEntry struct {
	Backends map[BackendServerKey]BackendServerDetails
}

type RoutingTable struct {
	Entries map[RoutingKey]RoutingTableEntry
	logger  lager.Logger
}

func NewRoutingTableEntry(backends []BackendServerInfo) RoutingTableEntry {
	routingTableEntry := RoutingTableEntry{
		Backends: make(map[BackendServerKey]BackendServerDetails),
	}
	for _, backend := range backends {
		backendServerKey := BackendServerKey{Address: backend.Address, Port: backend.Port}
		backendServerDetails := BackendServerDetails{ModificationTag: backend.ModificationTag, TTL: backend.TTL, UpdatedTime: time.Now()}

		routingTableEntry.Backends[backendServerKey] = backendServerDetails
	}
	return routingTableEntry
}

func NewRoutingTable(logger lager.Logger) RoutingTable {
	return RoutingTable{
		Entries: make(map[RoutingKey]RoutingTableEntry),
		logger:  logger.Session("routing-table"),
	}
}

func (e RoutingTableEntry) PruneBackends(defaultTTL int) {
	for backendKey, details := range e.Backends {
		if details.Expired(defaultTTL) {
			delete(e.Backends, backendKey)
		}
	}
}

// Used to determine whether the details have changed such that the routing configuration needs to be updated.
// e.g max number of connection
func (d BackendServerDetails) DifferentFrom(other BackendServerDetails) bool {
	return d.UpdateSucceededBy(other) && false
}

func (d BackendServerDetails) UpdateSucceededBy(other BackendServerDetails) bool {
	return d.ModificationTag.SucceededBy(&other.ModificationTag)
}

func (d BackendServerDetails) DeleteSucceededBy(other BackendServerDetails) bool {
	return d.ModificationTag == other.ModificationTag || d.ModificationTag.SucceededBy(&other.ModificationTag)
}

func (d BackendServerDetails) Expired(defaultTTL int) bool {
	ttl := d.TTL
	if ttl == 0 {
		ttl = defaultTTL
	}

	expiryTime := time.Now().Add(-time.Duration(ttl) * time.Second)

	return expiryTime.After(d.UpdatedTime)
}

func NewBackendServerInfo(key BackendServerKey, detail BackendServerDetails) BackendServerInfo {
	return BackendServerInfo{
		Address:         key.Address,
		Port:            key.Port,
		ModificationTag: detail.ModificationTag,
		TTL:             detail.TTL,
	}
}

func (table RoutingTable) PruneEntries(defaultTTL int) {
	for routeKey, entry := range table.Entries {
		entry.PruneBackends(defaultTTL)
		if len(entry.Backends) == 0 {
			delete(table.Entries, routeKey)
		}
	}
}

func (table RoutingTable) serverKeyDetailsFromInfo(info BackendServerInfo) (BackendServerKey, BackendServerDetails) {
	return BackendServerKey{Address: info.Address, Port: info.Port}, BackendServerDetails{ModificationTag: info.ModificationTag, TTL: info.TTL, UpdatedTime: time.Now()}
}

// Returns true if routing configuration should be modified, false if it should not.
func (table RoutingTable) Set(key RoutingKey, newEntry RoutingTableEntry) bool {
	existingEntry, ok := table.Entries[key]
	if ok == true && reflect.DeepEqual(existingEntry, newEntry) {
		return false
	}
	table.Entries[key] = newEntry
	return true
}

// Returns true if routing configuration should be modified, false if it should not.
func (table RoutingTable) UpsertBackendServerKey(key RoutingKey, info BackendServerInfo) bool {
	logger := table.logger.Session("upsert-backend", lager.Data{"key": key, "info": info})

	existingEntry, routingKeyFound := table.Entries[key]
	if !routingKeyFound {
		logger.Debug("routing-key-not-found", lager.Data{"routing-key": key})
		existingEntry = NewRoutingTableEntry([]BackendServerInfo{info})
		table.Entries[key] = existingEntry
		return true
	}

	newBackendKey, newBackendDetails := table.serverKeyDetailsFromInfo(info)
	currentBackendDetails, backendFound := existingEntry.Backends[newBackendKey]

	detailData := lager.Data{"old": currentBackendDetails, "new": newBackendDetails}
	if !backendFound ||
		currentBackendDetails.UpdateSucceededBy(newBackendDetails) {
		logger.Debug("applying-change-to-table", detailData)
		existingEntry.Backends[newBackendKey] = newBackendDetails
	} else {
		logger.Debug("skipping-stale-event", detailData)
	}

	if !backendFound || currentBackendDetails.DifferentFrom(newBackendDetails) {
		return true
	}

	return false
}

// Returns true if routing configuration should be modified, false if it should not.
func (table RoutingTable) DeleteBackendServerKey(key RoutingKey, info BackendServerInfo) bool {
	logger := table.logger.Session("delete-backend", lager.Data{"key": key, "info": info})

	backendServerKey, newDetails := table.serverKeyDetailsFromInfo(info)
	existingEntry, routingKeyFound := table.Entries[key]

	if routingKeyFound {
		existingDetails, backendFound := existingEntry.Backends[backendServerKey]

		detailData := lager.Data{"old": existingDetails, "new": newDetails}
		if backendFound && existingDetails.DeleteSucceededBy(newDetails) {
			logger.Debug("removing-from-table", detailData)
			delete(existingEntry.Backends, backendServerKey)
			if len(existingEntry.Backends) == 0 {
				delete(table.Entries, key)
			}
			return true
		} else {
			logger.Debug("skipping-stale-event", detailData)
		}
	}

	return false
}

func (table RoutingTable) Get(key RoutingKey) RoutingTableEntry {
	return table.Entries[key]
}

func (table RoutingTable) Size() int {
	return len(table.Entries)
}

func (k RoutingKey) String() string {
	return fmt.Sprintf("%d", k.Port)
}
