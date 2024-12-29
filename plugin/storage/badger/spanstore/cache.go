// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"sort"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
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

	store *badger.DB
	ttl   time.Duration
}

// NewCacheStore returns initialized CacheStore for badger use
func NewCacheStore(db *badger.DB, ttl time.Duration, prefill bool) *CacheStore {
	cs := &CacheStore{
		services:   make(map[string]uint64),
		operations: make(map[string]map[model.SpanKind]map[string]uint64),
		ttl:        ttl,
		store:      db,
	}

	if prefill {
		cs.populateCaches()
	}
	return cs
}

func (c *CacheStore) populateCaches() {
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()

	c.loadServices()

	for k := range c.services {
		c.loadOperations(k)
	}
}

func (c *CacheStore) loadServices() {
	c.store.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		serviceKey := []byte{serviceNameIndexKey}

		// Seek all the services first
		for it.Seek(serviceKey); it.ValidForPrefix(serviceKey); it.Next() {
			timestampStartIndex := len(it.Item().Key()) - (sizeOfTraceID + 8) // 8 = sizeof(uint64)
			serviceName := string(it.Item().Key()[len(serviceKey):timestampStartIndex])
			keyTTL := it.Item().ExpiresAt()
			if v, found := c.services[serviceName]; found {
				if v > keyTTL {
					continue
				}
			}
			c.services[serviceName] = keyTTL
		}
		return nil
	})
}

func (c *CacheStore) loadOperations(service string) {
	c.store.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		serviceKey := make([]byte, len(service)+1)
		serviceKey[0] = operationNameIndexKey
		copy(serviceKey[1:], service)

		// Seek all the services first
		for it.Seek(serviceKey); it.ValidForPrefix(serviceKey); it.Next() {
			timestampStartIndex := len(it.Item().Key()) - (sizeOfTraceID + 8) // 8 = sizeof(uint64)
			operationName := string(it.Item().Key()[len(serviceKey):timestampStartIndex])
			keyTTL := it.Item().ExpiresAt()
			if _, found := c.operations[service]; !found {
				c.operations[service] = make(map[model.SpanKind]map[string]uint64)
			}
			var kind model.SpanKind
			err := it.Item().Value(func(val []byte) error {
				if val == nil {
					kind = model.SpanKindUnspecified
					return nil
				}
				var err error
				kind, err = model.SpanKindFromString(string(val))
				return err
			})
			if err != nil {
				kind = model.SpanKindUnspecified
			}
			if _, found := c.operations[service][kind]; !found {
				c.operations[service][kind] = make(map[string]uint64)
			}
			if v, found := c.operations[service][kind][operationName]; found {
				if v > keyTTL {
					continue
				}
			}
			c.operations[service][kind][operationName] = keyTTL
		}
		return nil
	})
}

// Update caches the results of service and service + operation indexes and maintains their TTL
func (c *CacheStore) Update(service, operation string, kind model.SpanKind, expireTime uint64) {
	c.cacheLock.Lock()
	c.services[service] = expireTime
	if _, ok := c.operations[service]; !ok {
		c.operations[service] = make(map[model.SpanKind]map[string]uint64)
	}
	if _, ok := c.operations[service][kind]; !ok {
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
		if kind != "" {
			spanKind, err := model.SpanKindFromString(kind)
			if err != nil {
				return nil, err
			}
			for o, e := range c.operations[service][spanKind] {
				operations = insertOperations(c, operations, service, o, spanKind, currentTime, e)
				sort.Slice(operations, func(i, j int) bool {
					return operations[i].Name < operations[j].Name
				})
			}
		} else {
			for sKind := range c.operations[service] {
				for o, expiryTimeOfOperation := range c.operations[service][sKind] {
					operations = insertOperations(c, operations, service, o, sKind, currentTime, expiryTimeOfOperation)
					sort.Slice(operations, func(i, j int) bool {
						if operations[i].SpanKind == operations[j].SpanKind {
							return operations[i].Name < operations[j].Name
						}
						return operations[i].SpanKind < operations[j].SpanKind
					})
				}
			}
		}
	}
	return operations, nil
}

// insertOperations put the operations into operations array if not expired, if expired then it removes it from the cache!
func insertOperations(c *CacheStore, operations []spanstore.Operation, service, operation string, kind model.SpanKind, currentTime, expiryTime uint64) []spanstore.Operation {
	if expiryTime > currentTime {
		op := spanstore.Operation{Name: operation, SpanKind: string(kind)}
		operations = append(operations, op)
		return operations
	}
	delete(c.operations[service][kind], operation)
	return operations
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
