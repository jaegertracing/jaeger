package queue

import (
	"sync"
	"time"

	"github.com/uber/jaeger-lib/metrics"
)

type Sizeable interface {
	Size() int
}

type MemoryBoundedQueue struct {
	queue         *RingBufferQueue
	memSize       int
	memCapacity   int
	onDroppedItem func(item Sizeable)
}

func NewMemoryBoundedQueue(initLength int, memCapacity int, onDroppedItem func(item Sizeable)) *MemoryBoundedQueue {
	return &MemoryBoundedQueue{
		queue:         NewRingBufferQueue(initLength),
		memCapacity:   memCapacity,
		onDroppedItem: onDroppedItem,
	}
}

func (q *MemoryBoundedQueue) Enqueue(item Sizeable) bool {
	size := item.Size()
	for q.memSize+size > q.memCapacity {
		if q.queue.Empty() {
			return false // a single item is larger than max capacity
		}
		dropped, _ := q.Dequeue()
		q.onDroppedItem(dropped)
	}
	// now we're good with memory, but may need to make the queue longer
	if q.queue.Full() {
		q.queue.EnsureCapacity(2 * q.queue.Capacity())
	}
	_ = q.queue.Enqueue(item)
	q.memSize += size
	return true
}

func (q *MemoryBoundedQueue) Dequeue() (Sizeable, bool) {
	if q.queue.Empty() {
		return nil, false
	}
	i, _ := q.queue.Dequeue()
	item := i.(Sizeable)
	q.memSize -= item.Size()
	return item, true
}

func (q *MemoryBoundedQueue) Empty() bool {
	return q.queue.Empty()
}

type ConcurrentMemoryBoundedQueue struct {
	queue     *MemoryBoundedQueue
	consumeFn func(item Sizeable)
	cond      *sync.Cond
	close     bool // protected by cond.L
}

func NewConcurrentMemoryBoundedQueue(queue *MemoryBoundedQueue) *ConcurrentMemoryBoundedQueue {
	return &ConcurrentMemoryBoundedQueue{
		queue: queue,
		cond:  sync.NewCond(&sync.Mutex{}),
	}
}

func (q *ConcurrentMemoryBoundedQueue) Enqueue(item Sizeable) bool {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	ok := q.queue.Enqueue(item)
	if ok {
		q.cond.Broadcast()
	}
	return ok
}

func (q *ConcurrentMemoryBoundedQueue) StartConsumers(workers int, consumeFn func(item Sizeable)) {
	q.consumeFn = consumeFn
	var start sync.WaitGroup
	start.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			start.Done()
			for {
				q.cond.L.Lock()
				for q.queue.Empty() && !q.close {
					q.cond.Wait()
				}
				if q.close {
					q.cond.L.Unlock()
					return
				}
				item, ok := q.queue.Dequeue()
				q.cond.L.Unlock()
				if ok {
					q.consumeFn(item)
				}
			}
		}()
	}
	start.Wait()
}

func (q *ConcurrentMemoryBoundedQueue) Close() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	if !q.close {
		q.close = true
		q.cond.Broadcast()
	}
}

func (q *ConcurrentMemoryBoundedQueue) StartLengthReporting(
	reportPeriod time.Duration,
	queueLength metrics.Gauge,
	queueMemSize metrics.Gauge,
) {
	// TODO
}
