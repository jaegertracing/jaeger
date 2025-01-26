// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"expvar"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
)

var _ (io.Closer) = (*Collector)(nil)

func optionsForEphemeralPorts() *flags.CollectorOptions {
	collectorOpts := &flags.CollectorOptions{
		HTTP: confighttp.ServerConfig{
			Endpoint:   ":0",
			TLSSetting: &configtls.ServerConfig{},
		},
		GRPC: configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  ":0",
				Transport: confignet.TransportTypeTCP,
			},
			Keepalive: &configgrpc.KeepaliveServerConfig{
				ServerParameters: &configgrpc.KeepaliveServerParameters{
					MaxConnectionIdle: 10,
				},
			},
		},
		OTLP: struct {
			Enabled bool
			GRPC    configgrpc.ServerConfig
			HTTP    confighttp.ServerConfig
		}{
			Enabled: true,
			HTTP: confighttp.ServerConfig{
				Endpoint:   ":0",
				TLSSetting: &configtls.ServerConfig{},
			},
			GRPC: configgrpc.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  ":0",
					Transport: confignet.TransportTypeTCP,
				},
				Keepalive: &configgrpc.KeepaliveServerConfig{
					ServerParameters: &configgrpc.KeepaliveServerParameters{
						MaxConnectionIdle: 10,
					},
				},
			},
		},
		Zipkin: struct {
			confighttp.ServerConfig
			KeepAlive bool
		}{
			ServerConfig: confighttp.ServerConfig{
				Endpoint: ":0",
			},
		},
		Tenancy: tenancy.Options{},
	}
	return collectorOpts
}

type mockAggregator struct {
	callCount  atomic.Int32
	closeCount atomic.Int32
}

func (t *mockAggregator) RecordThroughput(string /* service */, string /* operation */, model.SamplerType, float64 /* probability */) {
	t.callCount.Add(1)
}

func (t *mockAggregator) HandleRootSpan(*model.Span) {
	t.callCount.Add(1)
}

func (*mockAggregator) Start() {}

func (t *mockAggregator) Close() error {
	t.closeCount.Add(1)
	return nil
}

func TestNewCollector(t *testing.T) {
	// prepare
	hc := healthcheck.New()
	logger := zap.NewNop()
	baseMetrics := metricstest.NewFactory(time.Hour)
	defer baseMetrics.Backend.Stop()
	spanWriter := &fakeSpanWriter{}
	samplingProvider := &mockSamplingProvider{}
	tm := &tenancy.Manager{}

	c := New(&CollectorParams{
		ServiceName:      "collector",
		Logger:           logger,
		MetricsFactory:   baseMetrics,
		TraceWriter:      v1adapter.NewTraceWriter(spanWriter),
		SamplingProvider: samplingProvider,
		HealthCheck:      hc,
		TenancyMgr:       tm,
	})

	collectorOpts := optionsForEphemeralPorts()
	require.NoError(t, c.Start(collectorOpts))
	assert.NotNil(t, c.SpanHandlers())
	require.NoError(t, c.Close())
}

func TestCollector_StartErrors(t *testing.T) {
	run := func(name string, options *flags.CollectorOptions, expErr string) {
		t.Run(name, func(t *testing.T) {
			hc := healthcheck.New()
			logger := zap.NewNop()
			baseMetrics := metricstest.NewFactory(time.Hour)
			defer baseMetrics.Backend.Stop()
			spanWriter := &fakeSpanWriter{}
			samplingProvider := &mockSamplingProvider{}
			tm := &tenancy.Manager{}

			c := New(&CollectorParams{
				ServiceName:      "collector",
				Logger:           logger,
				MetricsFactory:   baseMetrics,
				TraceWriter:      v1adapter.NewTraceWriter(spanWriter),
				SamplingProvider: samplingProvider,
				HealthCheck:      hc,
				TenancyMgr:       tm,
			})
			err := c.Start(options)
			require.ErrorContains(t, err, expErr)
			require.NoError(t, c.Close())
		})
	}

	var options *flags.CollectorOptions

	options = optionsForEphemeralPorts()
	options.GRPC.NetAddr.Endpoint = ":-1"
	run("gRPC", options, "could not start gRPC server")

	options = optionsForEphemeralPorts()
	options.HTTP.Endpoint = ":-1"
	run("HTTP", options, "could not start HTTP server")

	options = optionsForEphemeralPorts()
	options.Zipkin.Endpoint = ":-1"
	run("Zipkin", options, "could not start Zipkin receiver")

	options = optionsForEphemeralPorts()
	options.OTLP.GRPC.NetAddr.Endpoint = ":-1"
	run("OTLP/GRPC", options, "could not start OTLP receiver")

	options = optionsForEphemeralPorts()
	options.OTLP.HTTP.Endpoint = ":-1"
	run("OTLP/HTTP", options, "could not start OTLP receiver")
}

type mockSamplingProvider struct{}

func (*mockSamplingProvider) GetSamplingStrategy(context.Context, string /* serviceName */) (*api_v2.SamplingStrategyResponse, error) {
	return &api_v2.SamplingStrategyResponse{}, nil
}

func (*mockSamplingProvider) Close() error {
	return nil
}

func TestCollector_PublishOpts(t *testing.T) {
	// prepare
	hc := healthcheck.New()
	logger := zap.NewNop()
	metricsFactory := metricstest.NewFactory(time.Second)
	defer metricsFactory.Backend.Stop()
	spanWriter := &fakeSpanWriter{}
	samplingProvider := &mockSamplingProvider{}
	tm := &tenancy.Manager{}

	c := New(&CollectorParams{
		ServiceName:      "collector",
		Logger:           logger,
		MetricsFactory:   metricsFactory,
		TraceWriter:      v1adapter.NewTraceWriter(spanWriter),
		SamplingProvider: samplingProvider,
		HealthCheck:      hc,
		TenancyMgr:       tm,
	})
	collectorOpts := optionsForEphemeralPorts()
	collectorOpts.NumWorkers = 24
	collectorOpts.QueueSize = 42

	require.NoError(t, c.Start(collectorOpts))
	defer c.Close()
	c.publishOpts(collectorOpts)
	assert.EqualValues(t, 24, expvar.Get(metricNumWorkers).(*expvar.Int).Value())
	assert.EqualValues(t, 42, expvar.Get(metricQueueSize).(*expvar.Int).Value())
}

func TestAggregator(t *testing.T) {
	// prepare
	hc := healthcheck.New()
	logger := zap.NewNop()
	baseMetrics := metricstest.NewFactory(time.Hour)
	defer baseMetrics.Backend.Stop()
	spanWriter := &fakeSpanWriter{}
	samplingProvider := &mockSamplingProvider{}
	agg := &mockAggregator{}
	tm := &tenancy.Manager{}

	c := New(&CollectorParams{
		ServiceName:        "collector",
		Logger:             logger,
		MetricsFactory:     baseMetrics,
		TraceWriter:        v1adapter.NewTraceWriter(spanWriter),
		SamplingProvider:   samplingProvider,
		HealthCheck:        hc,
		SamplingAggregator: agg,
		TenancyMgr:         tm,
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
	_, err := c.spanProcessor.ProcessSpans(context.Background(), processor.SpansV1{
		Spans: spans,
		Details: processor.Details{
			SpanFormat: processor.JaegerSpanFormat,
		},
	})
	require.NoError(t, err)
	require.NoError(t, c.Close())

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
