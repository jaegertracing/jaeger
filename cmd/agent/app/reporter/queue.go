package reporter

import (
	"fmt"
	"sync"
	"time"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/queue"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

type Queue interface {
	Enqueue(*jaeger.Batch) error
}

type QueuedReporter struct {
	wrapped Reporter
	queue   Queue
	logger  *zap.Logger

	retryMutex sync.Mutex

	lastRetry            time.Time
	currentRetryInterval time.Duration
	maxRetryInterval     time.Duration
	initialRetryInterval time.Duration
}

func WrapWithQueue(reporter Reporter, logger *zap.Logger) *QueuedReporter {
	q := &QueuedReporter{
		wrapped:              reporter,
		logger:               logger,
		lastRetry:            time.Now(),
		initialRetryInterval: time.Millisecond * 100,
		maxRetryInterval:     20 * time.Second,
	}
	q.currentRetryInterval = q.initialRetryInterval
	q.queue = queue.NewBoundQueue(1000, q.batchProcessor, logger)
	return q
}

func (q *QueuedReporter) EmitZipkinBatch(spans []*zipkincore.Span) error {
	return q.wrapped.EmitZipkinBatch(spans)
}

func (q *QueuedReporter) EmitBatch(batch *jaeger.Batch) error {
	return q.queue.Enqueue(batch)
}

func (q *QueuedReporter) backOffTimer() time.Duration {
	// Has to be more than previous time from previous increase before we can reincrease
	// otherwise simultaneous threads could race to increase the sleep time too quickly
	t := time.Now()
	if q.lastRetry.Add(q.currentRetryInterval).Before(t) && q.currentRetryInterval < q.maxRetryInterval {
		// We can increase, more than the previous timer has been spent
		q.retryMutex.Lock()
		if q.lastRetry.Add(q.currentRetryInterval).Before(t) {
			// We have to do the recheck because someone could have changed the time between check and mutex locking
			newWait := q.currentRetryInterval * 2
			if newWait > q.maxRetryInterval {
				q.currentRetryInterval = q.maxRetryInterval
			} else {
				q.currentRetryInterval = newWait
			}
		}
		q.retryMutex.Unlock()
	}

	return q.currentRetryInterval
}

func (q *QueuedReporter) batchProcessor(batch *jaeger.Batch) error {
	err := q.wrapped.EmitBatch(batch)
	if err != nil {
		for IsRetryable(err) {
			// Block this processing instance before returning
			sleepTime := q.backOffTimer()
			q.logger.Info(fmt.Sprintf("Failed to contact the collector, waiting %s before retry", sleepTime.String()))
			time.Sleep(sleepTime)
			err = q.wrapped.EmitBatch(batch)
			if err == nil {
				q.retryMutex.Lock()
				q.currentRetryInterval = q.initialRetryInterval
				q.retryMutex.Unlock()
				return nil
			}
		}
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
