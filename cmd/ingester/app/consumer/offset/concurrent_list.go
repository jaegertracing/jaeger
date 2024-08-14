// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package offset

import (
	"sync"
)

// ConcurrentList is a list that maintains kafka offsets with thread-safe Insert and setToHighestContiguous operations
type ConcurrentList struct {
	offsets []int64
	mutex   sync.Mutex
}

func newConcurrentList(minOffset int64) *ConcurrentList {
	return &ConcurrentList{offsets: []int64{minOffset}}
}

// Insert into the list in O(1) time.
// This operation is thread-safe
func (s *ConcurrentList) insert(offset int64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.offsets = append(s.offsets, offset)
}

// setToHighestContiguous sets head to highestContiguous and returns the message and status.
// This is a O(n) operation.
// highestContiguous is defined as the highest sequential integer encountered while traversing from the head of the
// list.
// For e.g., if the list is [1, 2, 3, 5], the highestContiguous is 3.
// This operation is thread-safe
func (s *ConcurrentList) setToHighestContiguous() int64 {
	s.mutex.Lock()
	offsets := s.offsets
	s.offsets = nil
	s.mutex.Unlock()

	highestContiguousOffset := getHighestContiguous(offsets)

	var higherOffsets []int64
	for _, offset := range offsets {
		if offset >= highestContiguousOffset {
			higherOffsets = append(higherOffsets, offset)
		}
	}

	s.mutex.Lock()
	s.offsets = append(s.offsets, higherOffsets...)
	s.mutex.Unlock()
	return highestContiguousOffset
}

func getHighestContiguous(offsets []int64) int64 {
	offsetSet := make(map[int64]struct{}, len(offsets))
	minOffset := offsets[0]

	for _, offset := range offsets {
		offsetSet[offset] = struct{}{}
		if minOffset > offset {
			minOffset = offset
		}
	}

	highestContiguous := minOffset
	for {
		if _, ok := offsetSet[highestContiguous+1]; !ok {
			break
		}
		highestContiguous++
	}

	return highestContiguous
}
