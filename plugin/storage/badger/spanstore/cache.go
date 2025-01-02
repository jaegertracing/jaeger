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
		// This will firstly load all the data with kind as unspecified
		c.loadOperations(k)
		// This will load the new data with proper kind (overriding the old data)
		c.loadOperationsWithKind(k)
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
		opts.PrefetchValues = false
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
			kind := model.SpanKindUnspecified
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

func (c *CacheStore) loadOperationsWithKind(service string) {
	c.store.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		serviceKey := make([]byte, len(service)+1)
		serviceKey[0] = operationNameWithKindIndexKey
		copy(serviceKey[1:], service)
		for it.Seek(serviceKey); it.ValidForPrefix(serviceKey); it.Next() {
			timestampStartIndex := len(it.Item().Key()) - (sizeOfTraceID + 8)
			operationNameAndKind := string(it.Item().Key()[len(serviceKey):timestampStartIndex])
			keyTTL := it.Item().ExpiresAt()
			kind := getBadgerKindFromString(string(operationNameAndKind[0])).spanKind()
			operationName := operationNameAndKind[1:]
			if _, found := c.operations[service]; !found {
				c.operations[service] = make(map[model.SpanKind]map[string]uint64)
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
