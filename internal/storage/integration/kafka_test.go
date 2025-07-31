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
	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/kafka/consumer"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/kafka"
	"github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

const (
	defaultLocalKafkaBroker                 = "127.0.0.1:9092"
	defaultLocalKafkaBrokerSASLSSLPlaintext = "127.0.0.1:9093"
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

type kafkaConfig struct {
	brokerDefault  string
	testNameSuffix string
	groupIDSuffix  string
	clientIDSuffix string
	authType       string
	username       string
	password       string
	mechanism      string
	tlsEnabled     bool
	caCertPath     string
	skipHostVerify bool
}

func (s *KafkaIntegrationTestSuite) initializeWithConfig(t *testing.T, cfg kafkaConfig) {
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	const encoding = "json"
	var groupID, clientID string
	if cfg.groupIDSuffix != "" {
		groupID = "kafka-" + cfg.groupIDSuffix + "-integration-test"
		clientID = "kafka-" + cfg.clientIDSuffix + "-integration-test"
	} else {
		groupID = "kafka-integration-test"
		clientID = "kafka-integration-test"
	}
	broker := getEnv("KAFKA_BROKER", cfg.brokerDefault)
	username := getEnv("KAFKA_USERNAME", cfg.username)
	password := getEnv("KAFKA_PASSWORD", cfg.password)

	// A new topic is generated per execution to avoid data overlap
	var topic string
	if cfg.testNameSuffix != "" {
		topic = "jaeger-kafka-" + cfg.testNameSuffix + "-integration-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	} else {
		topic = "jaeger-kafka-integration-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}

	// Build producer flags
	producerFlags := []string{
		"--kafka.producer.topic", topic,
		"--kafka.producer.brokers", broker,
		"--kafka.producer.encoding", encoding,
	}

	if cfg.authType != "" {
		producerFlags = append(producerFlags,
			"--kafka.producer.authentication", cfg.authType,
			"--kafka.producer.plaintext.username", username,
			"--kafka.producer.plaintext.password", password,
			"--kafka.producer.plaintext.mechanism", cfg.mechanism,
		)
	}

	if cfg.tlsEnabled {
		producerFlags = append(producerFlags,
			"--kafka.producer.tls.enabled", "true",
			"--kafka.producer.tls.ca", cfg.caCertPath,
			"--kafka.producer.tls.skip-host-verify", strconv.FormatBool(cfg.skipHostVerify),
		)
	}

	// Setup producer
	f := kafka.NewFactory()
	v, command := config.Viperize(f.AddFlags)
	err := command.ParseFlags(producerFlags)
	require.NoError(t, err)
	f.InitFromViper(v, logger)
	err = f.Initialize(metrics.NullFactory, logger)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, f.Close())
	})
	spanWriter, err := f.CreateSpanWriter()
	require.NoError(t, err)

	// Build consumer flags
	consumerFlags := []string{
		"--kafka.consumer.topic", topic,
		"--kafka.consumer.brokers", broker,
		"--kafka.consumer.encoding", encoding,
		"--kafka.consumer.group-id", groupID,
		"--kafka.consumer.client-id", clientID,
		"--ingester.parallelism", "1000",
	}

	if cfg.authType != "" {
		consumerFlags = append(consumerFlags,
			"--kafka.consumer.authentication", cfg.authType,
			"--kafka.consumer.plaintext.username", username,
			"--kafka.consumer.plaintext.password", password,
			"--kafka.consumer.plaintext.mechanism", cfg.mechanism,
		)
	}

	if cfg.tlsEnabled {
		consumerFlags = append(consumerFlags,
			"--kafka.consumer.tls.enabled", "true",
			"--kafka.consumer.tls.ca", cfg.caCertPath,
			"--kafka.consumer.tls.skip-host-verify", strconv.FormatBool(cfg.skipHostVerify),
		)
	}

	// Setup consumer
	v, command = config.Viperize(app.AddFlags)
	err = command.ParseFlags(consumerFlags)
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

func (s *KafkaIntegrationTestSuite) initializeWithSASLPlaintext(t *testing.T) {
	s.initializeWithConfig(t, kafkaConfig{
		brokerDefault:  defaultLocalKafkaBrokerSASLPlaintext,
		testNameSuffix: "sasl-plaintext",
		groupIDSuffix:  "sasl-plaintext",
		clientIDSuffix: "sasl-plaintext",
		authType:       "plaintext",
		username:       "admin",
		password:       "admin-secret",
		mechanism:      "PLAIN",
		tlsEnabled:     false,
	})
}

func (s *KafkaIntegrationTestSuite) initializeWithSASLSSLPlaintext(t *testing.T) {
	s.initializeWithConfig(t, kafkaConfig{
		brokerDefault:  defaultLocalKafkaBrokerSASLSSLPlaintext,
		testNameSuffix: "sasl-ssl-plaintext",
		groupIDSuffix:  "sasl-ssl-plaintext",
		clientIDSuffix: "sasl-ssl-plaintext",
		authType:       "plaintext",
		username:       "admin",
		password:       "admin-secret",
		mechanism:      "PLAIN",
		tlsEnabled:     true,
		caCertPath:     "../../../internal/config/tlscfg/testdata/example-CA-cert.pem",
		skipHostVerify: true,
	})
}

func (s *KafkaIntegrationTestSuite) initialize(t *testing.T) {
	s.initializeWithConfig(t, kafkaConfig{
		brokerDefault:  defaultLocalKafkaBroker,
		testNameSuffix: "",
		groupIDSuffix:  "",
		clientIDSuffix: "",
		authType:       "",
		tlsEnabled:     false,
	})
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
