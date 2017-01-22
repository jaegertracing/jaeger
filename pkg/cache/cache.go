package cache

import "time"

// A Cache is a generalized interface to a cache.  See cache.LRU for a specific
// implementation (bounded cache with LRU eviction)
type Cache interface {
	// Get retrieves an element based on a key, returning nil if the element
	// does not exist
	Get(key string) interface{}

	// Put adds an element to the cache, returning the previous element
	Put(key string, value interface{}) interface{}

	// Delete deletes an element in the cache
	Delete(key string)

	// Size returns the number of entries currently stored in the Cache
	Size() int

	// CompareAndSwap adds an element to the cache if the existing entry matches the old value.
	// It returns the element in cache after function is executed and true if the element was replaced, false otherwise.
	CompareAndSwap(key string, old, new interface{}) (interface{}, bool)
}

// Options control the behavior of the cache
type Options struct {
	// TTL controls the time-to-live for a given cache entry.  Cache entries that
	// are older than the TTL will not be returned
	TTL time.Duration

	// InitialCapacity controls the initial capacity of the cache
	InitialCapacity int

	// OnEvict is an optional function called when an element is evicted.
	OnEvict EvictCallback

	// TimeNow is used to override the behavior of default time.Now(), e.g. in tests.
	TimeNow func() time.Time
}

// EvictCallback is a type for notifying applications when an item is
// scheduled for eviction from the Cache.
type EvictCallback func(key string, value interface{})
