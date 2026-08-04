// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestLRU(t *testing.T) {
	cache := NewLRUWithOptions(4, &Options{
		OnEvict: func(_ string, _ any) {
			// do nothing, just for code coverage
		},
	})

	cache.Put("A", "Foo")
	assert.Equal(t, "Foo", cache.Get("A"))
	assert.Nil(t, cache.Get("B"))
	assert.Equal(t, 1, cache.Size())

	cache.Put("B", "Bar")
	cache.Put("C", "Cid")
	cache.Put("D", "Delt")
	assert.Equal(t, 4, cache.Size())

	assert.Equal(t, "Bar", cache.Get("B"))
	assert.Equal(t, "Cid", cache.Get("C"))
	assert.Equal(t, "Delt", cache.Get("D"))

	cache.Put("A", "Foo2")
	assert.Equal(t, "Foo2", cache.Get("A"))

	cache.Put("E", "Epsi")
	assert.Equal(t, "Epsi", cache.Get("E"))
	assert.Equal(t, "Foo2", cache.Get("A"))
	assert.Nil(t, cache.Get("B")) // Oldest, should be evicted

	// Access C, D is now LRU
	cache.Get("C")
	cache.Put("F", "Felp")
	assert.Nil(t, cache.Get("D"))

	cache.Delete("A")
	assert.Nil(t, cache.Get("A"))
}

func TestCompareAndSwap(t *testing.T) {
	cache := NewLRUWithOptions(2, nil)

	item, ok := cache.CompareAndSwap("A", nil, "Foo")
	assert.True(t, ok)
	assert.Equal(t, "Foo", item)
	assert.Equal(t, "Foo", cache.Get("A"))
	assert.Nil(t, cache.Get("B"))
	assert.Equal(t, 1, cache.Size())

	item, ok = cache.CompareAndSwap("B", nil, "Bar")
	assert.True(t, ok)
	assert.Equal(t, 2, cache.Size())
	assert.Equal(t, "Bar", item)
	assert.Equal(t, "Bar", cache.Get("B"))

	item, ok = cache.CompareAndSwap("A", "Foo", "Foo2")
	assert.True(t, ok)
	assert.Equal(t, "Foo2", item)
	assert.Equal(t, "Foo2", cache.Get("A"))

	item, ok = cache.CompareAndSwap("A", nil, "Foo3")
	assert.False(t, ok)
	assert.Equal(t, "Foo2", item)
	assert.Equal(t, "Foo2", cache.Get("A"))

	item, ok = cache.CompareAndSwap("A", "Foo", "Foo3")
	assert.False(t, ok)
	assert.Equal(t, "Foo2", item)
	assert.Equal(t, "Foo2", cache.Get("A"))

	item, ok = cache.CompareAndSwap("F", "foo", "Foo3")
	assert.False(t, ok)
	assert.Nil(t, item)
	assert.Nil(t, cache.Get("F"))

	// Evict the oldest entry
	item, ok = cache.CompareAndSwap("E", nil, "Epsi")
	assert.True(t, ok)
	assert.Equal(t, "Epsi", item)
	assert.Equal(t, "Foo2", cache.Get("A"))
	assert.Nil(t, cache.Get("B")) // Oldest, should be evicted
}

func TestLRUWithTTL(t *testing.T) {
	clk := &simulatedClock{}
	cache := NewLRUWithOptions(5, &Options{
		TTL:     time.Millisecond * 100,
		TimeNow: clk.Now,
	})
	cache.Put("A", "Foo")
	assert.Equal(t, "Foo", cache.Get("A"))

	item, _ := cache.CompareAndSwap("A", "Foo", "Foo2")
	assert.Equal(t, "Foo2", item)
	assert.Equal(t, "Foo2", cache.Get("A"))

	clk.Elapse(time.Millisecond * 50)
	assert.Equal(t, "Foo2", cache.Get("A"))

	clk.Elapse(time.Millisecond * 100)
	assert.Nil(t, cache.Get("A"))
	assert.Equal(t, 0, cache.Size())
}

