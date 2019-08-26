// Copyright (c) 2019 The Jaeger Authors.
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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics/metricstest"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
)

func TestErrorConsumerLogging(t *testing.T) {
	// For codecov
	metricsFactory := metricstest.NewFactory(time.Microsecond)
	b := NewBoundQueue(1, 1, func(batch *jaeger.Batch) error {
		return fmt.Errorf("Logging error")
	}, zap.NewNop(), metricsFactory)

	for i := 0; i < 2; i++ {
		b.Enqueue(&jaeger.Batch{
			Process: &jaeger.Process{
				ServiceName: fmt.Sprintf("error_%d", i),
			},
		})
	}
}

func TestDroppedItems(t *testing.T) {
	assert := assert.New(t)

	mut := sync.Mutex{}
	mut.Lock()
	wg := sync.WaitGroup{}
	wg.Add(2)
	processNames := make([]string, 0, 2)

	metricsFactory := metricstest.NewFactory(time.Microsecond)
	b := NewBoundQueue(1, 1, func(batch *jaeger.Batch) error {
		fmt.Printf("%s\n", batch.GetProcess().ServiceName)
		mut.Lock() // Block processing until we let it go
		processNames = append(processNames, batch.GetProcess().ServiceName)
		mut.Unlock()
		wg.Done()
		return nil
	}, zap.NewNop(), metricsFactory)

	for i := 0; i < 2; i++ {
		// First one goes to processing, second to queue..
		assert.NoError(b.Enqueue(&jaeger.Batch{
			Process: &jaeger.Process{
				ServiceName: fmt.Sprintf("success_%d", i),
			},
		}))
	}

	// These should start throwing errors as the queue is full
	for i := 0; i < 2; i++ {
		assert.Error(b.Enqueue(&jaeger.Batch{
			Process: &jaeger.Process{
				ServiceName: fmt.Sprintf("error_%d", i),
			},
		}))
	}

	c, _ := metricsFactory.Snapshot()
	assert.Equal(int64(2), c["reporter.batches.dropped"])

	mut.Unlock()
	wg.Wait()
	assert.Equal(2, len(processNames))
	assert.Equal("success_0", processNames[0])
	assert.Equal("success_1", processNames[1])
}
