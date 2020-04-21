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

package pool

import (
	"sync"
	"sync/atomic"
	"testing"
)

var (
	n       int32 = 0
	wg      sync.WaitGroup
	rwMutex sync.RWMutex
)

func testJob() {
	rwMutex.RLock()
	defer rwMutex.RUnlock()
	atomic.AddInt32(&n, 1)
	wg.Done()
}

func TestExecute(t *testing.T) {
	tests := []struct {
		name        string
		numOfWorker int
		numOfJob    int
		expected    int32
	}{
		//num of worker == num of job
		{
			name:        "worker: 1, job: 1",
			numOfWorker: 1,
			numOfJob:    1,
			expected:    1,
		},
		{
			name:        "worker: 2, job: 2",
			numOfWorker: 2,
			numOfJob:    2,
			expected:    2,
		},
		//num of worker < num of job
		{
			name:        "worker: 1, job: 3",
			numOfWorker: 1,
			numOfJob:    3,
			expected:    3,
		},
		{
			name:        "worker: 2, job: 4",
			numOfWorker: 2,
			numOfJob:    4,
			expected:    4,
		},
		//num of worker > num of job
		{
			name:        "worker: 5, job: 4",
			numOfWorker: 5,
			numOfJob:    4,
			expected:    4,
		},
		{
			name:        "worker: 6, job: 2",
			numOfWorker: 6,
			numOfJob:    2,
			expected:    2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n = 0
			blockedJob := 0
			testPool := New(tt.numOfWorker)
			defer testPool.Stop()

			for i := 0; i < tt.numOfJob; i++ {
				if blockedJob == 0 {
					rwMutex.Lock()
				}
				testPool.Execute(testJob)
				blockedJob++
				if blockedJob == tt.numOfWorker {
					wg.Add(blockedJob)
					rwMutex.Unlock()
					wg.Wait()
					blockedJob = 0
				}
			}

			if blockedJob != 0 {
				wg.Add(blockedJob)
				rwMutex.Unlock()
				wg.Wait()
			}

			if n != tt.expected {
				t.Errorf("expect: %v, got: %v\n", tt.expected, n)
			}
		})
	}
}
