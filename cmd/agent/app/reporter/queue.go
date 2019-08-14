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

	retryTimerChange  time.Time
	retryTimer        time.Duration
	retryMaxWait      time.Duration
	retryDefaultSleep time.Duration
}

func WrapWithQueue(reporter Reporter, logger *zap.Logger) *QueuedReporter {
	q := &QueuedReporter{
		wrapped:           reporter,
		logger:            logger,
		retryDefaultSleep: time.Millisecond * 100,
		retryTimerChange:  time.Now(),
		retryMaxWait:      20 * time.Second,
	}
	q.retryTimer = q.retryDefaultSleep
	q.queue = queue.NewBoundQueue(1000, q.batchProcessor, logger)
	return q
}

func (q *QueuedReporter) EmitZipkinBatch(spans []*zipkincore.Span) error {
	return nil
}

func (q *QueuedReporter) EmitBatch(batch *jaeger.Batch) error {
	return q.queue.Enqueue(batch)
}

func (q *QueuedReporter) backOffTimer() time.Duration {
	// Has to be more than previous time from previous increase before we can reincrease
	// otherwise simultaneous threads could race to increase the sleep time too quickly
	t := time.Now()
	if q.retryTimerChange.Add(q.retryTimer).Before(t) && q.retryTimer < q.retryMaxWait {
		// We can increase, more than the previous timer has been spent
		q.retryMutex.Lock()
		if q.retryTimerChange.Add(q.retryTimer).Before(t) {
			// We have to do the recheck because someone could have changed the time between check and mutex locking
			newWait := q.retryTimer * 2
			if newWait > q.retryMaxWait {
				q.retryTimer = q.retryMaxWait
			} else {
				q.retryTimer = newWait
			}
		}
		q.retryMutex.Unlock()
	}

	return q.retryTimer
}

func (q *QueuedReporter) batchProcessor(batch *jaeger.Batch) error {
	err := q.wrapped.EmitBatch(batch)
	if err != nil {
		for q.wrapped.Retryable(err) {
			// Block this processing instance before returning
			sleepTime := q.backOffTimer()
			q.logger.Info(fmt.Sprintf("Failed to contact the collector, waiting %s before retry", sleepTime.String()))
			time.Sleep(sleepTime)
			err = q.wrapped.EmitBatch(batch)
			if err == nil {
				q.retryMutex.Lock()
				q.retryTimer = q.retryDefaultSleep
				q.retryMutex.Unlock()
				break
			}
		}
		return err
	}
	return nil
}

func (q *QueuedReporter) Retryable(err error) bool {
	return q.wrapped.Retryable(err)
}
