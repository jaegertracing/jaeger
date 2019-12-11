// Copyright (c) 2019 The Jaeger Authors.
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
	"time"
	"unsafe"

	"github.com/uber/jaeger-lib/metrics"
)

// BoundedQueue implements a producer-consumer exchange similar to a ring buffer queue,
// where the queue is bounded and if it fills up due to slow consumers, the new items written by
// the producer force the earliest items to be dropped. The implementation is actually based on
// channels, with a special Reaper goroutine that wakes up when the queue is full and consumers
// the items from the top of the queue until its size drops back to maxSize
type BoundedQueue struct {
	size          int32
	onDroppedItem func(item interface{})
	items         *chan interface{}
	stopCh        chan struct{}
	stopWG        sync.WaitGroup
	stopped       int32
	consumer      func(item interface{})
	workers       int
}

// NewBoundedQueue constructs the new queue of specified capacity, and with an optional
// callback for dropped items (e.g. useful to emit metrics).
func NewBoundedQueue(capacity int, onDroppedItem func(item interface{})) *BoundedQueue {
	queue := make(chan interface{}, capacity)
	return &BoundedQueue{
		onDroppedItem: onDroppedItem,
		items:         &queue,
		stopCh:        make(chan struct{}),
	}
}

// StartConsumers starts a given number of goroutines consuming items from the queue
// and passing them into the consumer callback.
func (q *BoundedQueue) StartConsumers(num int, consumer func(item interface{})) {
	q.workers = num
	q.consumer = consumer
	var startWG sync.WaitGroup
	for i := 0; i < q.workers; i++ {
		q.stopWG.Add(1)
		startWG.Add(1)
		go func(queue chan interface{}) {
			startWG.Done()
			defer q.stopWG.Done()
			for {
				select {
				case item, ok := <-queue:
					if ok {
						atomic.AddInt32(&q.size, -1)
						q.consumer(item)
					} else {
						// channel closed, finish worker
						return
					}
				case <-q.stopCh:
					// the whole queue is closing, finish worker
					return
				}
			}
		}(*q.items)
	}
	startWG.Wait()
}

// Produce is used by the producer to submit new item to the queue. Returns false in case of queue overflow.
func (q *BoundedQueue) Produce(item interface{}) bool {
	if atomic.LoadInt32(&q.stopped) != 0 {
		q.onDroppedItem(item)
		return false
	}

	// we might have two concurrent backing queues at the moment
	// their combined size is stored in q.size, and their combined capacity
	// should match the capacity of the new queue
	if atomic.LoadInt32(&q.size) >= int32(q.Capacity()) && q.Capacity() > 0 {
		// current consumers of the queue (like tests) expect the queue capacity = 0 to work
		// so, we don't drop items when the capacity is 0
		q.onDroppedItem(item)
		return false
	}

	select {
	case *q.items <- item:
		atomic.AddInt32(&q.size, 1)
		return true
	default:
		// should not happen, as overflows should have been captured earlier
		if q.onDroppedItem != nil {
			q.onDroppedItem(item)
		}
		return false
	}
}

// Stop stops all consumers, as well as the length reporter if started,
// and releases the items channel. It blocks until all consumers have stopped.
func (q *BoundedQueue) Stop() {
	atomic.StoreInt32(&q.stopped, 1) // disable producer
	close(q.stopCh)
	q.stopWG.Wait()
	close(*q.items)
}

// Size returns the current size of the queue
func (q *BoundedQueue) Size() int {
	return int(atomic.LoadInt32(&q.size))
}

// Capacity returns capacity of the queue
func (q *BoundedQueue) Capacity() int {
	return cap(*q.items)
}

// StartLengthReporting starts a timer-based goroutine that periodically reports
// current queue length to a given metrics gauge.
func (q *BoundedQueue) StartLengthReporting(reportPeriod time.Duration, gauge metrics.Gauge) {
	ticker := time.NewTicker(reportPeriod)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				size := q.Size()
				gauge.Update(int64(size))
			case <-q.stopCh:
				return
			}
		}
	}()
}

// Resize changes the capacity of the queue, returning whether the action was successful
func (q *BoundedQueue) Resize(capacity int) bool {
	if capacity == cap(*q.items) {
		// noop
		return false
	}

	previous := *q.items
	queue := make(chan interface{}, capacity)

	// swap queues
	// #nosec
	swapped := atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.items)), unsafe.Pointer(q.items), unsafe.Pointer(&queue))
	if swapped {
		// start a new set of consumers, based on the information given previously
		q.StartConsumers(q.workers, q.consumer)

		// gracefully drain the existing queue
		close(previous)
	}

	return swapped
}
