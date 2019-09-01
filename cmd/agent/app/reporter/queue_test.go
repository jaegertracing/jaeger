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

package reporter

import (
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics/metricstest"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/queue"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

var _ Queue = &queue.NonQueue{}
var _ Queue = &queue.Bound{}

type gRPCErrorReporter struct {
	createRetryError bool
	createFatalError bool

	retries   int32
	processed int32
	errors    int32

	testMutex sync.Mutex
}

func (g *gRPCErrorReporter) EmitZipkinBatch(spans []*zipkincore.Span) error {
	atomic.AddInt32(&g.processed, 1)
	return nil
}

func (g *gRPCErrorReporter) EmitBatch(batch *jaeger.Batch) error {
	return g.ForwardBatch(model.Batch{})
}

func (g *gRPCErrorReporter) ForwardBatch(batch model.Batch) error {
	g.testMutex.Lock()
	defer g.testMutex.Unlock()

	if g.createRetryError {
		atomic.AddInt32(&g.retries, 1)
		return &retryableError{fmt.Errorf("Error requested")}
	} else if g.createFatalError {
		atomic.AddInt32(&g.errors, 1)
		return fmt.Errorf("Fatal error")
	}
	atomic.AddInt32(&g.processed, 1)
	return nil
}

func TestDefaultOptions(t *testing.T) {
	assert := assert.New(t)
	gr := &gRPCErrorReporter{}
	metricsFactory := metricstest.NewFactory(time.Microsecond)
	q, err := WrapWithQueue(&Options{}, gr, zap.NewNop(), metricsFactory)
	assert.NoError(err)

	err = q.EmitBatch(&jaeger.Batch{})
	assert.NoError(err)

	for i := 0; i < 100 && atomic.LoadInt32(&gr.processed) == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(int32(1), atomic.LoadInt32(&gr.processed))

	c, _ := metricsFactory.Snapshot()
	assert.Equal(int64(1), c["reporter.batches.submitted|format=jaeger"])
}

func TestMemoryQueueZipkin(t *testing.T) {
	assert := assert.New(t)
	gr := &gRPCErrorReporter{}
	metricsFactory := metricstest.NewFactory(time.Microsecond)
	q, err := WrapWithQueue(&Options{QueueType: MEMORY, BoundedQueueSize: defaultBoundedQueueSize, ReporterConcurrency: 1}, gr, zap.NewNop(), metricsFactory)
	assert.NoError(err)

	err = q.EmitZipkinBatch([]*zipkincore.Span{{}})
	assert.NoError(err)
	assert.Equal(int32(1), atomic.LoadInt32(&gr.processed))

	_, g := metricsFactory.Snapshot()
	assert.Equal(int64(1), g["reporter.batch_size|format=zipkin"])
}

func TestMemoryQueueSuccess(t *testing.T) {
	assert := assert.New(t)
	gr := &gRPCErrorReporter{}
	metricsFactory := metricstest.NewFactory(time.Microsecond)
	q, err := WrapWithQueue(&Options{QueueType: MEMORY, BoundedQueueSize: defaultBoundedQueueSize, ReporterConcurrency: 1}, gr, zap.NewNop(), metricsFactory)
	assert.NoError(err)

	err = q.EmitBatch(&jaeger.Batch{})
	assert.NoError(err)

	for i := 0; i < 100 && atomic.LoadInt32(&gr.processed) == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(int32(1), atomic.LoadInt32(&gr.processed))

	c, _ := metricsFactory.Snapshot()
	assert.Equal(int64(1), c["reporter.batches.submitted|format=jaeger"])
}

func TestMemoryQueueFail(t *testing.T) {
	assert := assert.New(t)
	gr := &gRPCErrorReporter{createFatalError: true}
	metricsFactory := metricstest.NewFactory(time.Microsecond)
	q, err := WrapWithQueue(&Options{QueueType: MEMORY, BoundedQueueSize: defaultBoundedQueueSize, ReporterConcurrency: 1}, gr, zap.NewNop(), metricsFactory)
	assert.NoError(err)

	err = q.EmitBatch(&jaeger.Batch{})
	assert.NoError(err)

	for i := 0; i < 100 && atomic.LoadInt32(&gr.errors) == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(int32(1), atomic.LoadInt32(&gr.errors))
	assert.Equal(int32(0), atomic.LoadInt32(&gr.retries))

	c, _ := metricsFactory.Snapshot()
	assert.Equal(int64(1), c["reporter.batches.failures|format=jaeger"])

	err = q.Close()
	assert.NoError(err)
}

func TestMemoryQueueRetries(t *testing.T) {
	assert := assert.New(t)
	gr := &gRPCErrorReporter{}
	metricsFactory := metricstest.NewFactory(time.Microsecond)
	q, err := WrapWithQueue(&Options{QueueType: MEMORY, BoundedQueueSize: defaultBoundedQueueSize, ReporterConcurrency: 1}, gr, zap.NewNop(), metricsFactory)
	assert.NoError(err)

	gr.testMutex.Lock()
	gr.createRetryError = true
	gr.testMutex.Unlock()

	err = q.EmitBatch(&jaeger.Batch{})
	assert.NoError(err)

	for i := 0; i < 100 && atomic.LoadInt32(&gr.retries) == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}

	assert.True(atomic.LoadInt32(&gr.retries) > 0)

	// Now verify it is resent
	gr.testMutex.Lock()
	gr.createRetryError = false
	gr.testMutex.Unlock()

	for i := 0; i < 100 && atomic.LoadInt32(&gr.processed) == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(int32(1), atomic.LoadInt32(&gr.processed))

	c, _ := metricsFactory.Snapshot()
	assert.True(c["reporter.batches.retries|format=jaeger"] > int64(0))
}

