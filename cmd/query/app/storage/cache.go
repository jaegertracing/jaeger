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

package storage

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// Cache is used to read from the underlying data store
type Cache struct {
	storage spanstore.Reader
	cache   *autoRefreshCache
	logger  *zap.Logger
}

// autoRefreshCache cache that automatically refreshes itself
type autoRefreshCache struct {
	sync.RWMutex

	cache           []string
	refreshInterval time.Duration
	storage         spanstore.Reader
	stopRefreshChan chan struct{}
	logger          *zap.Logger
}

// NewCache returns a new cache that caches services data and automatically refreshes the data
func NewCache(storage spanstore.Reader, refreshInterval time.Duration, logger *zap.Logger) *Cache {
	return &Cache{
		storage: storage,
		logger:  logger,
		cache: &autoRefreshCache{
			storage:         storage,
			refreshInterval: refreshInterval,
			stopRefreshChan: make(chan struct{}),
			logger:          logger,
		},
	}
}

// FindTraces implements spanstore.Reader#FindTraces
func (c *Cache) FindTraces(ctx context.Context, traceQuery *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	return c.storage.FindTraces(ctx, traceQuery)
}

// GetTrace is used to populate a single trace using its traceID
func (c *Cache) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	return c.storage.GetTrace(ctx, traceID)
}

// GetServices returns all services traced by Jaeger
func (c *Cache) GetServices(ctx context.Context) ([]string, error) {
	return c.cache.get(), nil
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (c *Cache) GetOperations(ctx context.Context, service string) ([]string, error) {
	return c.storage.GetOperations(ctx, service)
}

// Initialize warm the cache and start up the auto refresh cache
func (c *Cache) Initialize() {
	if err := c.cache.warmCache(); err != nil {
		c.logger.Error("Cannot warm cache from storage", zap.Error(err))
	}
	c.cache.initializeCacheRefresh()
}

// Close stop refreshing the auto refresh cache
func (c *Cache) Close() error {
	close(c.cache.stopRefreshChan)
	return nil
}

func (ac *autoRefreshCache) get() []string {
	ac.RLock()
	defer ac.RUnlock()
	return ac.cache
}

func (ac *autoRefreshCache) put(value []string) {
	ac.Lock()
	defer ac.Unlock()
	ac.cache = value
}

func (ac *autoRefreshCache) isEmpty() bool {
	ac.RLock()
	defer ac.RUnlock()
	return len(ac.cache) == 0
}

func (ac *autoRefreshCache) initializeCacheRefresh() {
	go ac.refreshFromStorage(ac.refreshInterval)
}

// refreshFromStorage retrieves data from storage and replaces the current cache
func (ac *autoRefreshCache) refreshFromStorage(refreshInterval time.Duration) {
	ticker := time.NewTicker(refreshInterval)
	for {
		select {
		case <-ticker.C:
			ac.logger.Info("Refreshing cache from storage")
			if err := ac.warmCache(); err != nil {
				ac.logger.Error("Failed to retrieve cache from storage", zap.Error(err))
			}
		case <-ac.stopRefreshChan:
			return
		}
	}
}

// warmCache warm up the cache with data from storage
func (ac *autoRefreshCache) warmCache() error {
	services, err := ac.storage.GetServices(context.Background())
	if err != nil {
		return err
	}
	ac.put(services)
	return nil
}
