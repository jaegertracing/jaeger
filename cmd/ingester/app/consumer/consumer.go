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
	"io"
	"sync"

	"github.com/Shopify/sarama"
	sc "github.com/bsm/sarama-cluster"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
)

type consumer struct {
	metricsFactory   metrics.Factory
	logger           *zap.Logger
	processorFactory processorFactory

	close    chan struct{}
	isClosed sync.WaitGroup

	SaramaConsumer
}

// SaramaConsumer is an interface to features of Sarama that we use
type SaramaConsumer interface {
	Partitions() <-chan sc.PartitionConsumer
	MarkPartitionOffset(topic string, partition int32, offset int64, metadata string)
	io.Closer
}

func (c *consumer) mainLoop() {
	c.isClosed.Add(1)
	c.logger.Info("Starting main loop")
	go func() {
		for {
			select {
			case pc := <-c.Partitions():
				c.isClosed.Add(2)

				go c.handleMessages(pc)
				go c.handleErrors(pc.Partition(), pc.Errors())

			case <-c.close:
				c.isClosed.Done()
				return
			}
		}
	}()
}

func (c *consumer) handleMessages(pc sc.PartitionConsumer) {
	c.logger.Info("Starting message handler")
	defer c.isClosed.Done()
	defer c.closePartition(pc)

	msgMetrics := c.newMsgMetrics(pc.Partition())
	var msgProcessor processor.SpanProcessor

	for msg := range pc.Messages() {
		c.logger.Debug("Got msg", zap.Any("msg", msg))
		msgMetrics.counter.Inc(1)
		msgMetrics.offsetGauge.Update(msg.Offset)
		msgMetrics.lagGauge.Update(pc.HighWaterMarkOffset() - msg.Offset - 1)

		if msgProcessor == nil {
			msgProcessor = c.processorFactory.new(pc.Partition(), msg.Offset-1)
			defer msgProcessor.Close()
		}

		msgProcessor.Process(&saramaMessageWrapper{msg})
	}
}

func (c *consumer) closePartition(partitionConsumer sc.PartitionConsumer) {
	c.logger.Info("Closing partition consumer", zap.Int32("partition", partitionConsumer.Partition()))
	partitionConsumer.Close() // blocks until messages channel is drained
	c.logger.Info("Closed partition consumer", zap.Int32("partition", partitionConsumer.Partition()))
}

func (c *consumer) handleErrors(partition int32, errChan <-chan *sarama.ConsumerError) {
	c.logger.Info("Starting error handler")
	defer c.isClosed.Done()

	errMetrics := c.newErrMetrics(partition)
	for err := range errChan {
		errMetrics.errCounter.Inc(1)
		c.logger.Error("Error consuming from Kafka", zap.Error(err))
	}
}

func (c *consumer) Close() error {
	close(c.close)
	c.isClosed.Wait()
	return c.SaramaConsumer.Close()
}
