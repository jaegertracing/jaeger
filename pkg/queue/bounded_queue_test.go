// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package queue

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

// In this test we run a queue with capacity 1 and a single consumer.
// We want to test the overflow behavior, so we block the consumer
// by holding a startLock before submitting items to the queue.
func helper(t *testing.T, startConsumers func(q *BoundedQueue[string], consumerFn func(item string))) {
	mFact := metricstest.NewFactory(0)
	counter := mFact.Counter(metrics.Options{Name: "dropped", Tags: nil})
	gauge := mFact.Gauge(metrics.Options{Name: "size", Tags: nil})

	q := NewBoundedQueue[string](1, func( /* item */ string) {
		counter.Inc(1)
	})
	assert.Equal(t, 1, q.Capacity())

	var startLock sync.Mutex

	startLock.Lock() // block consumers
	consumerState := newConsumerState(t)

	startConsumers(q, func(item string) {
		consumerState.record(item)

		// block further processing until startLock is released
		startLock.Lock()
		//nolint:staticcheck // empty section is ok
		startLock.Unlock()
	})

	assert.True(t, q.Produce("a"))

	// at this point "a" may or may not have been received by the consumer go-routine
	// so let's make sure it has been
	consumerState.waitToConsumeOnce()

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
		if g["size"] != 0 {
			break
		}
		time.Sleep(time.Millisecond)
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

func TestBoundedQueue(t *testing.T) {
	helper(t, func(q *BoundedQueue[string], consumerFn func(item string)) {
		q.StartConsumers(1, consumerFn)
	})
}

func TestBoundedQueueWithFactory(t *testing.T) {
	helper(t, func(q *BoundedQueue[string], consumerFn func(item string)) {
		q.StartConsumersWithFactory(1, func() Consumer[string] { return ConsumerFunc[string](consumerFn) })
	})
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

func (s *consumerState) waitToConsumeOnce() {
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

func TestResizeUp(t *testing.T) {
	q := NewBoundedQueue(2, func(item string) {
		fmt.Printf("dropped: %v\n", item)
	})

	var firstConsumer, secondConsumer, releaseConsumers sync.WaitGroup
	firstConsumer.Add(1)
	secondConsumer.Add(1)
	releaseConsumers.Add(1)

	released, resized := false, false
	q.StartConsumers(1, func( /* item */ string) {
		if !resized { // we'll have a second consumer once the queue is resized
			// signal that the worker is processing
			firstConsumer.Done()
			// once we release the lock, we might end up with multiple calls to reach this
		} else if !released {
			secondConsumer.Done()
		}
		// wait until we are signaled that we can finish
		releaseConsumers.Wait()
	})
	defer q.Stop()

	assert.True(t, q.Produce("a")) // in process
	firstConsumer.Wait()

	assert.True(t, q.Produce("b"))  // in queue
	assert.True(t, q.Produce("c"))  // in queue
	assert.False(t, q.Produce("d")) // dropped
	assert.EqualValues(t, 2, q.Capacity())
	assert.EqualValues(t, q.Capacity(), q.Size())
	assert.Len(t, *q.items.Load(), q.Capacity())

	resized = true
	assert.True(t, q.Resize(4))
	assert.True(t, q.Produce("e")) // in process by the second consumer
	secondConsumer.Wait()

	assert.True(t, q.Produce("f"))  // in the new queue
	assert.True(t, q.Produce("g"))  // in the new queue
	assert.False(t, q.Produce("h")) // the new queue has the capacity, but the sum of queues doesn't

	assert.EqualValues(t, 4, q.Capacity())
	assert.EqualValues(t, q.Capacity(), q.Size()) // the combined queues are at the capacity right now
	assert.Len(t, *q.items.Load(), 2)             // the new internal queue should have two items only

	released = true
	releaseConsumers.Done()
}

func TestResizeDown(t *testing.T) {
	q := NewBoundedQueue(4, func(item string) {
		fmt.Printf("dropped: %v\n", item)
	})

	var consumer, releaseConsumers sync.WaitGroup
	consumer.Add(1)
	releaseConsumers.Add(1)

	released := false
	q.StartConsumers(1, func( /* item */ string) {
		// once we release the lock, we might end up with multiple calls to reach this
		if !released {
			// signal that the worker is processing
			consumer.Done()
		}

		// wait until we are signaled that we can finish
		releaseConsumers.Wait()
	})
	defer q.Stop()

	assert.True(t, q.Produce("a")) // in process
	consumer.Wait()

	assert.True(t, q.Produce("b")) // in queue
	assert.True(t, q.Produce("c")) // in queue
	assert.True(t, q.Produce("d")) // in queue
	assert.True(t, q.Produce("e")) // dropped
	assert.EqualValues(t, 4, q.Capacity())
	assert.EqualValues(t, q.Capacity(), q.Size())
	assert.Len(t, *q.items.Load(), q.Capacity())

	assert.True(t, q.Resize(2))
	assert.False(t, q.Produce("f")) // dropped

	assert.EqualValues(t, 2, q.Capacity())
	assert.EqualValues(t, 4, q.Size()) // the queue will eventually drain, but it will live for a while over capacity
	assert.Empty(t, *q.items.Load())   // the new queue is empty, as the old queue is still full and over capacity

	released = true
	releaseConsumers.Done()
}

func TestResizeOldQueueIsDrained(t *testing.T) {
	q := NewBoundedQueue(2, func(item any) {
		fmt.Printf("dropped: %v\n", item)
	})

	var consumerReady, expected, readyToConsume sync.WaitGroup
	consumerReady.Add(1)
	readyToConsume.Add(1)
	expected.Add(5) // we expect 5 items to be processed

	var consumed atomic.Int32
	consumed.Store(5)

	first := true
	q.StartConsumers(1, func( /* item */ any) {
		// first run only
		if first {
			first = false
			consumerReady.Done()
		}

		readyToConsume.Wait()

		if consumed.Add(-1) >= 0 {
			// we mark only the first 5 items as done
			// we *might* get one item more in the queue given the right conditions
			// but this small difference is OK -- making sure we are processing *exactly* N items
			// is costlier than just accept that there's a couple more items in the queue than expected
			expected.Done()
		}
	})
	defer q.Stop()

	assert.True(t, q.Produce("a"))
	consumerReady.Wait()

	assert.True(t, q.Produce("b"))
	assert.True(t, q.Produce("c"))
	assert.False(t, q.Produce("d"))

	q.Resize(4)

	assert.True(t, q.Produce("e"))
	assert.True(t, q.Produce("f"))

	readyToConsume.Done()
	expected.Wait() // once this returns, we've consumed all items, meaning that both queues are drained
}

func TestNoopResize(t *testing.T) {
	q := NewBoundedQueue(2, func( /* item */ any) {})

	assert.False(t, q.Resize(2))
}

func TestZeroSize(t *testing.T) {
	q := NewBoundedQueue(0, func( /* item */ any) {})

	q.StartConsumers(1, func( /* item */ any) {})
	defer q.Stop()

	assert.False(t, q.Produce("a")) // in process
}

func BenchmarkBoundedQueue(b *testing.B) {
	q := NewBoundedQueue(1000, func( /* item */ any) {})
	q.StartConsumers(10, func( /* item */ any) {})
	defer q.Stop()

	for n := 0; n < b.N; n++ {
		q.Produce(n)
	}
}

func BenchmarkBoundedQueueWithFactory(b *testing.B) {
	q := NewBoundedQueue(1000, func( /* item */ int) {})

	q.StartConsumersWithFactory(10, func() Consumer[int] {
		return ConsumerFunc[int](func( /* item */ int) {})
	})
	defer q.Stop()

	for n := 0; n < b.N; n++ {
		q.Produce(n)
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
