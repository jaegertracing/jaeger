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

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRingBufferQueue(t *testing.T) {
	tests := []struct {
		item  int  // positive to enqueue, negative to dequeue
		ok    bool // whether enqueue/dequeue should succeed
		empty bool
		full  bool
	}{
		{item: 1, ok: true, empty: false, full: false},
		{item: 2, ok: true, empty: false, full: false},
		{item: 3, ok: true, empty: false, full: true},
		{item: 4, ok: false, empty: false, full: true},
		{item: -1, ok: true, empty: false, full: false},
		{item: 4, ok: true, empty: false, full: true},
		{item: 5, ok: false, empty: false, full: true},
		{item: -2, ok: true, empty: false, full: false},
		{item: -3, ok: true, empty: false, full: false},
		{item: 5, ok: true, empty: false, full: false},
		{item: -4, ok: true, empty: false, full: false},
		{item: -5, ok: true, empty: true, full: false},
		{item: -6, ok: false, empty: true, full: false},
	}
	q := NewRingBufferQueue(3)
	assert.True(t, q.Empty())
	assert.True(t, q.Empty())
	for i, test := range tests {
		t.Run(fmt.Sprintf("iter-%d", i), func(t *testing.T) {
			if test.item > 0 {
				ok := q.Enqueue(test.item)
				assert.Equal(t, test.ok, ok, "enqueue result")
			} else {
				item, ok := q.Dequeue()
				assert.Equal(t, test.ok, ok, "dequeue result")
				if test.ok {
					assert.Equal(t, -test.item, item)
				}
			}
			assert.Equal(t, test.empty, q.Empty(), "Empty()")
			assert.Equal(t, test.full, q.Full(), "Full()")
		})
	}
}

func TestRingBufferQueue_EnsureCapacityNoop(t *testing.T) {
	q := NewRingBufferQueue(2)
	q.EnsureCapacity(1)
	assert.Equal(t, 2, q.size)
	assert.Len(t, q.buffer, 2)
	q.EnsureCapacity(2)
	assert.Equal(t, 2, q.size)
	assert.Len(t, q.buffer, 2)
}

func TestRingBufferQueue_EnsureCapacity(t *testing.T) {
	tests := []struct {
		items    []int // positive to enqueue, negative to dequeue
		newCap   int
		expCap   int
		expItems []int // content of the buffer after EnsureCapacity
	}{
		{items: []int{1, 2}, newCap: 4, expCap: 4, expItems: []int{1, 2, 0, 0}},
		{items: []int{1, 2, 3, -1}, newCap: 4, expCap: 4, expItems: []int{2, 3, 0, 0}},
		{items: []int{1, 2, 3, -1, -2, 4, 5}, newCap: 4, expCap: 4, expItems: []int{3, 4, 5, 0}},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("iter-%d", i), func(t *testing.T) {
			q := NewRingBufferQueue(3)
			for _, item := range test.items {
				if item > 0 {
					ok := q.Enqueue(item)
					assert.True(t, ok)
				} else {
					itm, ok := q.Dequeue()
					assert.True(t, ok)
					assert.Equal(t, -item, itm)
				}
			}
			q.EnsureCapacity(test.newCap)
			assert.Equal(t, test.expCap, q.size)
			assert.Len(t, q.buffer, test.expCap)
			// check buffer content
			for j, item := range test.expItems {
				if item != 0 {
					if !assert.Equal(t, item, q.buffer[j]) {
						t.Log(q.buffer)
					}
				}
			}
			// try to dequeue
			for _, item := range test.expItems {
				if item == 0 {
					assert.True(t, q.Empty())
					break
				}
				itm, ok := q.Dequeue()
				assert.True(t, ok)
				assert.Equal(t, item, itm)
			}
		})
	}
}
