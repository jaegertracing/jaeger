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
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"

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
}

func (s *KafkaIntegrationTestSuite) initialize() error {
	logger, _ := testutils.NewLogger()
	s.logger = logger
	// A new topic is generated per execution to avoid data overlap
	topic := "jaeger-kafka-integration-test-" + strconv.FormatInt(time.Now().UnixNano(), 10)

	f := kafka.NewFactory()
	v, command := config.Viperize(f.AddFlags, app.AddFlags)
	command.ParseFlags([]string{
		"--kafka.topic",
		topic,
		"--kafka.brokers",
		defaultLocalKafkaBroker,
		"--kafka.encoding",
		"json",
		"--ingester.brokers",
		defaultLocalKafkaBroker,
		"--ingester.topic",
		topic,
		"--ingester.group-id",
		"kafka-integration-test",
		"--ingester.parallelism",
		"1000",
		"--ingester.encoding",
		"json",
	})
	f.InitFromViper(v)
	if err := f.Initialize(metrics.NullFactory, s.logger); err != nil {
		return err
	}
	spanWriter, err := f.CreateSpanWriter()
	if err != nil {
		return err
	}

	options := app.Options{}
	options.InitFromViper(v)
	traceStore := memory.NewStore()
	spanConsumer, err := builder.CreateConsumer(logger, metrics.NullFactory, traceStore, options)
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

func (r *ingester) GetTrace(traceID model.TraceID) (*model.Trace, error) {
	return r.traceStore.GetTrace(traceID)
}

func (r *ingester) GetServices() ([]string, error) {
	return nil, nil
}

func (r *ingester) GetOperations(service string) ([]string, error) {
	return nil, nil
}

func (r *ingester) FindTraces(query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
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
