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

func TestBoundedQueueWithPriority(t *testing.T) {
	mFact := metrics.NewLocalFactory(0)
	counter := mFact.Counter("dropped", nil)

	q := NewBoundedQueue(
		1,
		func(item interface{}) {
			counter.Inc(1)
		},
		GetPriority(func(item interface{}) int {
			if item.(string) == "a" {
				return highPriority
			} else {
				return lowPriority
			}
		}),
	)
	assert.Equal(t, 1, q.Capacity())

	consumerState := newConsumerState(t)

	q.StartConsumers(1, func(item interface{}) {
		consumerState.record(item.(string))
	})

	assert.True(t, q.Produce("b"))
	assert.True(t, q.Produce("a"))

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
