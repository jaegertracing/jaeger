// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package cache

import (
	"math/rand"
	"reflect"
	"sync"
	"time"

	"go.uber.org/zap"
)

// autoRefreshCache cache that automatically refreshes itself
type autoRefreshCache struct {
	sync.RWMutex

	cache               map[string]string
	extSource           ServiceAliasMappingExternalSource
	storage             ServiceAliasMappingStorage
	logger              *zap.Logger
	readRefreshInterval time.Duration
	saveRefreshInterval time.Duration
	stopSaveChan        chan struct{}
	stopRefreshChan     chan struct{}
}

// NewAutoRefreshCache returns an autoRefreshCache
func NewAutoRefreshCache(
	extSource ServiceAliasMappingExternalSource,
	storage ServiceAliasMappingStorage,
	logger *zap.Logger,
	readRefreshInterval, saveRefreshInterval time.Duration,
) Cache {
	return &autoRefreshCache{
		cache:               make(map[string]string, 0),
		extSource:           extSource,
		storage:             storage,
		logger:              logger,
		readRefreshInterval: readRefreshInterval,
		saveRefreshInterval: saveRefreshInterval,
		stopSaveChan:        make(chan struct{}),
		stopRefreshChan:     make(chan struct{}),
	}
}

// Get given a key, return the corresponding value if it exists, else return an empty string
func (c *autoRefreshCache) Get(key string) string {
	c.RLock()
	defer c.RUnlock()
	return c.cache[key]
}

// Put implementation that does nothing
func (c *autoRefreshCache) Put(key string, value string) error {
	return nil
}

// IsEmpty returns true if the cache is empty, false otherwise
func (c *autoRefreshCache) IsEmpty() bool {
	c.RLock()
	defer c.RUnlock()
	return len(c.cache) == 0
}

// Initialize warm the cache and start up the auto cache refreshes
func (c *autoRefreshCache) Initialize() error {
	if err := c.warmCache(); err != nil {
		c.logger.Error("Cannot warm cache from storage or external source", zap.Error(err))
		// TODO (wjang) was this intentional? should this not return an error here?
	}
	c.initializeCacheRefresh()
	return nil
}

// StopRefresh stop refreshing the cache
func (c *autoRefreshCache) StopRefresh() {
	close(c.stopSaveChan)
	close(c.stopRefreshChan)
}

func (c *autoRefreshCache) initializeCacheRefresh() {
	go c.refreshFromStorage(c.readRefreshInterval)
	go c.refreshFromExternalSource(c.saveRefreshInterval)
}

// refreshFromExternalSource regularly retrieves data from an external source and saves it to storage
func (c *autoRefreshCache) refreshFromExternalSource(refreshInterval time.Duration) {
	time.Sleep(getRandomSleepTime(refreshInterval))
	c.updateAndSaveToStorage()
	ticker := time.NewTicker(refreshInterval)
	for {
		select {
		case <-ticker.C:
			c.updateAndSaveToStorage()
		case <-c.stopSaveChan:
			ticker.Stop()
			return
		}
	}
}

// updateAndSaveToStorage retrieves data from an external source and saves it to storage
func (c *autoRefreshCache) updateAndSaveToStorage() {
	cache, err := c.extSource.Load()
	if err != nil {
		c.logger.Error("Failed to retrieve cache from external source", zap.Error(err))
		return
	}
	// Get read lock so that the cache isn't modified while the cache is dumped to storage
	c.RLock()
	defer c.RUnlock()
	eq := mapEqual(c.cache, cache)
	if !eq {
		c.logger.Info("Dumping cache to storage")
		c.storage.Save(cache)
	}
}

func mapEqual(map1, map2 map[string]string) bool {
	return reflect.DeepEqual(map1, map2)
}

func getRandomSleepTime(interval time.Duration) time.Duration {
	return (interval / 2) + time.Duration(rand.Int63n(int64(interval/2)))
}

// refreshFromStorage retrieves data from storage and replaces cache as one of the other collector instances might have
// changed the content in storage
func (c *autoRefreshCache) refreshFromStorage(refreshInterval time.Duration) {
	ticker := time.NewTicker(refreshInterval)
	for {
		select {
		case <-ticker.C:
			c.logger.Info("Refreshing cache from storage")
			if cache, err := c.storage.Load(); err == nil {
				c.swapCache(cache)
			} else {
				c.logger.Error("Failed to retrieve cache from storage", zap.Error(err))
			}
		case <-c.stopRefreshChan:
			return
		}
	}
}

// warmCache warm up the cache with data from either storage (or an external source if the previous fails)
func (c *autoRefreshCache) warmCache() error {
	cache, err := c.storage.Load()
	if err != nil {
		cache, err = c.extSource.Load()
		if err != nil {
			return err
		}
	}
	c.swapCache(cache)
	return nil
}

func (c *autoRefreshCache) swapCache(cache map[string]string) {
	c.Lock()
	c.cache = cache
	c.Unlock()
}
