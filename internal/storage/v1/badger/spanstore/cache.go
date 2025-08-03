// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"sort"
	"sync"
	"time"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
)

// CacheStore saves expensive calculations from the K/V store
type CacheStore struct {
	// Given the small amount of data these will store, we use the same structure as the memory store
	cacheLock sync.Mutex // write heavy - Mutex is faster than RWMutex for writes
	services  map[string]uint64
	// This map is for the hierarchy: service name, kind and operation name.
	// Each service contains the span kinds, and then operation names belonging to that kind.
	// This structure will look like:
	/*
		"service1":{
			SpanKind.unspecified: {
				"operation1": uint64
			}
		}
	*/
	// The uint64 value is the expiry time of operation
	operations map[string]map[model.SpanKind]map[string]uint64

	ttl time.Duration
}

// NewCacheStore returns initialized CacheStore for badger use
func NewCacheStore(ttl time.Duration) *CacheStore {
	cs := &CacheStore{
		services:   make(map[string]uint64),
		operations: make(map[string]map[model.SpanKind]map[string]uint64),
		ttl:        ttl,
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
func (c *CacheStore) AddOperation(service, operation string, kind model.SpanKind, keyTTL uint64) {
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()
	if _, found := c.operations[service]; !found {
		c.operations[service] = make(map[model.SpanKind]map[string]uint64)
	}
	if _, found := c.operations[service][kind]; !found {
		c.operations[service][kind] = make(map[string]uint64)
	}
	if v, found := c.operations[service][kind][operation]; found {
		if v > keyTTL {
			return
		}
	}
	c.operations[service][kind][operation] = keyTTL
}

// Update caches the results of service and service + operation indexes and maintains their TTL
func (c *CacheStore) Update(service, operation string, kind model.SpanKind, expireTime uint64) {
	c.cacheLock.Lock()

	c.services[service] = expireTime
	if _, found := c.operations[service]; !found {
		c.operations[service] = make(map[model.SpanKind]map[string]uint64)
	}
	if _, found := c.operations[service][kind]; !found {
		c.operations[service][kind] = make(map[string]uint64)
	}
	c.operations[service][kind][operation] = expireTime
	c.cacheLock.Unlock()
}

// GetOperations returns all operations for a specific service & spanKind traced by Jaeger
func (c *CacheStore) GetOperations(service string, kind string) ([]spanstore.Operation, error) {
	operations := make([]spanstore.Operation, 0, len(c.services))
	//nolint: gosec // G115
	currentTime := uint64(time.Now().Unix())
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()
	if expiryTimeOfService, ok := c.services[service]; ok {
		if expiryTimeOfService < currentTime {
			// Expired, remove
			delete(c.services, service)
			delete(c.operations, service)
			return []spanstore.Operation{}, nil // empty slice rather than nil
		}
		for sKind := range c.operations[service] {
			if kind != "" && kind != string(sKind) {
				continue
			}
			for o, expiryTimeOfOperation := range c.operations[service][sKind] {
				if expiryTimeOfOperation > currentTime {
					op := spanstore.Operation{Name: o, SpanKind: string(sKind)}
					operations = append(operations, op)
				} else {
					delete(c.operations[service][sKind], o)
				}
				sort.Slice(operations, func(i, j int) bool {
					if operations[i].SpanKind == operations[j].SpanKind {
						return operations[i].Name < operations[j].Name
					}
					return operations[i].SpanKind < operations[j].SpanKind
				})
			}
		}
	}
	return operations, nil
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
