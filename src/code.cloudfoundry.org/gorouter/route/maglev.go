package route

// Original https://github.com/kkdai/maglev
//
// Copyright (c) 2019 Evan Lin (github.com/kkdai)
//
// This program and the accompanying materials are made available under
// the terms of the Apache License, Version 2.0 which is available at
// http://www.apache.org/licenses/LICENSE-2.0.
//
// CHANGES:
// - Modified for integration with CF GoRouter
// - Added MaglevLookup interface for testability and abstraction
// - Enhanced with structured logging using slog
// - Added thread-safe operations
// - Extended with getter methods for unit testing
// - Added error handling and safety checks
// - Customized for hash-based routing requirements

import (
	"context"
	"errors"
	"hash/fnv"
	"log/slog"
	"sort"
	"sync"
)

// Int subtype of lookup table (int16/int32/int64) limits the maximum possible size.
// The table size is a prime number, which offers effective distribution for applications with up to 30 endpoints. //
const lookupTableSize uint64 = 3001

// permutationParams stores the parameters needed to compute permutation values on-the-fly
type permutationParams struct {
	offset uint64
	skip   uint64
}

// MaglevLookup defines the interface for consistent hashing lookup table implementations.
// This interface allows for different implementations of the Maglev algorithm and
// enables easy testing with mock implementations.
type MaglevLookup interface {
	// Add a new endpoint to the lookup table
	Add(endpoint string)

	// Remove an endpoint from the lookup table
	Remove(endpoint string)

	// GetInstanceForHashHeader endpoint by specified request header value
	GetInstanceForHashHeader(hashHeaderValue string) (uint64, string, error)

	// GetEndpointId returns the endpoint ID by specified lookup table index
	GetEndpointId(lookupTableIndex uint64) string

	// GetLookupTableSize returns the size of the lookup table
	GetLookupTableSize() uint64

	// GetEndpointList returns a copy of the current endpoint list (for testing)
	GetEndpointList() []string

	// GetLookupTable returns a copy of the current lookup table (for testing)
	GetLookupTable() []int16

	// GetPermutationTable returns a copy of the current permutation table (for testing)
	GetPermutationTable() [][]uint16
}

// Maglev implementation of consistent hashing algorithm described in "Maglev: A Fast and Reliable Software Network
// Load Balancer" (https://storage.googleapis.com/gweb-research2023-media/pubtools/2904.pdf)
type Maglev struct {
	logger          *slog.Logger
	lookupTableSize uint64
	permutations    []permutationParams // Stores offset and skip for computing permutations on-the-fly
	lookupTable     []int16
	endpointList    []string
	lock            *sync.RWMutex
}

// NewMaglev initializes an empty maglev lookupTable table
func NewMaglev(logger *slog.Logger) *Maglev {
	if logger.Enabled(context.Background(), slog.LevelDebug) {
		logger.Debug("maglev-initialized", slog.Uint64("lookup-table-size", lookupTableSize))
	}
	return &Maglev{
		lock:            &sync.RWMutex{},
		lookupTableSize: lookupTableSize,
		lookupTable:     make([]int16, lookupTableSize),
		endpointList:    make([]string, 0, 2),
		permutations:    make([]permutationParams, 0, 2),
		logger:          logger,
	}
}

// Add a new endpoint to lookupTable if it's not already contained.
func (m *Maglev) Add(endpoint string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if m.lookupTableSize == uint64(len(m.endpointList)) {
		m.logger.Warn("maglev-add-lookuptable-capacity-exceeded", slog.String("endpoint-id", endpoint))
		return
	}

	index := sort.SearchStrings(m.endpointList, endpoint)
	if index < len(m.endpointList) && m.endpointList[index] == endpoint {
		if m.logger.Enabled(context.Background(), slog.LevelDebug) {
			m.logger.Debug("maglev-add-lookuptable-endpoint-exists", slog.String("endpoint-id", endpoint), slog.Int("current-endpoints", len(m.endpointList)))
		}
		return
	}

	m.endpointList = append(m.endpointList, "")
	copy(m.endpointList[index+1:], m.endpointList[index:])
	m.endpointList[index] = endpoint
	m.logger.Info("maglev-add-endpoint", slog.String("endpoint-id", endpoint), slog.Int("current-endpoints", len(m.endpointList)))

	m.generatePermutation(endpoint)
	m.fillLookupTable()
}

