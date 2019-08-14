package queue

import (
	"fmt"

	"github.com/jaegertracing/jaeger/pkg/queue"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"go.uber.org/zap"
)

type Bound struct {
	queue  *queue.BoundedQueue
	logger *zap.Logger
}

func NewBoundQueue(bufSize int, processor func(*jaeger.Batch) error, logger *zap.Logger) *Bound {
	b := &Bound{
		queue:  queue.NewBoundedQueue(bufSize, nil),
		logger: logger,
	}

	b.queue.StartConsumers(1, func(item interface{}) {
		// This queue does not have persistence, thus we don't handle transactionality
		err := processor(item.(*jaeger.Batch))
		if err != nil {
			b.logger.Error("Could not transmit span", zap.Error(err))
		}
	})

	return b
}

func (q *Bound) Enqueue(batch *jaeger.Batch) error {
	success := q.queue.Produce(batch)
	if !success {
		return fmt.Errorf("Destination queue could not accept new entries")
	}
	return nil
}
