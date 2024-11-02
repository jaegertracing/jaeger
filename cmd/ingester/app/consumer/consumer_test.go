// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	smocks "github.com/Shopify/sarama/mocks"
	cluster "github.com/bsm/sarama-cluster"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
	pmocks "github.com/jaegertracing/jaeger/cmd/ingester/app/processor/mocks"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/pkg/kafka/consumer"
	kmocks "github.com/jaegertracing/jaeger/pkg/kafka/consumer/mocks"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

//go:generate mockery -dir ../../../../pkg/kafka/config/ -name Consumer
//go:generate mockery -dir ../../../../../vendor/github.com/bsm/sarama-cluster/ -name PartitionConsumer

const (
	topic     = "morekuzambu"
	partition = int32(316)
	msgOffset = int64(1111110111111)
)

func TestConstructor(t *testing.T) {
	newConsumer, err := New(Params{MetricsFactory: metrics.NullFactory})
	require.NoError(t, err)
	assert.NotNil(t, newConsumer)
}

// partitionConsumerWrapper wraps a Sarama partition consumer into a Sarama cluster partition consumer
type partitionConsumerWrapper struct {
	topic     string
	partition int32

	sarama.PartitionConsumer
}

func (s partitionConsumerWrapper) Partition() int32 {
	return s.partition
}

func (s partitionConsumerWrapper) Topic() string {
	return s.topic
}

