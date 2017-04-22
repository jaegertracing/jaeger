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

package queue

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics"
)

func TestBoundedQueue(t *testing.T) {
	mFact := metrics.NewLocalFactory(0)
	counter := mFact.Counter("dropped", nil)
	gauge := mFact.Gauge("size", nil)

	q := NewBoundedQueue(2, func(item interface{}) {
		counter.Inc(1)
	})
	assert.Equal(t, 2, q.Capacity())

	var startLock sync.Mutex
	var consumedLock sync.Mutex
	consumed := make(map[string]bool)

	getConsumed := func() map[string]bool {
		consumedLock.Lock()
		defer consumedLock.Unlock()
		out := make(map[string]bool)
		for k, v := range consumed {
			out[k] = v
		}
		return out
	}

	startLock.Lock() // block consumers

	q.StartConsumers(1, func(item interface{}) {
		// block until allowed to start
		startLock.Lock()
		startLock.Unlock()

		consumedLock.Lock()
		defer consumedLock.Unlock()
		consumed[item.(string)] = true
	})

	assert.True(t, q.Produce("a"))
	assert.Equal(t, 1, q.Size())
	assert.True(t, q.Produce("b"))
	assert.Equal(t, 2, q.Size())
	assert.False(t, q.Produce("c"))
	assert.Equal(t, 2, q.Size())

	q.StartLengthReporting(time.Millisecond, gauge)
	// as soon as we call sleep(), the consumer go-routing might should wake
	// up and decrement q.Size(), thus below we assert the guage value == 1.
	for i := 0; i < 1000; i++ {
		_, g := mFact.Snapshot()
		if g["size"] == 0 {
			time.Sleep(time.Millisecond)
		}
	}

	c, g := mFact.Snapshot()
	assert.EqualValues(t, 1, c["dropped"])
	assert.EqualValues(t, 1, g["size"])

	startLock.Unlock() // unblock consumers

	assertConsumed := func(expected map[string]bool) {
		for i := 0; i < 1000; i++ {
			if consumed := getConsumed(); len(consumed) < 2 {
				time.Sleep(time.Millisecond)
			}
		}
		assert.Equal(t, expected, getConsumed())
		assert.Equal(t, 0, q.Size())
	}
	assertConsumed(map[string]bool{
		"a": true,
		"b": true,
	})

	// now that consumers are unblocked, we can add more items
	assert.True(t, q.Produce("d"))
	assert.True(t, q.Produce("e"))
	assert.True(t, q.Produce("f"))
	assertConsumed(map[string]bool{
		"a": true,
		"b": true,
		"d": true,
		"e": true,
		"f": true,
	})

	q.Stop()
	assert.False(t, q.Produce("x"), "cannot push to closed queue")
}
