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

package integration

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/ingester/app"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/builder"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const defaultLocalKafkaBroker = "127.0.0.1:9092"

type KafkaIntegrationTestSuite struct {
	StorageIntegration
	logger *zap.Logger
}

func (s *KafkaIntegrationTestSuite) initialize() error {
	s.logger, _ = testutils.NewLogger()
	const encoding = "json"
	const groupID = "kafka-integration-test"
	const ClientID = "kafka-integration-test"
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
	if err != nil {
		return err
	}
	f.InitFromViper(v)
	if err := f.Initialize(metrics.NullFactory, s.logger); err != nil {
		return err
	}
	spanWriter, err := f.CreateSpanWriter()
	if err != nil {
		return err
	}

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
		ClientID,
		"--ingester.parallelism",
		"1000",
	})
	if err != nil {
		return err
	}
	options := app.Options{}
	options.InitFromViper(v)
	traceStore := memory.NewStore()
	spanConsumer, err := builder.CreateConsumer(s.logger, metrics.NullFactory, traceStore, options)
	if err != nil {
		return err
	}
	spanConsumer.Start()

	s.SpanWriter = spanWriter
	s.SpanReader = &ingester{traceStore}
	s.Refresh = func() error { return nil }
	s.CleanUp = func() error { return nil }
	return nil
}

// The ingester consumes spans from kafka and writes them to an in-memory traceStore
type ingester struct {
	traceStore *memory.Store
}

func (r *ingester) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	return r.traceStore.GetTrace(ctx, traceID)
}

func (r *ingester) GetServices(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (r *ingester) GetOperations(ctx context.Context, service string) ([]string, error) {
	return nil, nil
}

func (r *ingester) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	return nil, nil
}

func (r *ingester) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	return nil, nil
}

func TestKafkaStorage(t *testing.T) {
	if os.Getenv("STORAGE") != "kafka" {
		t.Skip("Integration test against kafka skipped; set STORAGE env var to kafka to run this")
	}
	s := &KafkaIntegrationTestSuite{}
	require.NoError(t, s.initialize())
	t.Run("GetTrace", s.testGetTrace)
}
