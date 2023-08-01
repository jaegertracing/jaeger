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
	"context"
	"sync"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	kmocks "github.com/jaegertracing/jaeger/cmd/ingester/app/consumer/mocks"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
)

//go:generate mockery -dir ../../../../pkg/kafka/config/ -name Consumer

const (
	topic     = "jaegertracing_consumer"
	group     = "jaegertracing_consumer_group"
	partition = int32(316)
	msgOffset = int64(1111110111111)
)

func TestConstructor(t *testing.T) {
	newConsumer, err := New(Params{MetricsFactory: metrics.NullFactory})
	assert.NoError(t, err)
	assert.NotNil(t, newConsumer)
}

func TestSaramaConsumerWrapper_MarkPartitionOffset(t *testing.T) {
	sc := &kmocks.Consumer{}
	metadata := "meatbag"
	sc.On("MarkPartitionOffset", topic, partition, msgOffset, metadata).Return()
	sc.MarkPartitionOffset(topic, partition, msgOffset, metadata)
	sc.AssertCalled(t, "MarkPartitionOffset", topic, partition, msgOffset, metadata)
}

type Store struct {
	store                *memory.Store
	spanWriteCh          chan struct{}
	spanWriteChCloseOnce sync.Once
	t                    *testing.T
}

func (s *Store) WriteSpan(ctx context.Context, span *model.Span) error {
	err := s.store.WriteSpan(ctx, span)
	require.NoError(s.t, err)
	//
	//  1: start      consume -> 2: goroutine in sarama
	//  <--- 3: return <---|         |
	//  |                            |---> 2.2: consumeClaim -> 2.3: handleMessages -> 2.4: messageLoop -> 2.5: process -> 2.6: parallel process for goroutine
	//  |                                                                                                                          |
	// \|/                                                                                                                         |---> 2.7: writeSpan -> 2.8: spanWrite(memory) -> 2.9: spanWrite notice( close(spanWriteCh) ) ->  2.10: messageLoop in sarama -> ...
	//  |---> 3.2: <-spanWriteCh ->  3.3: consumer.Close() ->  3.4: finish !!!
	//
	s.spanWriteChCloseOnce.Do(func() {
		close(s.spanWriteCh)
	})
	return err
}

func TestGroupConsumer(t *testing.T) {
	config := sarama.NewConfig()
	config.ClientID = t.Name()
	config.Version = sarama.V2_0_0_0
	config.Consumer.Return.Errors = true
	config.Consumer.Group.Rebalance.Retry.Max = 2
	config.Consumer.Offsets.AutoCommit.Enable = false

	msg1 := newSampleSpan(t, model.NewTraceID(1, 2), model.NewSpanID(1))
	msg2 := newSampleSpan(t, model.NewTraceID(2, 3), model.NewSpanID(2))

	broker0 := sarama.NewMockBroker(t, 0)
	broker0.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).
			SetBroker(broker0.Addr(), broker0.BrokerID()).
			SetLeader(topic, 0, broker0.BrokerID()),
		"OffsetRequest": sarama.NewMockOffsetResponse(t).
			SetOffset(topic, 0, sarama.OffsetOldest, 0).
			SetOffset(topic, 0, sarama.OffsetNewest, 1),
		"FindCoordinatorRequest": sarama.NewMockFindCoordinatorResponse(t).
			SetCoordinator(sarama.CoordinatorGroup, group, broker0),
		"HeartbeatRequest": sarama.NewMockHeartbeatResponse(t),
		"JoinGroupRequest": sarama.NewMockSequence(
			sarama.NewMockJoinGroupResponse(t).SetError(sarama.ErrOffsetsLoadInProgress),
			sarama.NewMockJoinGroupResponse(t).SetGroupProtocol(sarama.RangeBalanceStrategyName),
		),
		"SyncGroupRequest": sarama.NewMockSequence(
			sarama.NewMockSyncGroupResponse(t).SetError(sarama.ErrOffsetsLoadInProgress),
			sarama.NewMockSyncGroupResponse(t).SetMemberAssignment(
				&sarama.ConsumerGroupMemberAssignment{
					Version: 0,
					Topics: map[string][]int32{
						topic: {0},
					},
				}),
		),
		"OffsetFetchRequest": sarama.NewMockOffsetFetchResponse(t).
			SetOffset(group, topic, 0, 0, "", sarama.ErrNoError).
			SetOffset(group, topic, 0, 1, "", sarama.ErrNoError).
			SetError(sarama.ErrNoError),
		"FetchRequest": sarama.NewMockSequence(
			sarama.NewMockFetchResponse(t, 1).
				SetMessage(topic, 0, 0, sarama.StringEncoder(msg1)).
				SetMessage(topic, 0, 1, sarama.StringEncoder(msg2)),
			sarama.NewMockFetchResponse(t, 1),
		),
	})

	saramaConsumer, err := sarama.NewConsumerGroup([]string{broker0.Addr()}, group, config)
	require.NoError(t, err)

	defer func() { _ = saramaConsumer.Close() }()

	unmarshaller := kafka.NewJSONUnmarshaller()
	innerSpanWriter := memory.NewStore()

	sw := &Store{
		store:       innerSpanWriter,
		spanWriteCh: make(chan struct{}, 1),
	}

	spParams := processor.SpanProcessorParams{
		Writer:       sw,
		Unmarshaller: unmarshaller,
	}

	spanProcessor := processor.NewSpanProcessor(spParams)

	logger, logBuf := testutils.NewLogger()
	factoryParams := ProcessorFactoryParams{
		Parallelism:    1,
		Topic:          topic,
		SaramaConsumer: saramaConsumer,
		BaseProcessor:  spanProcessor,
		Logger:         logger,
		Factory:        metrics.NullFactory,
	}

	processorFactory, err := NewProcessorFactory(factoryParams)
	require.NoError(t, err)

	consumerParams := Params{
		InternalConsumer:      saramaConsumer,
		ProcessorFactory:      *processorFactory,
		MetricsFactory:        metrics.NullFactory,
		Logger:                logger,
		DeadlockCheckInterval: 0,
	}

	consumer, err := New(consumerParams, WithWaitReady(true))
	require.NoError(t, err)

	sw.t = t

	consumer.Start()

	t.Log("Consumer is ready and wait message")

	<-sw.spanWriteCh

	consumer.Close()

	t.Logf("Consumer all logs: %s", logBuf.String())
}

