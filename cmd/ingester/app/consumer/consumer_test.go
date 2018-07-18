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
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/bsm/sarama-cluster"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/testutils"
	"go.uber.org/zap"

	kmocks "github.com/jaegertracing/jaeger/cmd/ingester/app/consumer/mocks"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor/mocks"
)

//go:generate mockery -name partitionConsumer -inpkg
//go:generate mockery -dir ../../../../../vendor/github.com/bsm/sarama-cluster/ -name PartitionConsumer

type consumerTest struct {
	saramaConsumer    *kmocks.SaramaConsumer
	consumer          *spanConsumer
	partitionConsumer *kmocks.PartitionConsumer
}

func withWrappedConsumer(fn func(c *consumerTest)) {
	sc := &kmocks.SaramaConsumer{}
	logger, _ := zap.NewDevelopment()
	metricsFactory := metrics.NewLocalFactory(0)
	c := &consumerTest{
		saramaConsumer: sc,
		consumer: &spanConsumer{
			metricsFactory:    metricsFactory,
			logger:            logger,
			close:             make(chan struct{}),
			isClosed:          sync.WaitGroup{},
			partitionConsumer: sc,
			processorFactory: processorFactory{
				topic:          "topic",
				consumer:       sc,
				metricsFactory: metricsFactory,
				logger:         logger,
				baseProcessor:  &mocks.SpanProcessor{},
				parallelism:    1,
			},
		},
	}

	c.partitionConsumer = &kmocks.PartitionConsumer{}
	pcha := make(chan cluster.PartitionConsumer, 1)
	pcha <- c.partitionConsumer
	c.saramaConsumer.On("Partitions").Return((<-chan cluster.PartitionConsumer)(pcha))
	c.saramaConsumer.On("Close").Return(nil)
	c.saramaConsumer.On("MarkPartitionOffset", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	fn(c)
}

func TestSaramaConsumerWrapper_MarkPartitionOffset(t *testing.T) {

	withWrappedConsumer(func(c *consumerTest) {
		topic := "morekuzambu"
		partition := int32(316)
		offset := int64(1111110111111)
		metadata := "meatbag"
		c.saramaConsumer.On("MarkPartitionOffset", topic, partition, offset, metadata).Return()

		c.consumer.MarkPartitionOffset(topic, partition, offset, metadata)

		c.saramaConsumer.AssertCalled(t, "MarkPartitionOffset", topic, partition, offset, metadata)
	})
}

func TestSaramaConsumerWrapper_start_Messages(t *testing.T) {
	withWrappedConsumer(func(c *consumerTest) {
		msg := &sarama.ConsumerMessage{}
		msg.Offset = 0
		msgCh := make(chan *sarama.ConsumerMessage, 1)
		msgCh <- msg

		errCh := make(chan *sarama.ConsumerError, 1)
		c.partitionConsumer.On("Partition").Return(int32(0))
		c.partitionConsumer.On("Errors").Return((<-chan *sarama.ConsumerError)(errCh))
		c.partitionConsumer.On("Messages").Return((<-chan *sarama.ConsumerMessage)(msgCh))
		c.partitionConsumer.On("HighWaterMarkOffset").Return(int64(1234))
		c.partitionConsumer.On("Close").Return(nil)

		mp := &mocks.SpanProcessor{}
		mp.On("Process", &saramaMessageWrapper{msg}).Return(nil)
		c.consumer.processorFactory.baseProcessor = mp

		c.consumer.Start()
		time.Sleep(100 * time.Millisecond)
		close(msgCh)
		close(errCh)
		c.consumer.Close()

		mp.AssertExpectations(t)

		f := (c.consumer.metricsFactory).(*metrics.LocalFactory)
		partitionTag := map[string]string{"partition": "0"}
		testutils.AssertCounterMetrics(t, f, testutils.ExpectedMetric{
			Name:  "sarama-consumer.messages",
			Tags:  partitionTag,
			Value: 1,
		})
		testutils.AssertGaugeMetrics(t, f, testutils.ExpectedMetric{
			Name:  "sarama-consumer.current-offset",
			Tags:  partitionTag,
			Value: 0,
		})
		testutils.AssertGaugeMetrics(t, f, testutils.ExpectedMetric{
			Name:  "sarama-consumer.offset-lag",
			Tags:  partitionTag,
			Value: 1233,
		})
	})
}

func TestSaramaConsumerWrapper_start_Errors(t *testing.T) {
	withWrappedConsumer(func(c *consumerTest) {
		errCh := make(chan *sarama.ConsumerError, 1)
		errCh <- &sarama.ConsumerError{
			Topic: "some-topic",
			Err:   errors.New("some error"),
		}

		msgCh := make(chan *sarama.ConsumerMessage)

		c.partitionConsumer.On("Partition").Return(int32(0))
		c.partitionConsumer.On("Errors").Return((<-chan *sarama.ConsumerError)(errCh))
		c.partitionConsumer.On("Messages").Return((<-chan *sarama.ConsumerMessage)(msgCh))
		c.partitionConsumer.On("Close").Return(nil)

		c.consumer.Start()
		time.Sleep(100 * time.Millisecond)
		close(msgCh)
		close(errCh)
		c.consumer.Close()
		f := (c.consumer.metricsFactory).(*metrics.LocalFactory)
		partitionTag := map[string]string{"partition": "0"}
		testutils.AssertCounterMetrics(t, f, testutils.ExpectedMetric{
			Name:  "sarama-consumer.errors",
			Tags:  partitionTag,
			Value: 1,
		})
	})
}
