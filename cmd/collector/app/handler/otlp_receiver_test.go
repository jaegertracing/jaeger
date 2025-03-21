// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pipeline"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func optionsWithPorts(port string) *flags.CollectorOptions {
	opts := &flags.CollectorOptions{
		OTLP: struct {
			Enabled bool
			GRPC    configgrpc.ServerConfig
			HTTP    confighttp.ServerConfig
		}{
			Enabled: true,
			HTTP: confighttp.ServerConfig{
				Endpoint: port,
			},
			GRPC: configgrpc.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  port,
					Transport: confignet.TransportTypeTCP,
				},
			},
		},
	}
	return opts
}

func TestStartOtlpReceiver(t *testing.T) {
	spanProcessor := &mockSpanProcessor{}
	logger := zaptest.NewLogger(t)
	tm := &tenancy.Manager{}
	rec, err := StartOTLPReceiver(optionsWithPorts(":0"), logger, spanProcessor, tm)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, rec.Shutdown(context.Background()))
	}()

	// Ideally, we want to test with a real gRPC client, but OTEL repos only have those as internal packages.
	// So we will rely on otlpreceiver being tested in the OTEL repos, and we only test the consumer function.
}

func makeTracesOneSpan() ptrace.Traces {
	traces := ptrace.NewTraces()
	rSpans := traces.ResourceSpans().AppendEmpty()
	sSpans := rSpans.ScopeSpans().AppendEmpty()
	span := sSpans.Spans().AppendEmpty()
	span.SetName("test")
	return traces
}

func TestStartOtlpReceiver_Error(t *testing.T) {
	spanProcessor := &mockSpanProcessor{}
	logger := zaptest.NewLogger(t)
	opts := optionsWithPorts(":-1")
	tm := &tenancy.Manager{}
	_, err := StartOTLPReceiver(opts, logger, spanProcessor, tm)
	require.ErrorContains(t, err, "could not start the OTLP receiver")

	newTraces := func(consumer.ConsumeTracesFunc, ...consumer.Option) (consumer.Traces, error) {
		return nil, errors.New("mock error")
	}
	f := otlpreceiver.NewFactory()
	_, err = startOTLPReceiver(opts, logger, spanProcessor, &tenancy.Manager{}, f, newTraces, f.CreateTraces)
	require.ErrorContains(t, err, "could not create the OTLP consumer")

	createTracesReceiver := func(
		context.Context, receiver.Settings, component.Config, consumer.Traces,
	) (receiver.Traces, error) {
		return nil, errors.New("mock error")
	}
	_, err = startOTLPReceiver(opts, logger, spanProcessor, &tenancy.Manager{}, f, consumer.NewTraces, createTracesReceiver)
	assert.ErrorContains(t, err, "could not create the OTLP receiver")
}

func TestOtelHost_ReportFatalError(t *testing.T) {
	logger, buf := testutils.NewLogger()
	host := &otelHost{logger: logger}

	defer func() {
		_ = recover()
		assert.Contains(t, buf.String(), "mock error")
	}()
	host.ReportFatalError(errors.New("mock error"))
	t.Errorf("ReportFatalError did not panic")
}

func TestOtelHost(t *testing.T) {
	host := &otelHost{}
	assert.Nil(t, host.GetFactory(component.KindReceiver, pipeline.SignalTraces))
	assert.Nil(t, host.GetExtensions())
	assert.Nil(t, host.GetExporters())
}

func TestConsumerHelper(t *testing.T) {
	spanProcessor := &mockSpanProcessor{}
	consumerHelper := &consumerHelper{
		batchConsumer: newBatchConsumer(zaptest.NewLogger(t),
			spanProcessor,
			processor.UnknownTransport, // could be gRPC or HTTP
			processor.OTLPSpanFormat,
			&tenancy.Manager{}),
	}
	err := consumerHelper.consume(context.Background(), makeTracesOneSpan())
	require.NoError(t, err)
	assert.Eventually(t, func() bool {
		return len(spanProcessor.getTraces()) == 1
	}, time.Second, time.Millisecond, "spanProcessor should have received one span")
	assert.Empty(t, spanProcessor.getSpans())
}

func TestConsumerHelper_Consume_Error(t *testing.T) {
	consumerHelper := &consumerHelper{
		batchConsumer: newBatchConsumer(zaptest.NewLogger(t),
			&mockSpanProcessor{expectedError: assert.AnError},
			processor.UnknownTransport, // could be gRPC or HTTP
			processor.OTLPSpanFormat,
			&tenancy.Manager{}),
	}
	err := consumerHelper.consume(context.Background(), makeTracesOneSpan())
	require.ErrorIs(t, err, assert.AnError)
}

func TestConsumerHelper_Consume_TenantError(t *testing.T) {
	consumerHelper := &consumerHelper{
		batchConsumer: newBatchConsumer(zaptest.NewLogger(t),
			&mockSpanProcessor{},
			processor.UnknownTransport, // could be gRPC or HTTP
			processor.OTLPSpanFormat,
			&tenancy.Manager{Enabled: true}),
	}
	err := consumerHelper.consume(context.Background(), makeTracesOneSpan())
	require.ErrorContains(t, err, "missing tenant header")
}