// TestCompareAndSwapWithExpiredEntry verifies that CompareAndSwap treats an
// expired TTL entry as absent, consistent with Get(). Before the fix,
// CompareAndSwap would find the stale map entry and compare without checking
// the expiration time, so CompareAndSwap(key, nil, newValue) would
// incorrectly fail even though Get(key) returned nil.
func TestCompareAndSwapWithExpiredEntry(t *testing.T) {
	evictions := 0
	clk := &simulatedClock{}
	c := NewLRUWithOptions(5, &Options{
		TTL:     time.Millisecond * 100,
		TimeNow: clk.Now,
		OnEvict: func(_ string, _ any) {
			evictions++
		},
	})

	// Put an entry and let it expire.
	c.Put("A", "Foo")
	assert.Equal(t, "Foo", c.Get("A"))
	evictions = 0 // reset: Get does not evict (not expired yet)

	clk.Elapse(time.Millisecond * 200) // well past TTL

	// Get should now report the key as absent.
	assert.Nil(t, c.Get("A"))
	assert.Equal(t, 1, evictions)
	evictions = 0

	// Re-insert so we can test CAS on an expired-but-still-in-map entry.
	c.Put("B", "Bar")
	clk.Elapse(time.Millisecond * 200) // expire "B" without an intervening Get

	// CompareAndSwap(key, nil, newValue) must succeed: the expired entry
	// should be treated as absent (oldValue == nil).
	item, ok := c.CompareAndSwap("B", nil, "Bar2")
	assert.True(t, ok, "CAS should succeed because expired entry is treated as absent")
	assert.Equal(t, "Bar2", item)
	assert.Equal(t, "Bar2", c.Get("B"))
	// The eviction callback must have fired exactly once for the expired entry.
	assert.Equal(t, 1, evictions)

	// A CAS with the wrong old value must still fail.
	item, ok = c.CompareAndSwap("B", "wrong", "Bar3")
	assert.False(t, ok)
	assert.Equal(t, "Bar2", item)
	assert.Equal(t, "Bar2", c.Get("B"))
}

func TestDefaultClock(t *testing.T) {
	cache := NewLRUWithOptions(5, &Options{
		TTL: time.Millisecond * 1,
	})
	cache.Put("A", "foo")
	assert.Equal(t, "foo", cache.Get("A"))
	time.Sleep(time.Millisecond * 3)
	assert.Nil(t, cache.Get("A"))
	assert.Equal(t, 0, cache.Size())
}

func TestLRUCacheConcurrentAccess(*testing.T) {
	cache := NewLRUWithOptions(5, nil)
	values := map[string]string{
		"A": "foo",
		"B": "bar",
		"C": "zed",
		"D": "dank",
		"E": "ezpz",
	}

	for k, v := range values {
		cache.Put(k, v)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			<-start

			for range 1000 {
				cache.Get("A")
			}
		})
	}

	close(start)
	wg.Wait()
}

func TestRemoveFunc(t *testing.T) {
	ch := make(chan bool)
	cache := NewLRUWithOptions(5, &Options{
		OnEvict: func(_ string, i any) {
			go func() {
				_, ok := i.(*testing.T)
				assert.True(t, ok)
				ch <- true
			}()
		},
	})

	cache.Put("testing", t)
	cache.Delete("testing")
	assert.Nil(t, cache.Get("testing"))

	timeout := time.NewTimer(time.Millisecond * 300)
	select {
	case b := <-ch:
		assert.True(t, b)
	case <-timeout.C:
		t.Error("RemovedFunc did not send true on channel ch")
	}
}

func TestRemovedFuncWithTTL(t *testing.T) {
	ch := make(chan bool)
	cache := NewLRUWithOptions(5, &Options{
		TTL: time.Millisecond * 5,
		OnEvict: func(_ string, i any) {
			go func() {
				_, ok := i.(*testing.T)
				assert.True(t, ok)
				ch <- true
			}()
		},
	})

	cache.Put("A", t)
	assert.Equal(t, t, cache.Get("A"))
	time.Sleep(time.Millisecond * 10)
	assert.Nil(t, cache.Get("A"))

	timeout := time.NewTimer(time.Millisecond * 30)
	select {
	case b := <-ch:
		assert.True(t, b)
	case <-timeout.C:
		t.Error("RemovedFunc did not send true on channel ch")
	}
}

type simulatedClock struct {
	sync.Mutex
	currTime time.Time
}

func (c *simulatedClock) Now() time.Time {
	c.Lock()
	defer c.Unlock()
	return c.currTime
}

func (c *simulatedClock) Elapse(d time.Duration) time.Time {
	c.Lock()
	defer c.Unlock()
	c.currTime = c.currTime.Add(d)
	return c.currTime
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
