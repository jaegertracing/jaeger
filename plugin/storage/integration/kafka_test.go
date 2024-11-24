// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/cmd/ingester/app"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/builder"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/kafka/consumer"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const defaultLocalKafkaBroker = "127.0.0.1:9092"

type KafkaIntegrationTestSuite struct {
	StorageIntegration
}

func (s *KafkaIntegrationTestSuite) initialize(t *testing.T) {
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	const encoding = "json"
	const groupID = "kafka-integration-test"
	const clientID = "kafka-integration-test"
	// A new topic is generated per execution to avoid data overlap
	topic := "jaeger-kafka-integration-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)

	f := kafka.NewFactory()
	v, command := config.Viperize(f.AddFlags)
	err := command.ParseFlags([]string{
		"--kafka.producer.topic",
		topic,
		"--kafka.producer.brokers",
		defaultLocalKafkaBroker,
		"--kafka.producer.encoding",
		encoding,
	})
	require.NoError(t, err)
	f.InitFromViper(v, logger)
	err = f.Initialize(metrics.NullFactory, logger)
	require.NoError(t, err)

	spanWriter, err := f.CreateSpanWriter()
	require.NoError(t, err)

	v, command = config.Viperize(app.AddFlags)
	err = command.ParseFlags([]string{
		"--kafka.consumer.topic",
		topic,
		"--kafka.consumer.brokers",
		defaultLocalKafkaBroker,
		"--kafka.consumer.encoding",
		encoding,
		"--kafka.consumer.group-id",
		groupID,
		"--kafka.consumer.client-id",
		clientID,
		"--ingester.parallelism",
		"1000",
	})
	require.NoError(t, err)
	options := app.Options{
		Configuration: consumer.Configuration{
			InitialOffset: sarama.OffsetOldest,
		},
	}
	options.InitFromViper(v)
	traceStore := memory.NewStore()
	spanConsumer, err := builder.CreateConsumer(logger, metrics.NullFactory, traceStore, options)
	require.NoError(t, err)
	spanConsumer.Start()

	s.SpanWriter = spanWriter
	s.SpanReader = &ingester{traceStore}
	s.CleanUp = func(_ *testing.T) {}
	s.SkipArchiveTest = true
}

// The ingester consumes spans from kafka and writes them to an in-memory traceStore
type ingester struct {
	traceStore *memory.Store
}

func (r *ingester) GetTrace(ctx context.Context, query spanstore.GetTraceParameters) (*model.Trace, error) {
	return r.traceStore.GetTrace(ctx, query)
}

func (*ingester) GetServices(context.Context) ([]string, error) {
	return nil, nil
}

func (*ingester) GetOperations(
	context.Context,
	spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	return nil, nil
}

func (*ingester) FindTraces(context.Context, *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	return nil, nil
}

func (*ingester) FindTraceIDs(context.Context, *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	return nil, nil
}

func TestKafkaStorage(t *testing.T) {
	SkipUnlessEnv(t, "kafka")
	s := &KafkaIntegrationTestSuite{}
	s.initialize(t)
	t.Run("GetTrace", s.testGetTrace)
}
