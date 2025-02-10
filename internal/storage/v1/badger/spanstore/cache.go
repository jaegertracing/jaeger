// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"sort"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"

	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
)

// CacheStore saves expensive calculations from the K/V store
type CacheStore struct {
	// Given the small amount of data these will store, we use the same structure as the memory store
	cacheLock  sync.Mutex // write heavy - Mutex is faster than RWMutex for writes
	services   map[string]uint64
	operations map[string]map[string]uint64

	store *badger.DB
	ttl   time.Duration
}

// NewCacheStore returns initialized CacheStore for badger use
func NewCacheStore(db *badger.DB, ttl time.Duration) *CacheStore {
	cs := &CacheStore{
		services:   make(map[string]uint64),
		operations: make(map[string]map[string]uint64),
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

// AddOperation adds the cache with operation names with most updated expiration time
func (c *CacheStore) AddOperation(service, operation string, keyTTL uint64) {
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()
	if _, found := c.operations[service]; !found {
		c.operations[service] = make(map[string]uint64)
	}
	if v, found := c.operations[service][operation]; found {
		if v > keyTTL {
			return
		}
	}
	c.operations[service][operation] = keyTTL
}

// Update caches the results of service and service + operation indexes and maintains their TTL
func (c *CacheStore) Update(service, operation string, expireTime uint64) {
	c.cacheLock.Lock()

	c.services[service] = expireTime
	if _, ok := c.operations[service]; !ok {
		c.operations[service] = make(map[string]uint64)
	}
	c.operations[service][operation] = expireTime
	c.cacheLock.Unlock()
}

// GetOperations returns all operations for a specific service & spanKind traced by Jaeger
func (c *CacheStore) GetOperations(service string) ([]spanstore.Operation, error) {
	operations := make([]string, 0, len(c.services))
	//nolint: gosec // G115
	t := uint64(time.Now().Unix())
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()

	if v, ok := c.services[service]; ok {
		if v < t {
			// Expired, remove
			delete(c.services, service)
			delete(c.operations, service)
			return []spanstore.Operation{}, nil // empty slice rather than nil
		}
		for o, e := range c.operations[service] {
			if e > t {
				operations = append(operations, o)
			} else {
				delete(c.operations[service], o)
			}
		}
	}

	sort.Strings(operations)

	// TODO: https://github.com/jaegertracing/jaeger/issues/1922
	// 	- return the operations with actual spanKind
	result := make([]spanstore.Operation, 0, len(operations))
	for _, op := range operations {
		result = append(result, spanstore.Operation{
			Name: op,
		})
	}
	return result, nil
}

// GetServices returns all services traced by Jaeger
func (c *CacheStore) GetServices() ([]string, error) {
	services := make([]string, 0, len(c.services))
	//nolint: gosec // G115
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

	sort.Strings(services)

	return services, nil
}
