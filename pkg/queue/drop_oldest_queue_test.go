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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQueue(t *testing.T) {
	var droppedItem interface{}
	var reapDone sync.WaitGroup
	var reapExit sync.WaitGroup
	var wg sync.WaitGroup
	var startLock sync.Mutex

	// Make sure that at least one consumer consumes and element before we produce them
	// all. If all 5 go out before an consumers, we'll have 1 reaped, 1 in reaper channel,
	// and 2 in the items channel, and we'll deadlock.
	consumerConsumed := sync.NewCond(&sync.Mutex{})
	consumerConsumed.L.Lock()

	wg.Add(4)       // will be waiting for 4 items from consumers
	reapDone.Add(1) // block main until reaper is done
	reapExit.Add(1) // block reaper before exit

	startLock.Lock() // block consumers

	q := MakeBoundedDropOldestQueue(2, 1, func(item interface{}) {
		droppedItem = item
		t.Logf("dropped %+v\n", item)
		reapDone.Done()
		// if reaper is too quick, it can reap more than 1 item, and we want 4 of them to go to consumers, so block it
		reapExit.Wait()
	})

	q.StartConsumers(2, func(item interface{}) {
		t.Logf("received item %+v\n", item)
		consumerConsumed.Broadcast()
		startLock.Lock()
		wg.Done()
		startLock.Unlock()
		t.Logf("consumed %+v\n", item)
	})
	produce := func(val string) {
		q.Produce(val) // goes to consumer 1 and blocks it on startLock
		t.Logf("produced %s, queue size %d\n", val, q.Size())
	}
	// With queue capacity of 3, and two consumers, we can product at most 5 items without blocking,
	// assuming that both consumers read one value and block
	// If they don't read, then main will block while producing 5 items, thus allowing consumers to read
	// and block.
	// In any case main will block either on produce or on reapWait
	for _, s := range [...]string{"one", "two"} {
		produce(s)
	}
	consumerConsumed.Wait()
	for _, s := range [...]string{"three", "four", "five"} {
		produce(s)
	}

	// wait for reaper to guarantee it reaps exactly one item
	reapDone.Wait()
	// depending on the order, any one of one/two/three could have been reaped
	assert.True(t, "one" == droppedItem || "two" == droppedItem || "three" == droppedItem)
	// unblock consumers (reaper is blocked on exit lock)
	startLock.Unlock()
	// wait for consumers to finish - passing this lock is a test that they consumed exactly 4 items
	wg.Wait()
	assert.Equal(t, 0, q.Size())
	// unblock the reaper, but it has nothing to do
	reapExit.Done()
}
