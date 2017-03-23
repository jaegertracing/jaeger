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
	"time"

	"github.com/uber/jaeger-lib/metrics"
)

// BoundedDropOldestQueue Implements a producer-consumer exchange similar to a ring buffer queue,
// where the queue is bounded and if it fills up due to slow consumers, the new items written by
// the producer force the earliest items to be dropped. The implementation is actually based on
// channels, with a special Reaper goroutine that wakes up when the queue is full and consumers
// the items from the top of the queue until its size drops back to maxSize
type BoundedDropOldestQueue struct {
	maxSize       int
	onDroppedItem func(item interface{})
	items         chan interface{}
	reaps         chan interface{}
}

// MakeBoundedDropOldestQueue constructs the new queue of specified max size, and with an optional
// callback for dropped items (e.g. useful to emit metrics)
func MakeBoundedDropOldestQueue(maxSize int, overflow int, onDroppedItem func(item interface{})) *BoundedDropOldestQueue {
	return &BoundedDropOldestQueue{
		maxSize:       maxSize,
		onDroppedItem: onDroppedItem,
		items:         make(chan interface{}, maxSize+overflow),
		reaps:         make(chan interface{}, overflow),
	}
}

// StartConsumers starts a given number of goroutines consuming items from the queue and passing them into
// the consumer callback. Also starts the Reaper goroutine.
func (q *BoundedDropOldestQueue) StartConsumers(num int, consumer func(item interface{})) {
	for i := 0; i < num; i++ {
		go func() {
			for item := range q.items {
				consumer(item)
			}
		}()
	}
	// start reaper
	go func() {
		for {
			select {
			case <-q.reaps:
				for q.Size() > q.maxSize {
					item := <-q.items
					if q.onDroppedItem != nil {
						q.onDroppedItem(item)
					}
				}
			}
		}
	}()
}

// Produce is used by the producer to submit new item to the queue. Returns false in case of queue overflow.
func (q *BoundedDropOldestQueue) Produce(item interface{}) bool {
	q.items <- item
	if len(q.items) > q.maxSize {
		q.reaps <- item
		return false
	}
	return true
}

// Size returns the current size of the queue
func (q *BoundedDropOldestQueue) Size() int {
	return len(q.items)
}

// MaxSize returns the current size of the queue
func (q *BoundedDropOldestQueue) MaxSize() int {
	return q.maxSize
}

// StartLengthReporting starts a timer-based gorouting that periodically reports current queue length to
// a given metrics gauge.
func (q *BoundedDropOldestQueue) StartLengthReporting(reportPeriod time.Duration, gauge metrics.Gauge) {
	ticker := time.NewTicker(reportPeriod)
	go func() {
		for {
			select {
			case <-ticker.C:
				gauge.Update(int64(q.Size()))
			}
		}
	}()
}
