package route

/******************************************************************************
 * Original github.com/kkdai/maglev/maglev.go
 *
 * Copyright (c) 2019 Evan Lin (github.com/kkdai)
 *
 * This program and the accompanying materials are made available under
 * the terms of the Apache License, Version 2.0 which is available at
 * http://www.apache.org/licenses/LICENSE-2.0.
 ******************************************************************************/

import (
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	// lookupTableSize is prime number for the size of the maglev lookup table, which should be approximately 100x
	// the number of expected endpoints
	lookupTableSize uint64 = 1801
)

// Maglev implementation of consistent hashing algorithm described in "Maglev: A Fast and Reliable Software Network
// Load Balancer" (https://storage.googleapis.com/gweb-research2023-media/pubtools/2904.pdf)
type Maglev struct {
	logger           *slog.Logger
	permutationTable [][]uint64
	lookupTable      []int
	endpointList     []string
	lock             *sync.RWMutex
}

// NewMaglev initializes an empty maglev lookupTable table
func NewMaglev(logger *slog.Logger) *Maglev {
	return &Maglev{
		lock:             &sync.RWMutex{},
		lookupTable:      make([]int, lookupTableSize),
		endpointList:     make([]string, 0, 2),
		permutationTable: make([][]uint64, 0, 2),
		logger:           logger,
	}
}

// Add a new endpoint to lookupTable if it's not already contained.
func (m *Maglev) Add(endpoint string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if lookupTableSize == uint64(len(m.endpointList)) {
		m.logger.Warn("maglev-add-lookuptable-capacity-exceeded", slog.String("endpoint-id", endpoint))
		return
	}

	index := sort.SearchStrings(m.endpointList, endpoint)
	if index < len(m.endpointList) && m.endpointList[index] == endpoint {
		m.logger.Debug("maglev-add-lookuptable-endpoint-exists", slog.String("endpoint-id", endpoint), slog.Int("current-endpoints", len(m.endpointList)))
		return
	}

	m.endpointList = append(m.endpointList, "")
	copy(m.endpointList[index+1:], m.endpointList[index:])
	m.endpointList[index] = endpoint

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
	m.permutationTable = append(m.permutationTable[:index], m.permutationTable[index+1:]...)

	m.fillLookupTable()
}

// Get endpoint by specified request header value
// Todo: Overload scenario: Get should return an index rather than an instance,
// so that we can iterate to the next endpoint in case it is overloaded (e.g. via another
// helper function that resolves the endpoint via the index)
func (m *Maglev) Get(headerValue string) (string, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if len(m.endpointList) == 0 {
		return "", errors.New("maglev-get-endpoint-no-endpoints")
	}
	key := m.hashKey(headerValue)
	return m.endpointList[m.lookupTable[key%lookupTableSize]], nil
}

func (m *Maglev) hashKey(headerValue string) uint64 {
	return m.calculateFNVHash64(headerValue)
}

// generatePermutation creates a permutationTable of the lookup table for each endpoint
func (m *Maglev) generatePermutation(endpoint string) {
	pos := sort.SearchStrings(m.endpointList, endpoint)
	if pos == len(m.endpointList) {
		m.logger.Debug("maglev-permutation-no-endpoints")
		return
	}

	endpointHash := m.calculateFNVHash64(endpoint)
	offset := endpointHash % lookupTableSize
	skip := (endpointHash % (lookupTableSize - 1)) + 1

	permutationForEndpoint := make([]uint64, lookupTableSize)
	for j := uint64(0); j < lookupTableSize; j++ {
		permutationForEndpoint[j] = (offset + j*skip) % lookupTableSize
	}

	// insert permutationForEndpoint at position pos, shifting the rest to the right
	m.permutationTable = append(m.permutationTable, nil)
	copy(m.permutationTable[pos+1:], m.permutationTable[pos:])
	m.permutationTable[pos] = permutationForEndpoint

}

func (m *Maglev) fillLookupTable() {
	if len(m.endpointList) == 0 {
		return
	}

	numberOfEndpoints := len(m.endpointList)
	next := make([]int, numberOfEndpoints)
	entry := make([]int, lookupTableSize)
	for j := range entry {
		entry[j] = -1
	}

	for n := uint64(0); n <= lookupTableSize; {
		for i := 0; i < numberOfEndpoints; i++ {
			candidate := m.findNextAvailableSlot(i, next, entry)
			entry[candidate] = int(i)
			next[i] = next[i] + 1
			n++

			if n == lookupTableSize {
				m.lookupTable = entry
				return
			}
		}
	}
}

func (m *Maglev) findNextAvailableSlot(i int, next []int, entry []int) uint64 {
	candidate := m.permutationTable[i][next[i]]
	for entry[candidate] >= 0 {
		next[i]++
		if next[i] >= len(m.permutationTable[i]) {
			// This should not happen in a properly functioning Maglev algorithm,
			// but we add this safety check to prevent panic
			m.logger.Error("maglev-permutation-table-exhausted",
				slog.Int("endpoint-index", i),
				slog.Int("next-value", next[i]),
				slog.Int("table-size", len(m.permutationTable[i])))
			// Reset to beginning of permutation table as fallback
			next[i] = 0
		}
		candidate = m.permutationTable[i][next[i]]
	}
	return candidate
}

// Getters for unit tests
func (m *Maglev) GetEndpointList() []string {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return append([]string(nil), m.endpointList...)
}

func (m *Maglev) GetLookupTable() []int {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return append([]int(nil), m.lookupTable...)
}

func (m *Maglev) GetPermutationTable() [][]uint64 {
	m.lock.RLock()
	defer m.lock.RUnlock()
	copied := make([][]uint64, len(m.permutationTable))
	for i, v := range m.permutationTable {
		copied[i] = append([]uint64(nil), v...)
	}
	return copied
}

func (m *Maglev) GetLookupTableSize() uint64 {
	return lookupTableSize
}

// TODO: Remove in final version
func (m *Maglev) PrintLookupTable() string {
	strArr := make([]string, len(m.lookupTable))
	for i, value := range m.lookupTable {
		strArr[i] = strconv.Itoa(value)
	}
	return fmt.Sprintf("[%s]", strings.Join(strArr, ", "))
}

// calculateFNVHash64 computes a hash using the non-cryptographic FNV hash algorithm.
func (m *Maglev) calculateFNVHash64(key string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return h.Sum64()
}
