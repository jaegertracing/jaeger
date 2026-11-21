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
	prefill    sync.Once
	readiness  OperationIndexReadiness

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
		keyTTL = laterExpiry(v, keyTTL)
	}
	c.services[service] = keyTTL
}

// AddOperation adds the cache with operation names with most updated expiration time
func (c *CacheStore) AddOperation(service, operation string, keyTTL uint64) {
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()
	c.cacheOperation(service, tracestore.Operation{Name: operation}, keyTTL)
}

func (c *CacheStore) cacheOperation(service string, operation tracestore.Operation, keyTTL uint64) {
	if _, found := c.operations[service]; !found {
		c.operations[service] = make(map[tracestore.Operation]uint64)
	}
	if v, found := c.operations[service][operation]; found {
		keyTTL = laterExpiry(v, keyTTL)
	}
	c.operations[service][operation] = keyTTL
}

// Update caches the results of service and service + operation indexes and maintains their TTL
func (c *CacheStore) Update(service, operation string, expireTime uint64) {
	c.updateOperations(service, operation, []string{""}, expireTime, expireTime)
}

func (c *CacheStore) updateOperations(service, operation string, kinds []string, serviceExpiry, operationExpiry uint64) {
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()
	if expiry, found := c.services[service]; found {
		serviceExpiry = laterExpiry(expiry, serviceExpiry)
	}
	c.services[service] = serviceExpiry
	for _, kind := range kinds {
		c.cacheOperation(service, tracestore.Operation{Name: operation, SpanKind: kind}, operationExpiry)
	}
}

// GetOperations returns distinct operation names for a service using legacy query semantics.
func (c *CacheStore) GetOperations(service string) ([]tracestore.Operation, error) {
	return c.queryOperations(tracestore.OperationQueryParams{ServiceName: service}, false)
}

func (c *CacheStore) queryOperations(query tracestore.OperationQueryParams, withKinds bool) ([]tracestore.Operation, error) {
	operations := make([]tracestore.Operation, 0)
	//nolint:gosec // G115
	t := uint64(time.Now().Unix())
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()

	service := query.ServiceName
	if v, ok := c.services[service]; ok {
		if v != 0 && v <= t {
			// Expired, remove
			delete(c.services, service)
			delete(c.operations, service)
			return []tracestore.Operation{}, nil // empty slice rather than nil
		}
		names := make(map[string]struct{})
		for o, e := range c.operations[service] {
			if e == 0 || e > t {
				if withKinds {
					if query.SpanKind == "" || query.SpanKind == o.SpanKind {
						operations = append(operations, o)
					}
				} else if _, found := names[o.Name]; !found {
					names[o.Name] = struct{}{}
					operations = append(operations, tracestore.Operation{Name: o.Name})
				}
			} else {
				delete(c.operations[service], o)
			}
		}
	}

	slices.SortFunc(operations, func(a, b tracestore.Operation) int {
		if order := cmp.Compare(a.Name, b.Name); order != 0 {
			return order
		}
		return cmp.Compare(a.SpanKind, b.SpanKind)
	})
	return operations, nil
}

// GetServices returns all services traced by Jaeger
func (c *CacheStore) GetServices() ([]string, error) {
	//nolint:gosec // G115
	t := uint64(time.Now().Unix())
	c.cacheLock.Lock()
	services := make([]string, 0, len(c.services))
	// Fetch the items
	for k, v := range c.services {
		if v == 0 || v > t {
			services = append(services, k)
		} else {
			// Service has expired, remove it along with its operations
			delete(c.services, k)
			delete(c.operations, k)
		}
	}
	c.cacheLock.Unlock()

	slices.Sort(services)

	return services, nil
}
