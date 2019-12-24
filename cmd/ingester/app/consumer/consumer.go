// Copyright (c) 2018 The Jaeger Authors.
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

package consumer

import (
	"sync"
	"context"

	"github.com/Shopify/sarama"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
	"github.com/jaegertracing/jaeger/pkg/kafka/consumer"
)

// Params are the parameters of a Consumer
type Params struct {
	ProcessorFactory ProcessorFactory
	MetricsFactory   metrics.Factory
	Logger           *zap.Logger
	InternalConsumer consumer.Consumer
}

// Consumer uses sarama to consume and handle messages from kafka
type Consumer struct {
	metricsFactory      metrics.Factory
	logger              *zap.Logger
	internalConsumer    consumer.Consumer
	processorFactory    ProcessorFactory
	partitionsHeld      int64
	partitionsHeldGauge metrics.Gauge
}

type consumerGroupHandler struct {
	processorFactory         ProcessorFactory
	partitionToProcessor     map[int32]processor.SpanProcessor
	logger                   *zap.Logger
	consumer                 *Consumer
	partitionToProcessorLock sync.RWMutex
}

func (h *consumerGroupHandler) Setup(session sarama.ConsumerGroupSession) error {
	h.partitionToProcessor = map[int32]processor.SpanProcessor{}
	return nil
}

func (h *consumerGroupHandler) getProcessFactory(session sarama.ConsumerGroupSession, partition int32, offset int64) processor.SpanProcessor {
	h.partitionToProcessorLock.RLock()
	msgProcessor := h.partitionToProcessor[partition]
	h.partitionToProcessorLock.RUnlock()
	if msgProcessor == nil {
		msgProcessor = h.processorFactory.new(session, partition, offset-1)
		h.partitionToProcessorLock.Lock()
		h.partitionToProcessor[partition] = msgProcessor
		h.partitionToProcessorLock.Unlock()
	}
	return msgProcessor
}
func (h *consumerGroupHandler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }
func (h *consumerGroupHandler) ConsumeClaim(sess sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	msgMetrics := h.consumer.newMsgMetrics(claim.Partition())

	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				h.logger.Info("Message channel closed. ", zap.Int32("partition", claim.Partition()))
				return nil
			}
			h.logger.Debug("Got msg", zap.Any("msg", msg))
			msgMetrics.counter.Inc(1)
			msgMetrics.offsetGauge.Update(msg.Offset)
			msgMetrics.lagGauge.Update(claim.HighWaterMarkOffset() - msg.Offset - 1)
			msgProcessor := h.getProcessFactory(sess, claim.Partition(), msg.Offset)
			msgProcessor.Process(&saramaMessageWrapper{msg})
		}
	}
	return nil
}

// New is a constructor for a Consumer
func New(params Params) (*Consumer, error) {
	return &Consumer{
		metricsFactory:      params.MetricsFactory,
		logger:              params.Logger,
		internalConsumer:    params.InternalConsumer,
		processorFactory:    params.ProcessorFactory,
		partitionsHeldGauge: partitionsHeldGauge(params.MetricsFactory),
	}, nil
}

// Start begins consuming messages in a go routine
func (c *Consumer) Start() {
	go func() {
		c.logger.Info("Starting main loop")
		ctx := context.Background()
		handler := consumerGroupHandler{
			processorFactory: c.processorFactory,
			logger:           c.logger,
			consumer:         c,
		}
		defer func() { _ = c.internalConsumer.Close() }()

		go func() {
			for err := range c.internalConsumer.Errors() {
				if error, ok := err.(*sarama.ConsumerError); ok {
					c.logger.Info("Starting error handler", zap.Int32("partition", error.Partition))
					errMetrics := c.newErrMetrics(error.Partition)
					errMetrics.errCounter.Inc(1)
					c.logger.Error("Error consuming from Kafka", zap.Error(err))
				}
			}
			c.logger.Info("Finished handling errors")
		}()

		for {
			err := c.internalConsumer.Consume(ctx, &handler)
			if err != nil {
				panic(err)
			}

		}
	}()
}

// Close closes the Consumer and underlying sarama consumer
func (c *Consumer) Close() error {
	c.logger.Info("Closing parent consumer")
	return c.internalConsumer.Close()
}