func newSampleSpan(t *testing.T, traceID model.TraceID, spanID model.SpanID) string {
	sampleTags := model.KeyValues{
		model.String("someStringTagKey", "someStringTagValue"),
	}
	sampleSpan := &model.Span{
		TraceID:       traceID,
		SpanID:        spanID,
		OperationName: "someOperationName",
		References: []model.SpanRef{
			{
				TraceID: traceID,
				SpanID:  spanID,
				RefType: model.ChildOf,
			},
		},
		Flags:     model.Flags(1),
		StartTime: model.EpochMicrosecondsAsTime(55555),
		Duration:  model.MicrosecondsAsDuration(50000),
		Tags:      sampleTags,
		Logs: []model.Log{
			{
				Timestamp: model.EpochMicrosecondsAsTime(12345),
				Fields:    sampleTags,
			},
		},
		Process: &model.Process{
			ServiceName: "someServiceName",
			Tags:        sampleTags,
		},
	}

	m := &jsonpb.Marshaler{}
	msg, err := m.MarshalToString(sampleSpan)
	if err != nil {
		require.NoError(t, err)
		return ""
	}
	return msg
}

func TestGroupConsumerWithDeadlockDetector(t *testing.T) {
	config := sarama.NewConfig()
	config.ClientID = t.Name()
	config.Version = sarama.V2_0_0_0
	config.Consumer.Return.Errors = true
	config.Consumer.Group.Rebalance.Retry.Max = 2
	config.Consumer.Offsets.AutoCommit.Enable = false

	broker0 := sarama.NewMockBroker(t, 0)
	broker0.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).
			SetBroker(broker0.Addr(), broker0.BrokerID()).
			SetLeader(topic, 0, broker0.BrokerID()),
		"OffsetRequest": sarama.NewMockOffsetResponse(t).
			SetOffset(topic, 0, sarama.OffsetOldest, 0).
			SetOffset(topic, 0, sarama.OffsetNewest, 1),
		"FindCoordinatorRequest": sarama.NewMockFindCoordinatorResponse(t).
			SetCoordinator(sarama.CoordinatorGroup, group, broker0),
		"HeartbeatRequest": sarama.NewMockHeartbeatResponse(t),
		"JoinGroupRequest": sarama.NewMockSequence(
			sarama.NewMockJoinGroupResponse(t).SetError(sarama.ErrOffsetsLoadInProgress),
			sarama.NewMockJoinGroupResponse(t).SetGroupProtocol(sarama.RangeBalanceStrategyName),
		),
		"SyncGroupRequest": sarama.NewMockSequence(
			sarama.NewMockSyncGroupResponse(t).SetError(sarama.ErrOffsetsLoadInProgress),
			sarama.NewMockSyncGroupResponse(t).SetMemberAssignment(
				&sarama.ConsumerGroupMemberAssignment{
					Version: 0,
					Topics: map[string][]int32{
						topic: {0},
					},
				}),
		),
		"OffsetFetchRequest": sarama.NewMockOffsetFetchResponse(t).SetOffset(
			group, topic, 0, 0, "", sarama.ErrNoError,
		).SetError(sarama.ErrNoError),
		"FetchRequest": sarama.NewMockSequence(
			sarama.NewMockFetchResponse(t, 1),
		),
	})

	saramaConsumer, err := sarama.NewConsumerGroup([]string{broker0.Addr()}, group, config)
	require.NoError(t, err)

	defer func() { _ = saramaConsumer.Close() }()

	unmarshaller := kafka.NewJSONUnmarshaller()
	innerSpanWriter := memory.NewStore()

	sw := &Store{
		store:       innerSpanWriter,
		spanWriteCh: make(chan struct{}, 1),
	}

	spParams := processor.SpanProcessorParams{
		Writer:       sw,
		Unmarshaller: unmarshaller,
	}

	spanProcessor := processor.NewSpanProcessor(spParams)

	logger, logBuf := testutils.NewLogger()
	factoryParams := ProcessorFactoryParams{
		Parallelism:    1,
		Topic:          topic,
		SaramaConsumer: saramaConsumer,
		BaseProcessor:  spanProcessor,
		Logger:         logger,
		Factory:        metrics.NullFactory,
	}

	processorFactory, err := NewProcessorFactory(factoryParams)
	require.NoError(t, err)

	consumerParams := Params{
		InternalConsumer: saramaConsumer,
		ProcessorFactory: *processorFactory,
		MetricsFactory:   metrics.NullFactory,
		Logger:           logger,
		// DeadlockCheckInterval is waiting for the consumer to have a message within the specified time interval,
		// set here to time.Microsecond * 100 for test, and actually needs to be set up more for your needs
		DeadlockCheckInterval: time.Microsecond * 100,
	}

	consumer, err := New(consumerParams,
		WithGlobalDeadlockDetectorEnabled(false),
		WithWaitReady(true),
	)
	require.NoError(t, err)

	sw.t = t

	consumer.Start()

	t.Log("Consumer is ready and wait message")

	<-consumer.Deadlock()

	consumer.Close()

	t.Logf("Consumer all logs: %s", logBuf.String())
}
