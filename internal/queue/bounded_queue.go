// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package queue

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/jaegertracing/jaeger/internal/metrics"
)

// Consumer consumes data from a bounded queue
type Consumer[T any] interface {
	Consume(item T)
}

// BoundedQueue implements a producer-consumer exchange similar to a ring buffer queue,
// where the queue is bounded and if it fills up due to slow consumers, the new items written by
// the producer force the earliest items to be dropped. The implementation is actually based on
// channels, with a special Reaper goroutine that wakes up when the queue is full and consumers
// the items from the top of the queue until its size drops back to maxSize
type BoundedQueue[T any] struct {
	workers       int
	stopWG        sync.WaitGroup
	size          atomic.Int32
	capacity      atomic.Uint32
	stopped       atomic.Uint32
	items         atomic.Pointer[chan T]
	onDroppedItem func(item T)
	factory       func() Consumer[T]
	stopCh        chan struct{}
}

// NewBoundedQueue constructs the new queue of specified capacity, and with an optional
// callback for dropped items (e.g. useful to emit metrics).
func NewBoundedQueue[T any](capacity int, onDroppedItem func(item T)) *BoundedQueue[T] {
	queue := make(chan T, capacity)
	bq := &BoundedQueue[T]{
		onDroppedItem: onDroppedItem,
		stopCh:        make(chan struct{}),
	}
	bq.items.Store(&queue)
	//nolint: gosec // G115
	bq.capacity.Store(uint32(capacity))
	return bq
}

// StartConsumersWithFactory creates a given number of consumers consuming items
// from the queue in separate goroutines.
func (q *BoundedQueue[T]) StartConsumersWithFactory(num int, factory func() Consumer[T]) {
	q.workers = num
	q.factory = factory
	var startWG sync.WaitGroup
	for i := 0; i < q.workers; i++ {
		q.stopWG.Add(1)
		startWG.Add(1)
		go func() {
			startWG.Done()
			defer q.stopWG.Done()
			consumer := q.factory()
			queue := q.items.Load()
			for {
				select {
				case item, ok := <-*queue:
					if ok {
						q.size.Add(-1)
						consumer.Consume(item)
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

// ConsumerFunc is an adapter to allow the use of
// a consume function callback as a Consumer.
type ConsumerFunc[T any] func(item T)

// Consume calls c(item)
func (c ConsumerFunc[T]) Consume(item T) {
	c(item)
}

// StartConsumers starts a given number of goroutines consuming items from the queue
// and passing them into the consumer callback.
func (q *BoundedQueue[T]) StartConsumers(num int, callback func(item T)) {
	q.StartConsumersWithFactory(num, func() Consumer[T] {
		return ConsumerFunc[T](callback)
	})
}

// Produce is used by the producer to submit new item to the queue. Returns false in case of queue overflow.
func (q *BoundedQueue[T]) Produce(item T) bool {
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
	queue := q.items.Load()
	select {
	case *queue <- item:
		return true
	default:
		// should not happen, as overflows should have been captured earlier
		q.size.Add(-1)
		if q.onDroppedItem != nil {
			q.onDroppedItem(item)
		}
		return false
	}
}

// Stop stops all consumers, as well as the length reporter if started,
// and releases the items channel. It blocks until all consumers have stopped.
func (q *BoundedQueue[T]) Stop() {
	q.stopped.Store(1) // disable producer
	close(q.stopCh)
	q.stopWG.Wait()
	close(*q.items.Load())
}

// Size returns the current size of the queue
func (q *BoundedQueue[T]) Size() int {
	return int(q.size.Load())
}

// Capacity returns capacity of the queue
func (q *BoundedQueue[T]) Capacity() int {
	return int(q.capacity.Load())
}

// StartLengthReporting starts a timer-based goroutine that periodically reports
// current queue length to a given metrics gauge.
func (q *BoundedQueue[T]) StartLengthReporting(reportPeriod time.Duration, gauge metrics.Gauge) {
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
func (q *BoundedQueue[T]) Resize(capacity int) bool {
	if capacity == q.Capacity() {
		// noop
		return false
	}

	previous := q.items.Load()
	queue := make(chan T, capacity)

	// swap queues
	swapped := q.items.CompareAndSwap(previous, &queue)
	if swapped {
		// start a new set of consumers, based on the information given previously
		q.StartConsumersWithFactory(q.workers, q.factory)

		// gracefully drain the existing queue
		close(*previous)

		// update the capacity
		//nolint: gosec // G115
		q.capacity.Store(uint32(capacity))
	}

	return swapped
}
