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
	"sync/atomic"
	"testing"
	"time"

	"reflect"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
)

// In this test we run a queue with capacity 1 and a single consumer.
// We want to test the overflow behavior, so we block the consumer
// by holding a startLock before submitting items to the queue.
// However, because we only control the code in the consumer callback,
// the first item may or may not be already received from the queue.
// To ensure that it is received, we
func TestBoundedQueue(t *testing.T) {
	mFact := metrics.NewLocalFactory(0)
	counter := mFact.Counter("dropped", nil)
	gauge := mFact.Gauge("size", nil)

	q := NewBoundedQueue(1, func(item interface{}) {
		counter.Inc(1)
	})
	assert.Equal(t, 1, q.Capacity())

	var startLock sync.Mutex

	startLock.Lock() // block consumers
	consumerState := newConsumerState(t)

	q.StartConsumers(1, func(item interface{}) {
		consumerState.record(item.(string))

		// block further processing until startLock is released
		startLock.Lock()
		startLock.Unlock()
	})

	assert.True(t, q.Produce("a"))

	// at this point "a" may or may not have been received by the consumer go-routine
	// so let's make sure it has been
	consumerState.waitToConsumerOnce()

	// at this point the item must have been read off the queue, but the consumer is blocked
	assert.Equal(t, 0, q.Size())
	consumerState.assertConsumed(map[string]bool{
		"a": true,
	})

	// produce two more items. The first one should be accepted, but not consumed.
	assert.True(t, q.Produce("b"))
	assert.Equal(t, 1, q.Size())
	// the second should be rejected since the queue is full
	assert.False(t, q.Produce("c"))
	assert.Equal(t, 1, q.Size())

	q.StartLengthReporting(time.Millisecond, gauge)
	for i := 0; i < 1000; i++ {
		_, g := mFact.Snapshot()
		if g["size"] == 0 {
			time.Sleep(time.Millisecond)
		}
	}

	c, g := mFact.Snapshot()
	assert.EqualValues(t, 1, c["dropped"])
	assert.EqualValues(t, 1, g["size"])

	startLock.Unlock() // unblock consumer

	consumerState.assertConsumed(map[string]bool{
		"a": true,
		"b": true,
	})

	// now that consumers are unblocked, we can add more items
	expected := map[string]bool{
		"a": true,
		"b": true,
	}
	for _, item := range []string{"d", "e", "f"} {
		assert.True(t, q.Produce(item))
		expected[item] = true
		consumerState.assertConsumed(expected)
	}

	q.Stop()
	assert.False(t, q.Produce("x"), "cannot push to closed queue")
}

type consumerState struct {
	sync.Mutex
	t            *testing.T
	consumed     map[string]bool
	consumedOnce int32
}

func newConsumerState(t *testing.T) *consumerState {
	return &consumerState{
		t:        t,
		consumed: make(map[string]bool),
	}
}

func (s *consumerState) record(val string) {
	s.Lock()
	defer s.Unlock()
	s.consumed[val] = true
	atomic.StoreInt32(&s.consumedOnce, 1)
}

func (s *consumerState) snapshot() map[string]bool {
	s.Lock()
	defer s.Unlock()
	out := make(map[string]bool)
	for k, v := range s.consumed {
		out[k] = v
	}
	return out
}

func (s *consumerState) waitToConsumerOnce() {
	for i := 0; i < 1000; i++ {
		if atomic.LoadInt32(&s.consumedOnce) == 0 {
			time.Sleep(time.Millisecond)
		}
	}
	require.EqualValues(s.t, 1, atomic.LoadInt32(&s.consumedOnce), "expected to consumer once")
}

func (s *consumerState) assertConsumed(expected map[string]bool) {
	for i := 0; i < 1000; i++ {
		if snapshot := s.snapshot(); !reflect.DeepEqual(snapshot, expected) {
			time.Sleep(time.Millisecond)
		}
	}
	assert.Equal(s.t, expected, s.snapshot())
}
