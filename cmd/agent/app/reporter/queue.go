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
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/common"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/queue"
	"github.com/jaegertracing/jaeger/model"
	jConverter "github.com/jaegertracing/jaeger/model/converter/thrift/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

const (
	defaultQueueType        = MEMORY
	defaultBoundedQueueSize = 1000
	defaultMaxRetryInterval = 20 * time.Second
	defaultQueueWorkers     = 8
)

// Queue is generic interface which includes methods common to all implemented queues
type Queue interface {
	Enqueue(model.Batch) error
	io.Closer
}

// QueuedReporter is a reporter that uses push-pull method that queues all incoming requests and then
// lets the wrapped reporter do the actual pushing to the server (such as gRPC). If the requests fails
// with retryable error the transaction is tried again.
type QueuedReporter struct {
	close uint32

	wrapped Forwarder
	queue   Queue
	logger  *zap.Logger

	retryMutex sync.Mutex

	lastRetryIntervalChange time.Time
	currentRetryInterval    time.Duration
	maxRetryInterval        time.Duration
	initialRetryInterval    time.Duration

	reporterMetrics *MetricsReporter
	agentTags       []model.KeyValue
}

// WrapWithQueue wraps the destination reporter with a queueing capabilities for retries
func WrapWithQueue(opts *Options, forwarder Forwarder, logger *zap.Logger, mFactory metrics.Factory) (*QueuedReporter, error) {
	q := &QueuedReporter{
		wrapped:                 forwarder,
		logger:                  logger,
		lastRetryIntervalChange: time.Now(),
		initialRetryInterval:    time.Millisecond * 100,
		maxRetryInterval:        opts.ReporterMaxRetryInterval,
		reporterMetrics:         NewMetricsReporter(forwarder, mFactory),
		agentTags:               model.KeyValueFromMap(opts.AgentTags),
		close:                   uint32(0),
	}
	q.currentRetryInterval = q.initialRetryInterval

	// Some sanity checks
	if opts.QueueType == "" {
		opts.QueueType = MEMORY
	}

	if opts.ReporterConcurrency < 1 {
		opts.ReporterConcurrency = 1
	}

	switch opts.QueueType {
	case MEMORY:
		q.queue = queue.NewBoundQueue(opts.BoundedQueueSize, opts.ReporterConcurrency, q.batchProcessor, logger, mFactory)
	case DIRECT:
		q.queue = queue.NewNonQueue(q.directProcessor)
	case BADGER:
		opts.BadgerOptions.Concurrency = opts.ReporterConcurrency
		queue, err := queue.NewBadgerQueue(opts.BadgerOptions, q.batchProcessor, logger)
		if err != nil {
			return nil, err
		}
		q.queue = queue
	}

	return q, nil
}

// EmitZipkinBatch forwards the spans to the wrapped reporter (without queue)
func (q *QueuedReporter) EmitZipkinBatch(spans []*zipkincore.Span) error {
	// EmitZipkinBatch does not use queue, instead it uses the older metrics passthrough
	// This method should not be called
	return q.reporterMetrics.EmitZipkinBatch(spans)
}

// EmitBatch sends the batch to the queue for async processing
func (q *QueuedReporter) EmitBatch(batch *jaeger.Batch) error {
	if batch != nil {
		spans := jConverter.ToDomain(batch.Spans, nil)
		process := jConverter.ToDomainProcess(batch.Process)
		spans, process = common.AddProcessTags(spans, process, q.agentTags)
		forwardBatch := model.Batch{Spans: spans, Process: process}
		err := q.queue.Enqueue(forwardBatch)
		return err
	}
	return nil
}

func (q *QueuedReporter) backOffTimer() time.Duration {
	// Has to be more than previous time from previous increase before we can reincrease
	// otherwise simultaneous threads could race to increase the sleep time too quickly
	t := time.Now()
	q.retryMutex.Lock()
	if q.lastRetryIntervalChange.Add(q.currentRetryInterval).Before(t) && q.currentRetryInterval < q.maxRetryInterval {
		// We have to do the recheck because someone could have changed the time between check and mutex locking
		newWait := q.currentRetryInterval * 2
		if newWait > q.maxRetryInterval {
			q.currentRetryInterval = q.maxRetryInterval
		} else {
			q.currentRetryInterval = newWait
		}
		q.lastRetryIntervalChange = time.Now()
		q.reporterMetrics.BatchMetrics.RetryInterval.Update(int64(q.currentRetryInterval))
	}
	q.retryMutex.Unlock()

	return q.currentRetryInterval
}

// batchProcessor retries the batch until it succeeds or gives up. Return value indicates if the
// processing was completed (a non retryable failure is considered complete). It's up to the
// queue implementation to process the processed flag.
func (q *QueuedReporter) batchProcessor(batch model.Batch) (bool, error) {
	spansCount := int64(len(batch.GetSpans()))

	err := q.wrapped.ForwardBatch(batch)
	if err != nil {
		for IsRetryable(err) {
			if q.closed() {
				return false, err
			}
			q.reporterMetrics.BatchMetrics.BatchesRetries.Inc(1)
			// Block this processing instance before returning
			sleepTime := q.backOffTimer()
			q.logger.Error(fmt.Sprintf("Failed to contact the collector, waiting %s before retry", sleepTime.String()))
			time.Sleep(sleepTime)
			err = q.wrapped.ForwardBatch(batch)
			if err == nil {
				q.retryMutex.Lock()
				q.currentRetryInterval = q.initialRetryInterval
				q.reporterMetrics.BatchMetrics.RetryInterval.Update(int64(q.currentRetryInterval))
				q.lastRetryIntervalChange = time.Now()
				q.retryMutex.Unlock()
				q.updateSuccessStats(spansCount)
				return true, nil
			}
		}
		q.reporterMetrics.BatchMetrics.BatchesFailures.Inc(1)
		q.reporterMetrics.BatchMetrics.SpansFailures.Inc(spansCount)
		q.logger.Error("Could not send batch", zap.Error(err))
		return true, err
	}
	q.updateSuccessStats(spansCount)
	return true, nil
}

func (q *QueuedReporter) closed() bool {
	return atomic.LoadUint32(&q.close) == 1
}

// directProcessor is only forwarding the item for processing and does not have
// any queueing capabilities.
func (q *QueuedReporter) directProcessor(batch model.Batch) (bool, error) {
	// No retries, report error directly back. Useful for testing to bypass the queue
	spansCount := int64(len(batch.GetSpans()))

	err := q.wrapped.ForwardBatch(batch)
	if err != nil {
		q.reporterMetrics.BatchMetrics.BatchesFailures.Inc(1)
		q.reporterMetrics.BatchMetrics.SpansFailures.Inc(spansCount)
		return true, err
	}
	q.updateSuccessStats(spansCount)
	return true, nil
}

func (q *QueuedReporter) updateSuccessStats(spansCount int64) {
	q.reporterMetrics.BatchMetrics.BatchesSubmitted.Inc(1)
	q.reporterMetrics.BatchMetrics.SpansSubmitted.Inc(spansCount)
	q.reporterMetrics.BatchMetrics.BatchSize.Update(spansCount)
}

// IsRetryable checks whether the error is implementing RetryableError and returns the value of error's IsRetryable
func IsRetryable(err error) bool {
	if r, ok := err.(RetryableError); ok {
		return r.IsRetryable()
	}
	return false
}

// Close implements io.Closer
func (q *QueuedReporter) Close() error {
	atomic.StoreUint32(&q.close, 1)
	return q.queue.Close()
}
