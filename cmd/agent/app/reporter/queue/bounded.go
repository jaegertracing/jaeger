package queue

import (
	"fmt"
	"time"

	"github.com/jaegertracing/jaeger/pkg/queue"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
)

type queueMetrics struct {
	// queueSize is a counter indicating how many items are currently in the queue
	QueueSize metrics.Gauge `metric:"boundqueue.queuesize"`
}

type Bound struct {
	queue   *queue.BoundedQueue
	logger  *zap.Logger
	metrics queueMetrics
}

func NewBoundQueue(bufSize int, processor func(*jaeger.Batch) error, logger *zap.Logger, mFactory metrics.Factory) *Bound {
	b := &Bound{
		queue:   queue.NewBoundedQueue(bufSize, nil),
		logger:  logger,
		metrics: queueMetrics{},
	}

	b.queue.StartConsumers(1, func(item interface{}) {
		// This queue does not have persistence, thus we don't handle transactionality
		err := processor(item.(*jaeger.Batch))
		if err != nil {
			b.logger.Error("Could not transmit batch", zap.Error(err))
		}
	})

	metrics.Init(&b.metrics, mFactory.Namespace(metrics.NSOptions{Name: "reporter"}), nil)
	b.queue.StartLengthReporting(1*time.Second, b.metrics.QueueSize)

	return b
}

func (b *Bound) Enqueue(batch *jaeger.Batch) error {
	success := b.queue.Produce(batch)
	if !success {
		return fmt.Errorf("Destination queue could not accept new entries")
	}
	return nil
}
