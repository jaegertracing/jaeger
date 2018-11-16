// Copyright (c) 2018 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spanstore

import (
	"sort"
	"sync"
	"time"

	"github.com/dgraph-io/badger"
)

// CacheStore saves expensive calculations from the K/V store
type CacheStore struct {
	// Given the small amount of data these will store, we use the same structure as the memory store
	cacheLock  sync.Mutex // write heavy - Mutex is faster than RWMutex for writes
	services   map[string]int64
	operations map[string]map[string]int64

	store *badger.DB
	ttl   time.Duration
}

// NewCacheStore returns initialized CacheStore for badger use
func NewCacheStore(db *badger.DB, ttl time.Duration) (*CacheStore, error) {
	cs := &CacheStore{
		services:   make(map[string]int64),
		operations: make(map[string]map[string]int64),
		ttl:        ttl,
		store:      db,
	}

	err := cs.prefillCaches()
	return cs, err
}

func (c *CacheStore) prefillCaches() error {
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()

	if err := c.loadServices(); err != nil {
		return err
	}

	for k := range c.services {
		if err := c.loadOperations(k); err != nil {
			return err
		}
	}

	return nil
}

func (c *CacheStore) loadServices() error {
	err := c.store.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		serviceKey := []byte{serviceNameIndexKey}

		// Seek all the services first
		for it.Seek(serviceKey); it.ValidForPrefix(serviceKey); it.Next() {
			timestampStartIndex := len(it.Item().Key()) - (sizeOfTraceID + 8) // 8 = sizeof(uint64)
			serviceName := string(it.Item().Key()[len(serviceKey):timestampStartIndex])
			keyTTL := int64(it.Item().ExpiresAt())
			if v, found := c.services[serviceName]; found {
				if v > keyTTL {
					continue
				}
			}
			c.services[serviceName] = keyTTL
		}
		return nil
	})

	return err
}

func (c *CacheStore) loadOperations(service string) error {

	err := c.store.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		serviceKey := make([]byte, 0, len(service)+1)
		serviceKey = append(serviceKey, operationNameIndexKey)
		serviceKey = append(serviceKey, service...)

		// Seek all the services first
		for it.Seek(serviceKey); it.ValidForPrefix(serviceKey); it.Next() {
			timestampStartIndex := len(it.Item().Key()) - (sizeOfTraceID + 8) // 8 = sizeof(uint64)
			operationName := string(it.Item().Key()[len(serviceKey):timestampStartIndex])
			keyTTL := int64(it.Item().ExpiresAt())
			if _, found := c.operations[service]; !found {
				c.operations[service] = make(map[string]int64)
			}

			if v, found := c.operations[service][operationName]; found {
				if v > keyTTL {
					continue
				}
			}
			c.operations[service][operationName] = keyTTL
		}
		return nil
	})

	return err
}

// Update caches the results of service and service + operation indexes and maintains their TTL
func (c *CacheStore) Update(service string, operation string) {
	c.cacheLock.Lock()
	t := time.Now().Add(c.ttl).Unix()

	c.services[service] = t
	if _, ok := c.operations[service]; !ok {
		c.operations[service] = make(map[string]int64)
	}
	c.operations[service][operation] = t
	c.cacheLock.Unlock()
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (c *CacheStore) GetOperations(service string) ([]string, error) {
	operations := make([]string, 0, len(c.services))
	t := time.Now().Unix()
	c.cacheLock.Lock()
	defer c.cacheLock.Unlock()

	if v, ok := c.services[service]; ok {
		if v < t {
			// Expired, remove
			delete(c.services, service)
			delete(c.operations, service)
			return operations, nil // empty slice rather than nil
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

	return operations, nil
}

// GetServices returns all services traced by Jaeger
func (c *CacheStore) GetServices() ([]string, error) {
	services := make([]string, 0, len(c.services))
	t := time.Now().Unix()
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
