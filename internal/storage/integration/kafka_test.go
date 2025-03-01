// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/ingester/app"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/builder"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/kafka"
	"github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/kafka/consumer"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

const (
	defaultLocalKafkaBroker                 = "127.0.0.1:9092"
	defaultLocalKafkaBrokerSASLSSLPlaintext = "127.0.0.1:29093"
	defaultLocalKafkaBrokerSASLPlaintext    = "127.0.0.1:9095"
)

func getEnv(key string, defaultValue string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaultValue
}

type KafkaIntegrationTestSuite struct {
	StorageIntegration
}

func (s *KafkaIntegrationTestSuite) initializeWithSASLPlaintext(t *testing.T) {
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	const encoding = "json"
	const groupID = "kafka-sasl-plaintext-integration-test"
	const clientID = "kafka-sasl-plaintext-integration-test"
	broker := getEnv("KAFKA_BROKER", defaultLocalKafkaBrokerSASLPlaintext)
	username := getEnv("KAFKA_USERNAME", "admin")
	password := getEnv("KAFKA_PASSWORD", "admin-secret")
	// A new topic is generated per execution to avoid data overlap
	f := kafka.NewFactory()
	topic := "jaeger-kafka-sasl-plaintext-integration-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	v, command := config.Viperize(f.AddFlags)
	err := command.ParseFlags([]string{
		"--kafka.producer.topic",
		topic,
		"--kafka.producer.brokers",
		broker,
		"--kafka.producer.encoding",
		encoding,
		"--kafka.producer.authentication",
		"plaintext",
		"--kafka.producer.plaintext.username",
		username,
		"--kafka.producer.plaintext.password",
		password,
		"--kafka.producer.plaintext.mechanism",
		"PLAIN",
	})
	require.NoError(t, err)
	f.InitFromViper(v, logger)
	err = f.Initialize(metrics.NullFactory, logger)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, f.Close())
	})
	spanWriter, err := f.CreateSpanWriter()
	require.NoError(t, err)
	v, command = config.Viperize(app.AddFlags)
	err = command.ParseFlags([]string{
		"--kafka.consumer.topic",
		topic,
		"--kafka.consumer.brokers",
		broker,
		"--kafka.consumer.encoding",
		encoding,
		"--kafka.consumer.group-id",
		groupID,
		"--kafka.consumer.client-id",
		clientID,
		"--kafka.consumer.authentication",
		"plaintext",
		"--kafka.consumer.plaintext.username",
		username,
		"--kafka.consumer.plaintext.password",
		password,
		"--kafka.consumer.plaintext.mechanism",
		"PLAIN",
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
	t.Cleanup(func() {
		assert.NoError(t, spanConsumer.Close())
	})
	spanConsumer.Start()

	spanReader := &ingester{traceStore}
	s.TraceReader = v1adapter.NewTraceReader(spanReader)
	s.TraceWriter = v1adapter.NewTraceWriter(spanWriter)
	s.CleanUp = func(_ *testing.T) {}
}

func (s *KafkaIntegrationTestSuite) initializeWithSASLSSLPlaintext(t *testing.T) {
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	const encoding = "json"
	const groupID = "kafka-sasl-ssl-plaintext-integration-test"
	const clientID = "kafka-sasl-ssl-plaintext-integration-test"
	broker := getEnv("KAFKA_BROKER", defaultLocalKafkaBrokerSASLSSLPlaintext)
	username := getEnv("KAFKA_USERNAME", "admin")
	password := getEnv("KAFKA_PASSWORD", "admin-secret")
	// A new topic is generated per execution to avoid data overlap
	f := kafka.NewFactory()
	topic := "jaeger-kafka-sasl-ssl-plaintext-integration-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	v, command := config.Viperize(f.AddFlags)
	err := command.ParseFlags([]string{
		"--kafka.producer.topic",
		topic,
		"--kafka.producer.brokers",
		broker,
		"--kafka.producer.encoding",
		encoding,
		"--kafka.producer.authentication",
		"plaintext",
		"--kafka.producer.plaintext.username",
		username,
		"--kafka.producer.plaintext.password",
		password,
		"--kafka.producer.plaintext.mechanism",
		"PLAIN",
		"--kafka.producer.tls.enabled",
		"true",
		"--kafka.producer.tls.ca",
		"/Users/amolverma/contri/jaeger/certs/ca.pem",
	})
	require.NoError(t, err)
	f.InitFromViper(v, logger)
	err = f.Initialize(metrics.NullFactory, logger)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, f.Close())
	})
	spanWriter, err := f.CreateSpanWriter()
	require.NoError(t, err)
	v, command = config.Viperize(app.AddFlags)
	err = command.ParseFlags([]string{
		"--kafka.consumer.topic",
		topic,
		"--kafka.consumer.brokers",
		broker,
		"--kafka.consumer.encoding",
		encoding,
		"--kafka.consumer.group-id",
		groupID,
		"--kafka.consumer.client-id",
		clientID,
		"--kafka.consumer.authentication",
		"plaintext",
		"--kafka.consumer.plaintext.username",
		username,
		"--kafka.consumer.plaintext.password",
		password,
		"--kafka.consumer.plaintext.mechanism",
		"PLAIN",
		"--kafka.consumer.tls.enabled",
		"true",
		"--kafka.consumer.tls.ca",
		"/Users/amolverma/contri/jaeger/certs/ca.pem",
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
	t.Cleanup(func() {
		assert.NoError(t, spanConsumer.Close())
	})
	spanConsumer.Start()

	spanReader := &ingester{traceStore}
	s.TraceReader = v1adapter.NewTraceReader(spanReader)
	s.TraceWriter = v1adapter.NewTraceWriter(spanWriter)
	s.CleanUp = func(_ *testing.T) {}
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
	t.Cleanup(func() {
		assert.NoError(t, f.Close())
	})
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
	t.Cleanup(func() {
		assert.NoError(t, spanConsumer.Close())
	})
	spanConsumer.Start()

	spanReader := &ingester{traceStore}
	s.TraceReader = v1adapter.NewTraceReader(spanReader)
	s.TraceWriter = v1adapter.NewTraceWriter(spanWriter)
	s.CleanUp = func(_ *testing.T) {}
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
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	s := &KafkaIntegrationTestSuite{}
	s.initialize(t)
	t.Run("GetTrace", s.testGetTrace)
}

func TestKafkaStorageWithSASLSSLPlaintext(t *testing.T) {
	SkipUnlessEnv(t, "kafka")
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	s := &KafkaIntegrationTestSuite{}
	s.initializeWithSASLSSLPlaintext(t)
	t.Run("GetTrace", s.testGetTrace)
}

func TestKafkaStorageWithSASLPlaintext(t *testing.T) {
	SkipUnlessEnv(t, "kafka")
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	s := &KafkaIntegrationTestSuite{}
	s.initializeWithSASLPlaintext(t)
	t.Run("GetTrace", s.testGetTrace)
}
