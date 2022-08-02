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
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/internal/metrics/fork"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

var _ (io.Closer) = (*Collector)(nil)

func optionsForEphemeralPorts() *flags.CollectorOptions {
	collectorOpts := &flags.CollectorOptions{}
	collectorOpts.GRPC.HostPort = ":0"
	collectorOpts.HTTP.HostPort = ":0"
	collectorOpts.OTLP.Enabled = true
	collectorOpts.OTLP.GRPC.HostPort = ":0"
	collectorOpts.OTLP.HTTP.HostPort = ":0"
	collectorOpts.Zipkin.HTTPHostPort = ":0"
	return collectorOpts
}

func TestNewCollector(t *testing.T) {
	// prepare
	hc := healthcheck.New()
	logger := zap.NewNop()
	baseMetrics := metricstest.NewFactory(time.Hour)
	spanWriter := &fakeSpanWriter{}
	strategyStore := &mockStrategyStore{}
	tm := &tenancy.Manager{}

	c := New(&CollectorParams{
		ServiceName:    "collector",
		Logger:         logger,
		MetricsFactory: baseMetrics,
		SpanWriter:     spanWriter,
		StrategyStore:  strategyStore,
		HealthCheck:    hc,
		TenancyMgr:     tm,
	})

	collectorOpts := optionsForEphemeralPorts()
	require.NoError(t, c.Start(collectorOpts))
	assert.NotNil(t, c.SpanHandlers())
	assert.NoError(t, c.Close())
}

func TestCollector_StartErrors(t *testing.T) {
	run := func(name string, options *flags.CollectorOptions, expErr string) {
		t.Run(name, func(t *testing.T) {
			hc := healthcheck.New()
			logger := zap.NewNop()
			baseMetrics := metricstest.NewFactory(time.Hour)
			spanWriter := &fakeSpanWriter{}
			strategyStore := &mockStrategyStore{}
			tm := &tenancy.Manager{}

			c := New(&CollectorParams{
				ServiceName:    "collector",
				Logger:         logger,
				MetricsFactory: baseMetrics,
				SpanWriter:     spanWriter,
				StrategyStore:  strategyStore,
				HealthCheck:    hc,
				TenancyMgr:     tm,
			})
			err := c.Start(options)
			require.Error(t, err)
			assert.Contains(t, err.Error(), expErr)
		})
	}

	var options *flags.CollectorOptions

	options = optionsForEphemeralPorts()
	options.GRPC.HostPort = ":-1"
	run("gRPC", options, "could not start gRPC server")

	options = optionsForEphemeralPorts()
	options.HTTP.HostPort = ":-1"
	run("HTTP", options, "could not start HTTP server")

	options = optionsForEphemeralPorts()
	options.Zipkin.HTTPHostPort = ":-1"
	run("Zipkin", options, "could not start Zipkin server")

	options = optionsForEphemeralPorts()
	options.OTLP.GRPC.HostPort = ":-1"
	run("OTLP/GRPC", options, "could not start OTLP receiver")

	options = optionsForEphemeralPorts()
	options.OTLP.HTTP.HostPort = ":-1"
	run("OTLP/HTTP", options, "could not start OTLP receiver")
}

type mockStrategyStore struct{}

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
	tm := &tenancy.Manager{}

	c := New(&CollectorParams{
		ServiceName:    "collector",
		Logger:         logger,
		MetricsFactory: metricsFactory,
		SpanWriter:     spanWriter,
		StrategyStore:  strategyStore,
		HealthCheck:    hc,
		TenancyMgr:     tm,
	})
	collectorOpts := optionsForEphemeralPorts()
	collectorOpts.NumWorkers = 24
	collectorOpts.QueueSize = 42

	require.NoError(t, c.Start(collectorOpts))
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
	tm := &tenancy.Manager{}

	c := New(&CollectorParams{
		ServiceName:    "collector",
		Logger:         logger,
		MetricsFactory: baseMetrics,
		SpanWriter:     spanWriter,
		StrategyStore:  strategyStore,
		HealthCheck:    hc,
		Aggregator:     agg,
		TenancyMgr:     tm,
	})
	collectorOpts := optionsForEphemeralPorts()
	collectorOpts.NumWorkers = 10
	collectorOpts.QueueSize = 10
	require.NoError(t, c.Start(collectorOpts))

	// assert that aggregator was added to the collector
	spans := []*model.Span{
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
	}
	_, err := c.spanProcessor.ProcessSpans(spans, processor.SpansOptions{SpanFormat: processor.JaegerSpanFormat})
	assert.NoError(t, err)
	assert.NoError(t, c.Close())

	// spans are processed by background workers, so we may need to wait
	for i := 0; i < 1000; i++ {
		if agg.callCount.Load() == 1 && agg.closeCount.Load() == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.EqualValues(t, 1, agg.callCount.Load(), "aggregator was used")
	assert.EqualValues(t, 1, agg.closeCount.Load(), "aggregator close was called")
}
