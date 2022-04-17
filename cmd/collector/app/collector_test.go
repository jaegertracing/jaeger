// Copyright (c) 2020 The Jaeger Authors.
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

package app

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics/fork"
	"github.com/uber/jaeger-lib/metrics/metricstest"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

var _ (io.Closer) = (*Collector)(nil)

func TestNewCollector(t *testing.T) {
	// prepare
	hc := healthcheck.New()
	logger := zap.NewNop()
	baseMetrics := metricstest.NewFactory(time.Hour)
	spanWriter := &fakeSpanWriter{}
	strategyStore := &mockStrategyStore{}

	c := New(&CollectorParams{
		ServiceName:    "collector",
		Logger:         logger,
		MetricsFactory: baseMetrics,
		SpanWriter:     spanWriter,
		StrategyStore:  strategyStore,
		HealthCheck:    hc,
	})
	collectorOpts := &CollectorOptions{}

	// test
	c.Start(collectorOpts)

	// verify
	assert.NoError(t, c.Close())
}

type mockStrategyStore struct {
}

func (m *mockStrategyStore) GetSamplingStrategy(_ context.Context, serviceName string) (*sampling.SamplingStrategyResponse, error) {
	return &sampling.SamplingStrategyResponse{}, nil
}

func TestCollector_PublishOpts(t *testing.T) {
	// prepare
	hc := healthcheck.New()
	logger := zap.NewNop()
	baseMetrics := metricstest.NewFactory(time.Second)
	forkFactory := metricstest.NewFactory(time.Second)
	metricsFactory := fork.New("internal", forkFactory, baseMetrics)
	spanWriter := &fakeSpanWriter{}
	strategyStore := &mockStrategyStore{}

	c := New(&CollectorParams{
		ServiceName:    "collector",
		Logger:         logger,
		MetricsFactory: metricsFactory,
		SpanWriter:     spanWriter,
		StrategyStore:  strategyStore,
		HealthCheck:    hc,
	})
	collectorOpts := &CollectorOptions{
		NumWorkers: 24,
		QueueSize:  42,
	}

	c.Start(collectorOpts)
	defer c.Close()

	forkFactory.AssertGaugeMetrics(t, metricstest.ExpectedMetric{
		Name:  "internal.collector.num-workers",
		Value: 24,
	})
	forkFactory.AssertGaugeMetrics(t, metricstest.ExpectedMetric{
		Name:  "internal.collector.queue-size",
		Value: 42,
	})
}

func TestAggregator(t *testing.T) {
	// prepare
	hc := healthcheck.New()
	logger := zap.NewNop()
	baseMetrics := metricstest.NewFactory(time.Hour)
	spanWriter := &fakeSpanWriter{}
	strategyStore := &mockStrategyStore{}
	agg := &mockAggregator{}

	c := New(&CollectorParams{
		ServiceName:    "collector",
		Logger:         logger,
		MetricsFactory: baseMetrics,
		SpanWriter:     spanWriter,
		StrategyStore:  strategyStore,
		HealthCheck:    hc,
		Aggregator:     agg,
	})
	collectorOpts := &CollectorOptions{
		QueueSize:  10,
		NumWorkers: 10,
	}

	// test
	c.Start(collectorOpts)

	// assert that aggregator was added to the collector
	_, err := c.spanProcessor.ProcessSpans([]*model.Span{
		{
			OperationName: "y",
			Process: &model.Process{
				ServiceName: "x",
			},
			Tags: []model.KeyValue{
				{
					Key:  "sampler.type",
					VStr: "probabilistic",
				},
				{
					Key:  "sampler.param",
					VStr: "1",
				},
			},
		},
	}, processor.SpansOptions{SpanFormat: processor.JaegerSpanFormat})
	assert.NoError(t, err)

	// verify
	assert.NoError(t, c.Close())

	// assert that aggregator was used
	assert.Equal(t, 1, agg.callCount)

	// assert that aggregator close was called
	assert.Equal(t, 1, agg.closeCount)
}
