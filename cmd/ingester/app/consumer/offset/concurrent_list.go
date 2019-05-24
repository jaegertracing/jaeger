// Copyright (c) 2018 The Jaeger Authors.
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

package offset

import (
	"errors"
	"sync"
)

var errExceededSize = errors.New("list full")

// ConcurrentList is a list that maintains kafka offsets with thread-safe Insert and setToHighestContiguous operations
type ConcurrentList struct {
	offsets []int64
	size    int
	mutex   sync.Mutex
}

func newConcurrentList(minOffset int64, size int) *ConcurrentList {
	return &ConcurrentList{offsets: []int64{minOffset}, size: size}
}

// Insert into the list in O(1) time.
// This operation is thread-safe
func (s *ConcurrentList) insert(offset int64) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.offsets = append(s.offsets, offset)
	if len(s.offsets) > s.size {
		return errExceededSize
	}
	return nil
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
		if _, ok := offsetSet[highestContiguous+1]; ok {
			highestContiguous++
		} else {
			break
		}
	}

	return highestContiguous
}
