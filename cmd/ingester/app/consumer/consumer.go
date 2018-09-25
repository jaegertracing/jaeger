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
	"time"

	"github.com/Shopify/sarama"
	sc "github.com/bsm/sarama-cluster"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
	"github.com/jaegertracing/jaeger/pkg/kafka/consumer"
)

// Params are the parameters of a Consumer
type Params struct {
	ProcessorFactory ProcessorFactory
	Factory          metrics.Factory
	Logger           *zap.Logger
	InternalConsumer consumer.Consumer
}

// Consumer uses sarama to consume and handle messages from kafka
type Consumer struct {
	metricsFactory metrics.Factory
	logger         *zap.Logger

	internalConsumer consumer.Consumer
	processorFactory ProcessorFactory

	partitionIDToState map[int32]*consumerState
	timeSince          func(time.Time) time.Duration
}

type consumerState struct {
	wg                sync.WaitGroup
	partitionConsumer sc.PartitionConsumer
}

// New is a constructor for a Consumer
func New(params Params) (*Consumer, error) {
	return &Consumer{
		metricsFactory:     params.Factory,
		logger:             params.Logger,
		internalConsumer:   params.InternalConsumer,
		processorFactory:   params.ProcessorFactory,
		partitionIDToState: make(map[int32]*consumerState),
		timeSince:          time.Since,
	}, nil
}

// Start begins consuming messages in a go routine
func (c *Consumer) Start() {
	go func() {
		c.logger.Info("Starting main loop")
		for pc := range c.internalConsumer.Partitions() {
			if p, ok := c.partitionIDToState[pc.Partition()]; ok {
				// This is a guard against simultaneously draining messages
				// from the last time the partition was assigned and
				// processing new messages for the same partition, which may lead
				// to the cleanup process not completing
				p.wg.Wait()
			}
			c.partitionIDToState[pc.Partition()] = &consumerState{partitionConsumer: pc}
			go c.handleMessages(pc)
			go c.handleErrors(pc.Partition(), pc.Errors())
		}
	}()
}

// Close closes the Consumer and underlying sarama consumer
func (c *Consumer) Close() error {
	for _, p := range c.partitionIDToState {
		c.closePartition(p.partitionConsumer)
		p.wg.Wait()
	}
	c.logger.Info("Closing parent consumer")
	return c.internalConsumer.Close()
}

func (c *Consumer) handleMessages(pc sc.PartitionConsumer) {
	c.logger.Info("Starting message handler", zap.Int32("partition", pc.Partition()))
	c.partitionIDToState[pc.Partition()].wg.Add(1)
	defer c.partitionIDToState[pc.Partition()].wg.Done()
	defer c.closePartition(pc)

	msgMetrics := c.newMsgMetrics(pc.Partition())
	var msgProcessor processor.SpanProcessor

	for msg := range pc.Messages() {
		c.logger.Debug("Got msg", zap.Any("msg", msg))
		msgMetrics.counter.Inc(1)
		msgMetrics.offsetGauge.Update(msg.Offset)
		if !msg.BlockTimestamp.IsZero() { // This timestamp is zero for Kafka broker (or the broker data format) < v0.10.2
			msgMetrics.lagGaugeNs.Update(c.timeSince(msg.BlockTimestamp).Nanoseconds())
		}
		msgMetrics.lagGauge.Update(pc.HighWaterMarkOffset() - msg.Offset - 1)

		if msgProcessor == nil {
			msgProcessor = c.processorFactory.new(pc.Partition(), msg.Offset-1)
			defer msgProcessor.Close()
		}

		msgProcessor.Process(&saramaMessageWrapper{msg})
	}
	c.logger.Info("Finished handling messages", zap.Int32("partition", pc.Partition()))
}

func (c *Consumer) closePartition(partitionConsumer sc.PartitionConsumer) {
	c.logger.Info("Closing partition consumer", zap.Int32("partition", partitionConsumer.Partition()))
	partitionConsumer.Close() // blocks until messages channel is drained
	c.logger.Info("Closed partition consumer", zap.Int32("partition", partitionConsumer.Partition()))
}

func (c *Consumer) handleErrors(partition int32, errChan <-chan *sarama.ConsumerError) {
	c.logger.Info("Starting error handler", zap.Int32("partition", partition))
	c.partitionIDToState[partition].wg.Add(1)
	defer c.partitionIDToState[partition].wg.Done()

	errMetrics := c.newErrMetrics(partition)
	for err := range errChan {
		errMetrics.errCounter.Inc(1)
		c.logger.Error("Error consuming from Kafka", zap.Error(err))
	}
	c.logger.Info("Finished handling errors", zap.Int32("partition", partition))
}