// Remove an endpoint from lookupTable if it's contained.
func (m *Maglev) Remove(endpoint string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	index := sort.SearchStrings(m.endpointList, endpoint)
	if index >= len(m.endpointList) || m.endpointList[index] != endpoint {
		m.logger.Debug("maglev-remove-endpoint-not-found", slog.String("endpoint-id", endpoint))
		return
	}

	m.endpointList = append(m.endpointList[:index], m.endpointList[index+1:]...)
	m.permutations = append(m.permutations[:index], m.permutations[index+1:]...)

	if m.logger.Enabled(context.Background(), slog.LevelDebug) {
		m.logger.Debug("maglev-remove-endpoint", slog.String("endpoint-id", endpoint), slog.Int("current-endpoints", len(m.endpointList)))
	}

	m.fillLookupTable()
}

// GetInstanceForHashHeader lookup table index and private instance ID for the specified request header value
func (m *Maglev) GetInstanceForHashHeader(hashHeaderValue string) (uint64, string, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if len(m.endpointList) == 0 {
		return 0, "", errors.New("no endpoint available")
	}
	key := m.calculateFNVHash64(hashHeaderValue)
	index := key % m.lookupTableSize
	return index, m.endpointList[m.lookupTable[key%m.lookupTableSize]], nil
}

// GetEndpointId by specified lookup table index
func (m *Maglev) GetEndpointId(lookupTableIndex uint64) string {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return m.endpointList[m.lookupTable[lookupTableIndex]]
}

// generatePermutation stores the permutation parameters (offset and skip) for the endpoint
func (m *Maglev) generatePermutation(endpoint string) {
	pos := sort.SearchStrings(m.endpointList, endpoint)
	if pos == len(m.endpointList) {
		m.logger.Debug("maglev-permutation-no-endpoints")
		return
	}

	endpointHash := m.calculateFNVHash64(endpoint)
	permutationParameters := permutationParams{
		offset: endpointHash % m.lookupTableSize,
		skip:   (endpointHash % (m.lookupTableSize - 1)) + 1,
	}

	// insert params at position pos, shifting the rest to the right
	m.permutations = append(m.permutations, permutationParams{})
	copy(m.permutations[pos+1:], m.permutations[pos:])
	m.permutations[pos] = permutationParameters
}

// computePermutation calculates the permutation value for endpoint i at position j on-the-fly
func (m *Maglev) computePermutation(i int, j int) uint16 {
	params := m.permutations[i]
	return uint16((params.offset + uint64(j)*params.skip) % m.lookupTableSize)
}

func (m *Maglev) fillLookupTable() {
	if len(m.endpointList) == 0 {
		return
	}

	numberOfEndpoints := len(m.endpointList)
	next := make([]int, numberOfEndpoints)
	entry := make([]int16, m.lookupTableSize)
	for j := range entry {
		entry[j] = -1
	}

	for n := uint64(0); n <= m.lookupTableSize; {
		for i := 0; i < numberOfEndpoints; i++ {
			candidate := m.findNextAvailableSlot(i, next, entry)
			entry[candidate] = int16(i)
			next[i] = next[i] + 1
			n++

			if n == m.lookupTableSize {
				m.lookupTable = entry
				return
			}
		}
	}
}

func (m *Maglev) findNextAvailableSlot(i int, next []int, entry []int16) uint16 {
	candidate := m.computePermutation(i, next[i])
	for entry[candidate] >= 0 {
		next[i]++
		if next[i] >= int(m.lookupTableSize) {
			// This should not happen in a properly functioning Maglev algorithm,
			// but we add this safety check to prevent panic
			m.logger.Error("maglev-permutation-table-exhausted",
				slog.Int("endpoint-index", i),
				slog.Int("next-value", next[i]),
				slog.Int("table-size", int(m.lookupTableSize)))
			// Reset to beginning of permutation table as fallback
			next[i] = 0
		}
		candidate = m.computePermutation(i, next[i])
	}
	return candidate
}

// GetEndpointList is used in unit tests
func (m *Maglev) GetEndpointList() []string {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return append([]string(nil), m.endpointList...)
}

func (m *Maglev) GetLookupTable() []int16 {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return append([]int16(nil), m.lookupTable...)
}

func (m *Maglev) GetPermutationTable() [][]uint16 {
	m.lock.RLock()
	defer m.lock.RUnlock()

	// Compute permutation table on-the-fly for testing
	copied := make([][]uint16, len(m.permutations))
	for i := range m.permutations {
		copied[i] = make([]uint16, m.lookupTableSize)
		for j := uint64(0); j < m.lookupTableSize; j++ {
			copied[i][j] = m.computePermutation(i, int(j))
		}
	}
	return copied
}

func (m *Maglev) GetLookupTableSize() uint64 {
	return m.lookupTableSize
}

// calculateFNVHash64 computes a hash using the non-cryptographic FNV hash algorithm.
func (m *Maglev) calculateFNVHash64(key string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return h.Sum64()
}

// Compile-time check to ensure Maglev implements MaglevLookup interface
var _ MaglevLookup = (*Maglev)(nil)
