// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"cmp"
	"slices"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// CacheStore saves expensive calculations from the K/V store
type CacheStore struct {
	// Given the small amount of data these will store, we use the same structure as the memory store
	cacheLock  sync.Mutex // write heavy - Mutex is faster than RWMutex for writes
	services   map[string]uint64
	operations map[string]map[tracestore.Operation]uint64

	store *badger.DB
	ttl   time.Duration
}

// NewCacheStore returns initialized CacheStore for badger use
func NewCacheStore(db *badger.DB, ttl time.Duration) *CacheStore {
	cs := &CacheStore{
		services:   make(map[string]uint64),
		operations: make(map[string]map[tracestore.Operation]uint64),
		ttl:        ttl,
		store:      db,
	}
	return cs
}

// AddService fills the services into the cache with the most updated expiration time
func (c *CacheStore) AddService(service string, keyTTL uint64) {
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()
	if v, found := c.services[service]; found {
		if v > keyTTL {
			return
		}
	}
	c.services[service] = keyTTL
}

// AddOperation adds the cache with operation names with most updated expiration time.
// spanKind may be empty when the value is not known (e.g. when preloading from legacy index).
func (c *CacheStore) AddOperation(service, spanKind, operation string, keyTTL uint64) {
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()
	if _, found := c.operations[service]; !found {
		c.operations[service] = make(map[tracestore.Operation]uint64)
	}
	op := tracestore.Operation{Name: operation, SpanKind: spanKind}
	if v, found := c.operations[service][op]; found {
		if v > keyTTL {
			return
		}
	}
	c.operations[service][op] = keyTTL
}

// Update caches the results of service and service + operation indexes and maintains their TTL.
// spanKind may be empty when the value is not known (e.g. when preloading from legacy index).
func (c *CacheStore) Update(service, spanKind, operation string, expireTime uint64) {
	c.cacheLock.Lock()

	c.services[service] = expireTime
	if _, ok := c.operations[service]; !ok {
		c.operations[service] = make(map[tracestore.Operation]uint64)
	}
	c.operations[service][tracestore.Operation{Name: operation, SpanKind: spanKind}] = expireTime
	c.cacheLock.Unlock()
}

// GetOperations returns all operations for a specific service, optionally filtered by spanKind.
func (c *CacheStore) GetOperations(query tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	//nolint:gosec // G115
	t := uint64(time.Now().Unix())
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()

	if v, ok := c.services[query.ServiceName]; ok {
		if v < t {
			// Expired, remove
			delete(c.services, query.ServiceName)
			delete(c.operations, query.ServiceName)
			return []tracestore.Operation{}, nil // empty slice rather than nil
		}
	}

	var result []tracestore.Operation
	for op, e := range c.operations[query.ServiceName] {
		if e <= t {
			delete(c.operations[query.ServiceName], op)
			continue
		}
		if query.SpanKind == "" || query.SpanKind == op.SpanKind {
			result = append(result, op)
		}
	}

	slices.SortFunc(result, func(a, b tracestore.Operation) int {
		if c := cmp.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return cmp.Compare(a.SpanKind, b.SpanKind)
	})

	return result, nil
}

// GetServices returns all services traced by Jaeger
func (c *CacheStore) GetServices() ([]string, error) {
	services := make([]string, 0, len(c.services))
	//nolint:gosec // G115
	t := uint64(time.Now().Unix())
	c.cacheLock.Lock()
	// Fetch the items
	for k, v := range c.services {
		if v > t {
			services = append(services, k)
		} else {
			// Service has expired, remove it
			delete(c.services, k)
		}
	}
	c.cacheLock.Unlock()

	slices.Sort(services)

	return services, nil
}
