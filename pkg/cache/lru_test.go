// Copyright (c) 2017 Uber Technologies, Inc.
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

package cache

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLRU(t *testing.T) {
	cache := NewLRUWithOptions(4, &Options{
		OnEvict: func(k string, i interface{}) {
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
	cache := NewLRU(2)

	item, ok := cache.CompareAndSwap("A", nil, "Foo")
	assert.Equal(t, true, ok)
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
	assert.True(t, ok)
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

func TestLRUCacheConcurrentAccess(t *testing.T) {
	cache := NewLRU(5)
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
	for i := 0; i < 20; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			for i := 0; i < 1000; i++ {
				cache.Get("A")
			}
		}()
	}

	close(start)
	wg.Wait()
}

func TestRemoveFunc(t *testing.T) {
	ch := make(chan bool)
	cache := NewLRUWithOptions(5, &Options{
		OnEvict: func(k string, i interface{}) {
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
		OnEvict: func(k string, i interface{}) {
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
