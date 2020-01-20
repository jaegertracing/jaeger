package queue

import (
	"sync"
	"time"

	"github.com/uber/jaeger-lib/metrics"
)

type Queue interface {
	Enqueue(item interface{}) bool
	Dequeue() (interface{}, bool)
	Empty() bool
}

// ConcurrentQueue implements a producer-consumer pattern.  It starts multiple goroutines for consumers
// and manages the synchronization of the underlying basic Queue.
// When the underlying queue cannot fit any more items due to capacity, the earliest items are removed
// and passed to the onDroppedItem callback.
type ConcurrentQueue struct {
	consumeFn     func(item interface{})
	onDroppedItem func(item interface{})
	queue         Queue
	cond          *sync.Cond
	close         bool // protected by cond.L
}

func NewConcurrentQueue(queue Queue, onDroppedItem func(item interface{})) *ConcurrentQueue {
	return &ConcurrentQueue{
		queue:         queue,
		cond:          sync.NewCond(&sync.Mutex{}),
		onDroppedItem: onDroppedItem,
	}
}

func (q *ConcurrentQueue) Produce(item interface{}) bool {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	for {
		ok := q.queue.Enqueue(item)
		if ok {
			break
		}
		item, ok := q.queue.Dequeue()
		if !ok {
			// item appears too large to fit even in an empty queue
			// TODO: do we want to call onDroppedItem for these?
			return false
		}
		q.onDroppedItem(item)
	}
	q.cond.Broadcast()
	return true
}

func (q *ConcurrentQueue) StartConsumers(workers int, consumeFn func(item interface{})) {
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

func (q *ConcurrentQueue) Close() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()
	if !q.close {
		q.close = true
		q.cond.Broadcast()
	}
}

func (q *ConcurrentQueue) StartLengthReporting(
	reportPeriod time.Duration,
	queueLength metrics.Gauge,
	queueMemSize metrics.Gauge,
) {

}
