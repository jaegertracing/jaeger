package queue

// MemoryBoundedQueue implements a Queue on top of a circular buffer,
// with an upper bound on the amount of memory used by all enqueued items,
// calculated by the means of the sizeFn function.
//
// The ring buffer may be dynamically expanded from the initial capacity.
//
// Not safe to use from multiple goroutines.
type MemoryBoundedQueue struct {
	sizeFn      func(item interface{}) uint64
	memCapacity uint64
	memAmount   uint64 // actual amount of memory used
	queue       *RingBufferQueue
}

func NewMemoryBoundedQueue(initSize int, memCapacity uint64, sizeFn func(item interface{}) uint64) *MemoryBoundedQueue {
	if initSize <= 0 || memCapacity <= 0 {
		return nil
	}
	return &MemoryBoundedQueue{
		queue:       NewRingBufferQueue(initSize),
		memCapacity: memCapacity,
		sizeFn:      sizeFn,
	}
}

func (q *MemoryBoundedQueue) Enqueue(item interface{}) bool {
	size := q.sizeFn(item)
	if q.memAmount+size > q.memCapacity {
		return false
	}
	// we're good with memory, but may need to make the queue longer
	if q.queue.Full() {
		q.queue.EnsureCapacity(2 * q.queue.Capacity())
	}
	ok := q.queue.Enqueue(item)
	if ok {
		q.memAmount += size
	}
	return ok
}

func (q *MemoryBoundedQueue) Dequeue() (interface{}, bool) {
	if q.queue.Empty() {
		return nil, false
	}
	item, ok := q.queue.Dequeue()
	if ok {
		q.memAmount -= q.sizeFn(item)
	}
	return item, ok
}

func (q *MemoryBoundedQueue) Empty() bool {
	return q.queue.Empty()
}
