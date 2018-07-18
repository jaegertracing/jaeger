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
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func insert(list *ConcurrentList, offsets ...int64) {
	for _, offset := range offsets {
		list.insert(offset)
	}
}

func TestInsert(t *testing.T) {
	for _, testCase := range generatePermutations([]int64{1, 2, 3}) {
		min, toInsert := extractMin(testCase)
		s := newConcurrentList(min)
		insert(s, toInsert...)
		assert.ElementsMatch(t, testCase, s.offsets)
	}
}

func TestGetHighestAndReset(t *testing.T) {
	testCases := []struct {
		input          []int64
		expectedOffset int64
		expectedList   []int64
	}{
		{
			input:          []int64{1},
			expectedOffset: 1,
			expectedList:   []int64{1},
		},
		{
			input:          []int64{1, 20},
			expectedOffset: 1,
			expectedList:   []int64{1, 20},
		},
		{
			input:          []int64{1, 2},
			expectedOffset: 2,
			expectedList:   []int64{2},
		},
		{
			input:          []int64{4, 5, 6},
			expectedOffset: 6,
			expectedList:   []int64{6},
		},
		{
			input:          []int64{1, 2, 4, 5},
			expectedOffset: 2,
			expectedList:   []int64{2, 4, 5},
		},
	}

	for _, testCase := range testCases {
		for _, input := range generatePermutations(testCase.input) {
			t.Run(fmt.Sprintf("%v", input), func(t *testing.T) {
				min, input := extractMin(input)
				s := newConcurrentList(min)
				insert(s, input...)
				actualOffset := s.setToHighestContiguous()
				assert.ElementsMatch(t, testCase.expectedList, s.offsets)
				assert.Equal(t, testCase.expectedOffset, actualOffset)
			})
		}
	}
}

func TestMultipleInsertsAndResets(t *testing.T) {
	l := newConcurrentList(100)

	for i := 101; i < 200; i++ {
		l.insert(int64(i))
	}
	l.insert(50)

	assert.Equal(t, 101, len(l.offsets))
	assert.Equal(t, int64(50), l.offsets[100])

	r := l.setToHighestContiguous()
	assert.Equal(t, int64(50), r)
	assert.Equal(t, 101, len(l.offsets))

	for i := 51; i < 99; i++ {
		l.insert(int64(i))
	}

	r = l.setToHighestContiguous()
	assert.Equal(t, int64(98), r)
	assert.Equal(t, 101, len(l.offsets))
}

// Heaps algorithm as per https://stackoverflow.com/questions/30226438/generate-all-permutations-in-go
func generatePermutations(arr []int64) [][]int64 {
	var helper func([]int64, int)
	res := [][]int64{}

	helper = func(arr []int64, n int) {
		if n == 1 {
			tmp := make([]int64, len(arr))
			copy(tmp, arr)
			res = append(res, tmp)
		} else {
			for i := 0; i < n; i++ {
				helper(arr, n-1)
				if n%2 == 1 {
					tmp := arr[i]
					arr[i] = arr[n-1]
					arr[n-1] = tmp
				} else {
					tmp := arr[0]
					arr[0] = arr[n-1]
					arr[n-1] = tmp
				}
			}
		}
	}
	helper(arr, len(arr))
	return res
}

func extractMin(arr []int64) (int64, []int64) {
	minIdx := 0
	for i := range arr {
		if arr[minIdx] > arr[i] {
			minIdx = i
		}
	}
	var toRet []int64
	toRet = append(toRet, arr[:minIdx]...)
	toRet = append(toRet, arr[minIdx+1:]...)

	return arr[minIdx], toRet
}

// BenchmarkInserts-8   	100000000	        70.6 ns/op	      49 B/op	       0 allocs/op
func BenchmarkInserts(b *testing.B) {
	l := newConcurrentList(0)
	for i := 1; i < b.N; i++ {
		l.insert(int64(i))
	}
}

// BenchmarkReset-8   	   10000	   1006342 ns/op	 1302421 B/op	      64 allocs/op
func BenchmarkResetTwice(b *testing.B) {
	var toInsert []int64
	for i := int(10e7); i < b.N+int(10e7); i++ {
		toInsert = append(toInsert, int64(i))
	}

	l := newConcurrentList(toInsert[0])

	// Create a gap
	toInsert[b.N/2] = 0

	for i := 0; i < b.N; i++ {
		n := i + rand.Intn(b.N-i)
		toInsert[i], toInsert[n] = toInsert[n], toInsert[i]
	}

	for i := 0; i < b.N; i++ {
		l.insert(toInsert[i])
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.setToHighestContiguous()
	}

	b.StopTimer()
	l.offsets = l.offsets[1:]
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		l.setToHighestContiguous()
	}
}
