package reporter

import (
	"fmt"
	"sync"
	"time"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/queue"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

const (
	defaultQueueType        = "memory"
	defaultBoundedQueueSize = 1000
	defaultMaxRetryInterval = 20 * time.Second
	defaultQueueWorkers     = 8
)

type Queue interface {
	Enqueue(*jaeger.Batch) error
}

type QueuedReporter struct {
	wrapped Reporter
	queue   Queue
	logger  *zap.Logger

	retryMutex sync.Mutex

	lastRetryIntervalChange time.Time
	currentRetryInterval    time.Duration
	maxRetryInterval        time.Duration
	initialRetryInterval    time.Duration

	reporterMetrics *MetricsReporter
}

// WrapWithQueue wraps the destination reporter with a queueing capabilities for retries
func WrapWithQueue(opts *Options, reporter Reporter, logger *zap.Logger, mFactory metrics.Factory) *QueuedReporter {
	q := &QueuedReporter{
		wrapped:                 reporter,
		logger:                  logger,
		lastRetryIntervalChange: time.Now(),
		initialRetryInterval:    time.Millisecond * 100,
		maxRetryInterval:        opts.ReporterMaxRetryInterval,
		reporterMetrics:         NewMetricsReporter(reporter, mFactory),
	}
	q.currentRetryInterval = q.initialRetryInterval

	switch opts.QueueType {
	case MEMORY:
		q.queue = queue.NewBoundQueue(opts.BoundedQueueSize, opts.ReporterConcurrency, q.batchProcessor, logger, mFactory)
	}

	return q
}

// EmitZipkinBatch forwards the spans to the wrapped reporter (without queue)
func (q *QueuedReporter) EmitZipkinBatch(spans []*zipkincore.Span) error {
	// EmitZipkinBatch does not use queue, instead it uses the older metrics passthrough
	return q.reporterMetrics.EmitZipkinBatch(spans)
}

// EmitBatch sends the batch to the queue for async processing
func (q *QueuedReporter) EmitBatch(batch *jaeger.Batch) error {
	spansCount := int64(len(batch.GetSpans()))
	q.reporterMetrics.BatchMetrics.BatchSize.Update(spansCount)
	return q.queue.Enqueue(batch)
}

func (q *QueuedReporter) backOffTimer() time.Duration {
	// Has to be more than previous time from previous increase before we can reincrease
	// otherwise simultaneous threads could race to increase the sleep time too quickly
	t := time.Now()
	if q.lastRetryIntervalChange.Add(q.currentRetryInterval).Before(t) && q.currentRetryInterval < q.maxRetryInterval {
		// We can increase, more than the previous timer has been spent
		q.retryMutex.Lock()
		if q.lastRetryIntervalChange.Add(q.currentRetryInterval).Before(t) {
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
	}

	return q.currentRetryInterval
}

func (q *QueuedReporter) batchProcessor(batch *jaeger.Batch) error {
	spansCount := int64(len(batch.GetSpans()))

	q.reporterMetrics.BatchMetrics.BatchesSubmitted.Inc(1)
	q.reporterMetrics.BatchMetrics.SpansSubmitted.Inc(spansCount)

	err := q.wrapped.EmitBatch(batch)
	if err != nil {
		for IsRetryable(err) {
			q.reporterMetrics.BatchMetrics.BatchesRetries.Inc(1)
			// Block this processing instance before returning
			sleepTime := q.backOffTimer()
			q.logger.Info(fmt.Sprintf("Failed to contact the collector, waiting %s before retry", sleepTime.String()))
			time.Sleep(sleepTime)
			err = q.wrapped.EmitBatch(batch)
			if err == nil {
				q.retryMutex.Lock()
				q.currentRetryInterval = q.initialRetryInterval
				q.reporterMetrics.BatchMetrics.RetryInterval.Update(int64(q.currentRetryInterval))
				q.lastRetryIntervalChange = time.Now()
				q.retryMutex.Unlock()
				return nil
			}
		}
		q.reporterMetrics.BatchMetrics.BatchesFailures.Inc(1)
		q.reporterMetrics.BatchMetrics.SpansFailures.Inc(spansCount)
		q.logger.Error("Could not send batch", zap.Error(err))
		return err
	}
	return nil
}

func IsRetryable(err error) bool {
	if r, ok := err.(RetryableError); ok {
		return r.IsRetryable()
	}
	return false
}
