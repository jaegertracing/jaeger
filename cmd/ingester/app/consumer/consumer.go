// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"sync"
	"time"

	"github.com/Shopify/sarama"
	sc "github.com/bsm/sarama-cluster"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/kafka/consumer"
)

// Params are the parameters of a Consumer
type Params struct {
	ProcessorFactory      ProcessorFactory
	MetricsFactory        metrics.Factory
	Logger                *zap.Logger
	InternalConsumer      consumer.Consumer
	DeadlockCheckInterval time.Duration
}

// Consumer uses sarama to consume and handle messages from kafka
type Consumer struct {
	metricsFactory metrics.Factory
	logger         *zap.Logger

	internalConsumer consumer.Consumer
	processorFactory ProcessorFactory

	deadlockDetector deadlockDetector

	partitionIDToState  map[int32]*consumerState
	partitionMapLock    sync.Mutex
	partitionsHeld      int64
	partitionsHeldGauge metrics.Gauge

	doneWg sync.WaitGroup
}

type consumerState struct {
	partitionConsumer sc.PartitionConsumer
}

// New is a constructor for a Consumer
func New(params Params) (*Consumer, error) {
	deadlockDetector := newDeadlockDetector(params.MetricsFactory, params.Logger, params.DeadlockCheckInterval)
	return &Consumer{
		metricsFactory:      params.MetricsFactory,
		logger:              params.Logger,
		internalConsumer:    params.InternalConsumer,
		processorFactory:    params.ProcessorFactory,
		deadlockDetector:    deadlockDetector,
		partitionIDToState:  make(map[int32]*consumerState),
		partitionsHeldGauge: partitionsHeldGauge(params.MetricsFactory),
	}, nil
}

// Start begins consuming messages in a go routine
func (c *Consumer) Start() {
	c.deadlockDetector.start()
	c.doneWg.Add(1)
	go func() {
		defer c.doneWg.Done()
		c.logger.Info("Starting main loop")
		for pc := range c.internalConsumer.Partitions() {
			c.partitionMapLock.Lock()
			c.partitionIDToState[pc.Partition()] = &consumerState{partitionConsumer: pc}
			c.partitionMapLock.Unlock()
			c.partitionMetrics(pc.Topic(), pc.Partition()).startCounter.Inc(1)

			c.doneWg.Add(2)
			go c.handleMessages(pc)
			go c.handleErrors(pc.Topic(), pc.Partition(), pc.Errors())
		}
	}()
}

// Close closes the Consumer and underlying sarama consumer
func (c *Consumer) Close() error {
	// Close the internal consumer, which will close each partition consumers' message and error channels.
	c.logger.Info("Closing parent consumer")
	err := c.internalConsumer.Close()

	c.logger.Debug("Closing deadlock detector")
	c.deadlockDetector.close()

	c.logger.Debug("Waiting for messages and errors to be handled")
	c.doneWg.Wait()

	return err
}

// handleMessages handles incoming Kafka messages on a channel
func (c *Consumer) handleMessages(pc sc.PartitionConsumer) {
	c.logger.Info("Starting message handler", zap.Int32("partition", pc.Partition()))
	c.partitionMapLock.Lock()
	c.partitionsHeld++
	c.partitionsHeldGauge.Update(c.partitionsHeld)
	c.partitionMapLock.Unlock()
	defer func() {
		c.closePartition(pc)
		c.partitionMapLock.Lock()
		c.partitionsHeld--
		c.partitionsHeldGauge.Update(c.partitionsHeld)
		c.partitionMapLock.Unlock()
		c.doneWg.Done()
	}()

	msgMetrics := c.newMsgMetrics(pc.Topic(), pc.Partition())

	var msgProcessor processor.SpanProcessor

	deadlockDetector := c.deadlockDetector.startMonitoringForPartition(pc.Partition())
	defer deadlockDetector.close()

	for {
		select {
		case msg, ok := <-pc.Messages():
			if !ok {
				c.logger.Info("Message channel closed. ", zap.Int32("partition", pc.Partition()))
				return
			}
			c.logger.Debug("Got msg", zap.Any("msg", msg))
			msgMetrics.counter.Inc(1)
			msgMetrics.offsetGauge.Update(msg.Offset)
			msgMetrics.lagGauge.Update(pc.HighWaterMarkOffset() - msg.Offset - 1)
			deadlockDetector.incrementMsgCount()

			if msgProcessor == nil {
				msgProcessor = c.processorFactory.new(pc.Topic(), pc.Partition(), msg.Offset-1)
				// revive:disable-next-line defer
				defer msgProcessor.Close()
			}

			err := msgProcessor.Process(saramaMessageWrapper{msg})
			if err != nil {
				c.logger.Error("Failed to process a Kafka message", zap.Error(err), zap.Int32("partition", msg.Partition), zap.Int64("offset", msg.Offset))
			}

		case <-deadlockDetector.closePartitionChannel():
			c.logger.Info("Closing partition due to inactivity", zap.Int32("partition", pc.Partition()))
			return
		}
	}
}

func (c *Consumer) closePartition(partitionConsumer sc.PartitionConsumer) {
	c.logger.Info("Closing partition consumer", zap.Int32("partition", partitionConsumer.Partition()))
	partitionConsumer.Close() // blocks until messages channel is drained
	c.partitionMetrics(partitionConsumer.Topic(), partitionConsumer.Partition()).closeCounter.Inc(1)
	c.logger.Info("Closed partition consumer", zap.Int32("partition", partitionConsumer.Partition()))
}

// handleErrors handles incoming Kafka consumer errors on a channel
func (c *Consumer) handleErrors(topic string, partition int32, errChan <-chan *sarama.ConsumerError) {
	c.logger.Info("Starting error handler", zap.Int32("partition", partition))
	defer c.doneWg.Done()

	errMetrics := c.newErrMetrics(topic, partition)
	for err := range errChan {
		errMetrics.errCounter.Inc(1)
		c.logger.Error("Error consuming from Kafka", zap.Error(err))
	}
	c.logger.Info("Finished handling errors", zap.Int32("partition", partition))
}