func TestBackoffTimer(t *testing.T) {
	assert := assert.New(t)

	metricsFactory := metricstest.NewFactory(time.Microsecond)
	q, err := WrapWithQueue(&Options{QueueType: MEMORY, ReporterMaxRetryInterval: time.Duration(time.Second)}, nil, zap.NewNop(), metricsFactory)
	assert.NoError(err)

	dur := q.backOffTimer()
	assert.True(q.initialRetryInterval == dur)

	q.lastRetryIntervalChange = q.lastRetryIntervalChange.Add(-1 * time.Hour)

	dur2 := q.backOffTimer()
	assert.True(dur2 > dur)

	// Reach the maximum time
	for i := 0; i < 100; i++ {
		q.lastRetryIntervalChange = q.lastRetryIntervalChange.Add(-1 * time.Hour)
		assert.True(q.maxRetryInterval >= q.backOffTimer())
	}
	assert.Equal(q.maxRetryInterval, q.currentRetryInterval)

	// Check metrics also
	_, g := metricsFactory.Snapshot()
	assert.Equal(int64(q.currentRetryInterval), g["reporter.retry-interval-ns|format=jaeger"])
}

func TestBadgerPersistence(t *testing.T) {
	dir, _ := ioutil.TempDir("", "badger")
	bo := &queue.BadgerOptions{
		Directory: dir,
	}
	defer os.RemoveAll(dir)

	assert := assert.New(t)
	gr := &gRPCErrorReporter{}
	metricsFactory := metricstest.NewFactory(time.Microsecond)
	q, err := WrapWithQueue(&Options{QueueType: BADGER, ReporterConcurrency: 1, BadgerOptions: bo}, gr, zap.NewNop(), metricsFactory)
	assert.NoError(err)

	gr.testMutex.Lock()
	gr.createRetryError = true
	gr.testMutex.Unlock()

	err = q.EmitBatch(&jaeger.Batch{})
	assert.NoError(err)

	for i := 0; i < 100 && atomic.LoadInt32(&gr.retries) == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}

	assert.True(atomic.LoadInt32(&gr.retries) > 0)

	err = q.Close()
	assert.NoError(err)

	// Now verify it is resent after restart
	gr2 := &gRPCErrorReporter{}
	metricsFactory2 := metricstest.NewFactory(time.Microsecond)
	q2, err := WrapWithQueue(&Options{QueueType: BADGER, ReporterConcurrency: 1, BadgerOptions: bo}, gr2, zap.NewNop(), metricsFactory2)
	assert.NoError(err)

	for i := 0; i < 300 && atomic.LoadInt32(&gr2.processed) == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(int32(1), atomic.LoadInt32(&gr2.processed))

	c, _ := metricsFactory2.Snapshot()
	assert.True(c["reporter.batches.submitted|format=jaeger"] > int64(0))

	err = q2.Close()
	assert.NoError(err)
}

func TestIsRetryable(t *testing.T) {
	assert := assert.New(t)
	err := fmt.Errorf("NoInterface")
	assert.False(IsRetryable(err))

	rerr := &retryableError{err}
	assert.True(IsRetryable(rerr))
}

func TestClose(t *testing.T) {
	assert := assert.New(t)
	gr := &gRPCErrorReporter{createRetryError: true}
	metricsFactory := metricstest.NewFactory(time.Microsecond)
	q, err := WrapWithQueue(&Options{QueueType: MEMORY, BoundedQueueSize: defaultBoundedQueueSize, ReporterConcurrency: 1}, gr, zap.NewNop(), metricsFactory)
	assert.NoError(err)

	err = q.EmitBatch(&jaeger.Batch{})
	assert.NoError(err)

	// There should be one inflight transaction
	err = q.Close()
	assert.NoError(err)
}

// gRPCReporterError is capsulated error coming from the gRPC interface
type retryableError struct {
	Err error
}

func (r *retryableError) Error() string {
	return r.Err.Error()
}

// IsRetryable checks if the gRPC errors are temporary errors and are errors from the status package
func (r *retryableError) IsRetryable() bool {
	return true
}
