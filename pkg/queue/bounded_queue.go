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
	"time"

	"github.com/uber/jaeger-lib/metrics"
)

// BoundedQueue implements a producer-consumer exchange similar to a ring buffer queue,
// where the queue is bounded and if it fills up due to slow consumers, the new items written by
// the producer force the earliest items to be dropped. The implementation is actually based on
// channels, with a special Reaper goroutine that wakes up when the queue is full and consumers
// the items from the top of the queue until its size drops back to maxSize
type BoundedQueue struct {
	capacity      int
	size          int32
	onDroppedItem func(item interface{})
	items         chan interface{}
	stopCh        chan struct{}
	stopWG        sync.WaitGroup
	stopped       int32
}

// NewBoundedQueue constructs the new queue of specified capacity, and with an optional
// callback for dropped items (e.g. useful to emit metrics).
func NewBoundedQueue(capacity int, onDroppedItem func(item interface{})) *BoundedQueue {
	return &BoundedQueue{
		capacity:      capacity,
		onDroppedItem: onDroppedItem,
		items:         make(chan interface{}, capacity),
		stopCh:        make(chan struct{}),
	}
}

// StartConsumers starts a given number of goroutines consuming items from the queue
// and passing them into the consumer callback.
func (q *BoundedQueue) StartConsumers(num int, consumer func(item interface{})) {
	var startWG sync.WaitGroup
	for i := 0; i < num; i++ {
		q.stopWG.Add(1)
		startWG.Add(1)
		go func() {
			startWG.Done()
			defer q.stopWG.Done()
			for {
				select {
				case item := <-q.items:
					atomic.AddInt32(&q.size, -1)
					consumer(item)
				case <-q.stopCh:
					return
				}
			}
		}()
	}
	startWG.Wait()
}

// Produce is used by the producer to submit new item to the queue. Returns false in case of queue overflow.
func (q *BoundedQueue) Produce(item interface{}) bool {
	if atomic.LoadInt32(&q.stopped) != 0 {
		q.onDroppedItem(item)
		return false
	}
	select {
	case q.items <- item:
		atomic.AddInt32(&q.size, 1)
		return true
	default:
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
	close(q.items)
}

// Size returns the current size of the queue
func (q *BoundedQueue) Size() int {
	return int(atomic.LoadInt32(&q.size))
}

// Capacity returns capacity of the queue
func (q *BoundedQueue) Capacity() int {
	return q.capacity
}

// StartLengthReporting starts a timer-based gorouting that periodically reports
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
