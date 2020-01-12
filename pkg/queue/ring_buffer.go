// Copyright (c) 2020 The Jaeger Authors.
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

type RingBufferQueue struct {
	size, head, tail int
	buffer           []interface{}
}

func NewRingBufferQueue(size int) *RingBufferQueue {
	return &RingBufferQueue{
		buffer: make([]interface{}, size),
		size:   size,
		head:   -1,
		tail:   -1,
	}
}

func (q *RingBufferQueue) Capacity() int {
	return q.size
}

func (q *RingBufferQueue) Empty() bool {
	return q.head == -1
}

func (q *RingBufferQueue) Full() bool {
	return (q.tail+1)%q.size == q.head
}

func (q *RingBufferQueue) Enqueue(item interface{}) bool {
	if q.Full() {
		return false
	}
	if q.head == -1 {
		// empty queue
		q.head, q.tail = 0, 0
		q.buffer[0] = item
	} else {
		q.tail = (q.tail + 1) % q.size
		q.buffer[q.tail] = item
	}
	return true
}

func (q *RingBufferQueue) Dequeue() (interface{}, bool) {
	if q.Empty() {
		return nil, false
	}
	var item interface{}
	if q.head == q.tail {
		// single element
		item = q.buffer[q.tail]
		q.head, q.tail = -1, -1
	} else {
		item = q.buffer[q.head]
		q.head = (q.head + 1) % q.size
	}
	return item, true
}

func (q *RingBufferQueue) EnsureCapacity(cap int) {
	if q.size >= cap {
		return
	}
	buffer := make([]interface{}, cap)
	if q.head <= q.tail {
		// not wrapped
		length := q.tail - q.head
		copy(buffer, q.buffer[q.head:q.tail+1])
		q.head = 0
		q.tail = length
	} else {
		// wrapped
		copy(buffer, q.buffer[q.head:])
		top := q.size - q.head
		copy(buffer[top:], q.buffer[0:q.tail+1])
		q.head = 0
		q.tail += top
	}
	q.buffer = buffer
	q.size = cap
}
