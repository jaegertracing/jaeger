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
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/queue"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
)

type queueMetrics struct {
	// QueueSize is a counter indicating how many items are currently in the queue
	QueueSize metrics.Gauge `metric:"boundqueue.queuesize"`

	// BatchesDropped indicates the number of batches dropped by server
	BatchesDropped metrics.Counter `metric:"batches.dropped"`
}

// Bound is using BoundedQueue (from bounded_queue.go) for QueueReporter
type Bound struct {
	queue   *queue.BoundedQueue
	logger  *zap.Logger
	metrics queueMetrics
}

// NewBoundQueue creates a new bounded queue with non-transactional processing
func NewBoundQueue(bufSize, concurrency int, processor func(*jaeger.Batch) error, logger *zap.Logger, mFactory metrics.Factory) *Bound {
	b := &Bound{
		logger:  logger,
		metrics: queueMetrics{},
	}
	b.queue = queue.NewBoundedQueue(bufSize, b.droppedItem)

	b.queue.StartConsumers(concurrency, func(item interface{}) {
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

func (b *Bound) droppedItem(item interface{}) {
	b.metrics.BatchesDropped.Inc(1)
}

// Enqueue pushes the batch to the queue or returns and error if the queue is full
func (b *Bound) Enqueue(batch *jaeger.Batch) error {
	success := b.queue.Produce(batch)
	if !success {
		return fmt.Errorf("destination queue could not accept new entries")
	}
	return nil
}