func newSaramaClusterConsumer(saramaPartitionConsumer sarama.PartitionConsumer, mc *smocks.PartitionConsumer) *kmocks.Consumer {
	pcha := make(chan cluster.PartitionConsumer, 1)
	pcha <- &partitionConsumerWrapper{
		topic:             topic,
		partition:         partition,
		PartitionConsumer: saramaPartitionConsumer,
	}
	saramaClusterConsumer := &kmocks.Consumer{}
	saramaClusterConsumer.On("Partitions").Return((<-chan cluster.PartitionConsumer)(pcha))
	saramaClusterConsumer.On("Close").Return(nil).Run(func(_ mock.Arguments) {
		mc.Close()
		close(pcha)
	})
	saramaClusterConsumer.On("MarkPartitionOffset", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	return saramaClusterConsumer
}

func newConsumer(
	t *testing.T,
	metricsFactory metrics.Factory,
	_ string, /* topic */
	processor processor.SpanProcessor,
	consumer consumer.Consumer,
) *Consumer {
	logger, _ := zap.NewDevelopment()
	consumerParams := Params{
		MetricsFactory:   metricsFactory,
		Logger:           logger,
		InternalConsumer: consumer,
		ProcessorFactory: ProcessorFactory{
			consumer:       consumer,
			metricsFactory: metricsFactory,
			logger:         logger,
			baseProcessor:  processor,
			parallelism:    1,
		},
	}

	c, err := New(consumerParams)
	require.NoError(t, err)
	return c
}

func TestSaramaConsumerWrapper_MarkPartitionOffset(t *testing.T) {
	sc := &kmocks.Consumer{}
	metadata := "meatbag"
	sc.On("MarkPartitionOffset", topic, partition, msgOffset, metadata).Return()
	sc.MarkPartitionOffset(topic, partition, msgOffset, metadata)
	sc.AssertCalled(t, "MarkPartitionOffset", topic, partition, msgOffset, metadata)
}

func TestSaramaConsumerWrapper_start_Messages(t *testing.T) {
	localFactory := metricstest.NewFactory(0)

	msg := &sarama.ConsumerMessage{}

	isProcessed := sync.WaitGroup{}
	isProcessed.Add(1)
	mp := &pmocks.SpanProcessor{}
	mp.On("Process", saramaMessageWrapper{msg}).Return(func(_ processor.Message) error {
		isProcessed.Done()
		return nil
	})

	saramaConsumer := smocks.NewConsumer(t, &sarama.Config{})
	mc := saramaConsumer.ExpectConsumePartition(topic, partition, msgOffset)
	mc.ExpectMessagesDrainedOnClose()

	saramaPartitionConsumer, e := saramaConsumer.ConsumePartition(topic, partition, msgOffset)
	require.NoError(t, e)

	undertest := newConsumer(t, localFactory, topic, mp, newSaramaClusterConsumer(saramaPartitionConsumer, mc))

	undertest.partitionIDToState = map[int32]*consumerState{
		partition: {
			partitionConsumer: &partitionConsumerWrapper{
				topic:             topic,
				partition:         partition,
				PartitionConsumer: &smocks.PartitionConsumer{},
			},
		},
	}

	undertest.Start()

	mc.YieldMessage(msg)
	isProcessed.Wait()

	localFactory.AssertGaugeMetrics(t, metricstest.ExpectedMetric{
		Name:  "sarama-consumer.partitions-held",
		Value: 1,
	})

	mp.AssertExpectations(t)
	// Ensure that the partition consumer was updated in the map
	assert.Equal(t, saramaPartitionConsumer.HighWaterMarkOffset(),
		undertest.partitionIDToState[partition].partitionConsumer.HighWaterMarkOffset())
	undertest.Close()

	localFactory.AssertCounterMetrics(t, metricstest.ExpectedMetric{
		Name:  "sarama-consumer.partitions-held",
		Value: 0,
	})

	tags := map[string]string{
		"topic":     topic,
		"partition": fmt.Sprint(partition),
	}
	localFactory.AssertCounterMetrics(t, metricstest.ExpectedMetric{
		Name:  "sarama-consumer.messages",
		Tags:  tags,
		Value: 1,
	})
	localFactory.AssertGaugeMetrics(t, metricstest.ExpectedMetric{
		Name:  "sarama-consumer.current-offset",
		Tags:  tags,
		Value: int(msgOffset),
	})
	localFactory.AssertGaugeMetrics(t, metricstest.ExpectedMetric{
		Name: "sarama-consumer.offset-lag",
		Tags: tags,
		// Prior to sarama v1.31.0 this would be 0, it's unclear why this changed.
		// v=1 seems to be correct because high watermark in mock is incremented upon
		// consuming the message, and func HighWaterMarkOffset() returns internal value
		// (already incremented) + 1, so the difference is always 2, and we then
		// subtract 1 from it.
		Value: 1,
	})
	localFactory.AssertCounterMetrics(t, metricstest.ExpectedMetric{
		Name:  "sarama-consumer.partition-start",
		Tags:  tags,
		Value: 1,
	})
}

func TestSaramaConsumerWrapper_start_Errors(t *testing.T) {
	localFactory := metricstest.NewFactory(0)

	saramaConsumer := smocks.NewConsumer(t, &sarama.Config{})
	mc := saramaConsumer.ExpectConsumePartition(topic, partition, msgOffset)
	mc.ExpectErrorsDrainedOnClose()

	saramaPartitionConsumer, e := saramaConsumer.ConsumePartition(topic, partition, msgOffset)
	require.NoError(t, e)

	undertest := newConsumer(t, localFactory, topic, &pmocks.SpanProcessor{}, newSaramaClusterConsumer(saramaPartitionConsumer, mc))

	undertest.Start()
	mc.YieldError(errors.New("Daisy, Daisy"))

	for i := 0; i < 1000; i++ {
		time.Sleep(time.Millisecond)

		c, _ := localFactory.Snapshot()
		if len(c) == 0 {
			continue
		}

		tags := map[string]string{
			"topic":     topic,
			"partition": fmt.Sprint(partition),
		}
		localFactory.AssertCounterMetrics(t, metricstest.ExpectedMetric{
			Name:  "sarama-consumer.errors",
			Tags:  tags,
			Value: 1,
		})
		undertest.Close()
		return
	}

	t.Fail()
}

func TestHandleClosePartition(t *testing.T) {
	metricsFactory := metricstest.NewFactory(0)

	mp := &pmocks.SpanProcessor{}
	saramaConsumer := smocks.NewConsumer(t, &sarama.Config{})
	mc := saramaConsumer.ExpectConsumePartition(topic, partition, msgOffset)
	mc.ExpectErrorsDrainedOnClose()
	saramaPartitionConsumer, e := saramaConsumer.ConsumePartition(topic, partition, msgOffset)
	require.NoError(t, e)

	undertest := newConsumer(t, metricsFactory, topic, mp, newSaramaClusterConsumer(saramaPartitionConsumer, mc))
	undertest.deadlockDetector = newDeadlockDetector(metricsFactory, undertest.logger, 200*time.Millisecond)
	undertest.Start()
	defer undertest.Close()

	for i := 0; i < 10; i++ {
		undertest.deadlockDetector.allPartitionsDeadlockDetector.incrementMsgCount() // Don't trigger panic on all partitions detector
		time.Sleep(100 * time.Millisecond)
		c, _ := metricsFactory.Snapshot()
		if c["sarama-consumer.partition-close|partition=316|topic=morekuzambu"] == 1 {
			return
		}
	}
	assert.Fail(t, "Did not close partition")
}
