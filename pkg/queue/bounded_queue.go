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
	uatomic "go.uber.org/atomic"
)

// BoundedQueue implements a producer-consumer exchange similar to a ring buffer queue,
// where the queue is bounded and if it fills up due to slow consumers, the new items written by
// the producer force the earliest items to be dropped. The implementation is actually based on
// channels, with a special Reaper goroutine that wakes up when the queue is full and consumers
// the items from the top of the queue until its size drops back to maxSize
type BoundedQueue struct {
	workers       int
	stopWG        sync.WaitGroup
	size          *uatomic.Uint32
	capacity      *uatomic.Uint32
	stopped       *uatomic.Uint32
	items         *chan interface{}
	onDroppedItem func(item interface{})
	consumer      func(item interface{})
	stopCh        chan struct{}
}

// NewBoundedQueue constructs the new queue of specified capacity, and with an optional
// callback for dropped items (e.g. useful to emit metrics).
func NewBoundedQueue(capacity int, onDroppedItem func(item interface{})) *BoundedQueue {
	queue := make(chan interface{}, capacity)
	return &BoundedQueue{
		onDroppedItem: onDroppedItem,
		items:         &queue,
		stopCh:        make(chan struct{}),
		capacity:      uatomic.NewUint32(uint32(capacity)),
		stopped:       uatomic.NewUint32(0),
		size:          uatomic.NewUint32(0),
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
		go func() {
			startWG.Done()
			defer q.stopWG.Done()
			queue := *q.items
			for {
				select {
				case item, ok := <-queue:
					if ok {
						q.size.Sub(1)
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
		}()
	}
	startWG.Wait()
}

// Produce is used by the producer to submit new item to the queue. Returns false in case of queue overflow.
func (q *BoundedQueue) Produce(item interface{}) bool {
	if q.stopped.Load() != 0 {
		q.onDroppedItem(item)
		return false
	}

	// we might have two concurrent backing queues at the moment
	// their combined size is stored in q.size, and their combined capacity
	// should match the capacity of the new queue
	if q.Size() >= q.Capacity() {
		// note that all items will be dropped if the capacity is 0
		q.onDroppedItem(item)
		return false
	}

	q.size.Add(1)
	select {
	case *q.items <- item:
		return true
	default:
		// should not happen, as overflows should have been captured earlier
		q.size.Sub(1)
		if q.onDroppedItem != nil {
			q.onDroppedItem(item)
		}
		return false
	}
}

// Stop stops all consumers, as well as the length reporter if started,
// and releases the items channel. It blocks until all consumers have stopped.
func (q *BoundedQueue) Stop() {
	q.stopped.Store(1) // disable producer
	close(q.stopCh)
	q.stopWG.Wait()
	close(*q.items)
}

// Size returns the current size of the queue
func (q *BoundedQueue) Size() int {
	return int(q.size.Load())
}

// Capacity returns capacity of the queue
func (q *BoundedQueue) Capacity() int {
	return int(q.capacity.Load())
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
	if capacity == q.Capacity() {
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

		// update the capacity
		q.capacity.Store(uint32(capacity))
	}

	return swapped
}
