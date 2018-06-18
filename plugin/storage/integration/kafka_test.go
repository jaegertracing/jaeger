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
	"bytes"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
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
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{"--kafka.topic=" + topic, "--kafka.brokers=" + defaultLocalKafkaBroker})
	f.InitFromViper(v)
	if err := f.Initialize(metrics.NullFactory, s.logger); err != nil {
		return err
	}
	spanWriter, err := f.CreateSpanWriter()
	if err != nil {
		return err
	}
	spanReader, err := createSpanReader(topic)
	if err != nil {
		return err
	}
	s.SpanWriter = spanWriter
	s.SpanReader = spanReader
	s.Refresh = func() error { return nil }
	s.CleanUp = func() error { return nil }
	return nil
}

type spanReader struct {
	logger   *zap.Logger
	topic    string
	consumer sarama.PartitionConsumer
}

func createSpanReader(topic string) (spanstore.Reader, error) {
	logger, _ := testutils.NewLogger()
	c, err := sarama.NewConsumer([]string{defaultLocalKafkaBroker}, nil)
	if err != nil {
		return nil, err
	}
	pc, err := c.ConsumePartition(topic, 0, sarama.OffsetOldest)
	if err != nil {
		return nil, err
	}
	return &spanReader{
		consumer: pc,
		topic:    topic,
		logger:   logger,
	}, nil
}

func (r *spanReader) GetTrace(traceID model.TraceID) (*model.Trace, error) {
	result := &model.Trace{}
	var err error
	doneCh := make(chan struct{})
	go func() {
		for {
			select {
			case msg := <-r.consumer.Messages():
				newSpan := model.Span{}
				if err = jsonpb.Unmarshal(bytes.NewReader(msg.Value), &newSpan); err != nil {
					r.logger.Error("protobuf unmarshaling error", zap.Error(err))
				}
				if newSpan.TraceID == traceID {
					result.Spans = append(result.Spans, &newSpan)
				}
			case <-doneCh:
				return
			}
		}
	}()
	time.Sleep(100 * time.Millisecond)
	doneCh <- struct{}{}
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *spanReader) GetServices() ([]string, error) {
	return nil, nil
}

func (r *spanReader) GetOperations(service string) ([]string, error) {
	return nil, nil
}

func (r *spanReader) FindTraces(query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
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
