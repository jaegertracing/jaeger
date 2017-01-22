package cache

import (
	"container/list"
	"sync"
	"time"
)

// LRU is a concurrent fixed size cache that evicts elements in LRU order as well as by TTL.
type LRU struct {
	mux      sync.Mutex
	byAccess *list.List
	byKey    map[string]*list.Element
	maxSize  int
	ttl      time.Duration
	TimeNow  func() time.Time
	onEvict  EvictCallback
}

// NewLRU creates a new LRU cache with default options.
func NewLRU(maxSize int) *LRU {
	return NewLRUWithOptions(maxSize, nil)
}

// NewLRUWithOptions creates a new LRU cache with the given options.
func NewLRUWithOptions(maxSize int, opts *Options) *LRU {
	if opts == nil {
		opts = &Options{}
	}
	if opts.TimeNow == nil {
		opts.TimeNow = time.Now
	}
	return &LRU{
		byAccess: list.New(),
		byKey:    make(map[string]*list.Element, opts.InitialCapacity),
		ttl:      opts.TTL,
		maxSize:  maxSize,
		TimeNow:  opts.TimeNow,
		onEvict:  opts.OnEvict,
	}
}

// Get retrieves the value stored under the given key
func (c *LRU) Get(key string) interface{} {
	c.mux.Lock()
	defer c.mux.Unlock()

	elt := c.byKey[key]
	if elt == nil {
		return nil
	}

	cacheEntry := elt.Value.(*cacheEntry)
	if !cacheEntry.expiration.IsZero() && c.TimeNow().After(cacheEntry.expiration) {
		// Entry has expired
		if c.onEvict != nil {
			c.onEvict(cacheEntry.key, cacheEntry.value)
		}
		c.byAccess.Remove(elt)
		delete(c.byKey, cacheEntry.key)
		return nil
	}

	c.byAccess.MoveToFront(elt)
	return cacheEntry.value
}

// Put puts a new value associated with a given key, returning the existing value (if present)
func (c *LRU) Put(key string, value interface{}) interface{} {
	c.mux.Lock()
	defer c.mux.Unlock()
	elt := c.byKey[key]
	return c.putWithMutexHold(key, value, elt)
}

// CompareAndSwap puts a new value associated with a given key if existing value matches oldValue.
// It returns itemInCache as the element in cache after the function is executed and replaced as true if value is replaced, false otherwise.
func (c *LRU) CompareAndSwap(key string, oldValue, newValue interface{}) (itemInCache interface{}, replaced bool) {
	c.mux.Lock()
	defer c.mux.Unlock()

	elt := c.byKey[key]
	// If entry not found, old value should be nil
	if elt == nil && oldValue != nil {
		return nil, false
	}

	if elt != nil {
		// Entry found, compare it with that you expect.
		entry := elt.Value.(*cacheEntry)
		if entry.value != oldValue {
			return entry.value, false
		}
	}
	c.putWithMutexHold(key, newValue, elt)
	return newValue, true
}

// putWithMutexHold populates the cache and returns the inserted value.
// Caller is expected to hold the c.mut mutex before calling.
func (c *LRU) putWithMutexHold(key string, value interface{}, elt *list.Element) interface{} {
	if elt != nil {
		entry := elt.Value.(*cacheEntry)
		existing := entry.value
		entry.value = value
		if c.ttl != 0 {
			entry.expiration = c.TimeNow().Add(c.ttl)
		}
		c.byAccess.MoveToFront(elt)
		return existing
	}

	entry := &cacheEntry{
		key:   key,
		value: value,
	}

	if c.ttl != 0 {
		entry.expiration = c.TimeNow().Add(c.ttl)
	}
	c.byKey[key] = c.byAccess.PushFront(entry)
	for len(c.byKey) > c.maxSize {
		oldest := c.byAccess.Remove(c.byAccess.Back()).(*cacheEntry)
		if c.onEvict != nil {
			c.onEvict(oldest.key, oldest.value)
		}
		delete(c.byKey, oldest.key)
	}

	return nil
}

// Delete deletes a key, value pair associated with a key
func (c *LRU) Delete(key string) {
	c.mux.Lock()
	defer c.mux.Unlock()

	elt := c.byKey[key]
	if elt != nil {
		entry := c.byAccess.Remove(elt).(*cacheEntry)
		if c.onEvict != nil {
			c.onEvict(entry.key, entry.value)
		}
		delete(c.byKey, key)
	}
}

// Size returns the number of entries currently in the lru, useful if cache is not full
func (c *LRU) Size() int {
	c.mux.Lock()
	defer c.mux.Unlock()

	return len(c.byKey)
}

type cacheEntry struct {
	key        string
	expiration time.Time
	value      interface{}
}
