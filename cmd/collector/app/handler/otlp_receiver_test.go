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
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pipeline"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"

	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/config/corscfg"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func optionsWithPorts(port string) *flags.CollectorOptions {
	opts := &flags.CollectorOptions{}
	opts.OTLP.GRPC = flags.GRPCOptions{
		HostPort: port,
	}
	opts.OTLP.HTTP = flags.HTTPOptions{
		HostPort: port,
	}
	return opts
}

func TestStartOtlpReceiver(t *testing.T) {
	spanProcessor := &mockSpanProcessor{}
	logger, _ := testutils.NewLogger()
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

func TestConsumerDelegate(t *testing.T) {
	testCases := []struct {
		expectErr error
		expectLog string
	}{
		{}, // no errors
		{expectErr: errors.New("test-error"), expectLog: "test-error"},
	}
	for _, test := range testCases {
		t.Run(test.expectLog, func(t *testing.T) {
			logger, logBuf := testutils.NewLogger()
			spanProcessor := &mockSpanProcessor{expectedError: test.expectErr}
			consumer := newConsumerDelegate(logger, spanProcessor, &tenancy.Manager{})

			err := consumer.consume(context.Background(), makeTracesOneSpan())

			if test.expectErr != nil {
				require.Equal(t, test.expectErr, err)
				assert.Contains(t, logBuf.String(), test.expectLog)
			} else {
				require.NoError(t, err)
				assert.Len(t, spanProcessor.getSpans(), 1)
			}
		})
	}
}

func TestStartOtlpReceiver_Error(t *testing.T) {
	spanProcessor := &mockSpanProcessor{}
	logger, _ := testutils.NewLogger()
	opts := optionsWithPorts(":-1")
	tm := &tenancy.Manager{}
	_, err := StartOTLPReceiver(opts, logger, spanProcessor, tm)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not start the OTLP receiver")

	newTraces := func(consumer.ConsumeTracesFunc, ...consumer.Option) (consumer.Traces, error) {
		return nil, errors.New("mock error")
	}
	f := otlpreceiver.NewFactory()
	_, err = startOTLPReceiver(opts, logger, spanProcessor, &tenancy.Manager{}, f, newTraces, f.CreateTraces)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not create the OTLP consumer")

	createTracesReceiver := func(
		context.Context, receiver.Settings, component.Config, consumer.Traces,
	) (receiver.Traces, error) {
		return nil, errors.New("mock error")
	}
	_, err = startOTLPReceiver(opts, logger, spanProcessor, &tenancy.Manager{}, f, consumer.NewTraces, createTracesReceiver)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not create the OTLP receiver")
}

func TestProtoFromTracesError(t *testing.T) {
	mockErr := errors.New("mock error")
	c := &consumerDelegate{
		protoFromTraces: func(_ ptrace.Traces) ([]*model.Batch, error) {
			return nil, mockErr
		},
	}
	err := c.consume(context.Background(), ptrace.Traces{})
	assert.Equal(t, mockErr, err)
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

func TestApplyOTLPGRPCServerSettings(t *testing.T) {
	otlpFactory := otlpreceiver.NewFactory()
	otlpReceiverConfig := otlpFactory.CreateDefaultConfig().(*otlpreceiver.Config)

	grpcOpts := &flags.GRPCOptions{
		HostPort:                ":54321",
		MaxReceiveMessageLength: 42 * 1024 * 1024,
		MaxConnectionAge:        33 * time.Second,
		MaxConnectionAgeGrace:   37 * time.Second,
		TLS: tlscfg.Options{
			Enabled:        true,
			CAPath:         "ca",
			CertPath:       "cert",
			KeyPath:        "key",
			ClientCAPath:   "clientca",
			MinVersion:     "1.1",
			MaxVersion:     "1.3",
			ReloadInterval: 24 * time.Hour,
		},
	}
	applyGRPCSettings(otlpReceiverConfig.GRPC, grpcOpts)
	out := otlpReceiverConfig.GRPC
	assert.Equal(t, ":54321", out.NetAddr.Endpoint)
	assert.EqualValues(t, 42, out.MaxRecvMsgSizeMiB)
	require.NotNil(t, out.Keepalive)
	require.NotNil(t, out.Keepalive.ServerParameters)
	assert.Equal(t, 33*time.Second, out.Keepalive.ServerParameters.MaxConnectionAge)
	assert.Equal(t, 37*time.Second, out.Keepalive.ServerParameters.MaxConnectionAgeGrace)
	require.NotNil(t, out.TLSSetting)
	assert.Equal(t, "ca", out.TLSSetting.CAFile)
	assert.Equal(t, "cert", out.TLSSetting.CertFile)
	assert.Equal(t, "key", out.TLSSetting.KeyFile)
	assert.Equal(t, "clientca", out.TLSSetting.ClientCAFile)
	assert.Equal(t, "1.1", out.TLSSetting.MinVersion)
	assert.Equal(t, "1.3", out.TLSSetting.MaxVersion)
	assert.Equal(t, 24*time.Hour, out.TLSSetting.ReloadInterval)
}

func TestApplyOTLPHTTPServerSettings(t *testing.T) {
	otlpFactory := otlpreceiver.NewFactory()
	otlpReceiverConfig := otlpFactory.CreateDefaultConfig().(*otlpreceiver.Config)

	httpOpts := &flags.HTTPOptions{
		HostPort: ":12345",
		TLS: tlscfg.Options{
			Enabled:        true,
			CAPath:         "ca",
			CertPath:       "cert",
			KeyPath:        "key",
			ClientCAPath:   "clientca",
			MinVersion:     "1.1",
			MaxVersion:     "1.3",
			ReloadInterval: 24 * time.Hour,
		},
		CORS: corscfg.Options{
			AllowedOrigins: []string{"http://example.domain.com", "http://*.domain.com"},
			AllowedHeaders: []string{"Content-Type", "Accept", "X-Requested-With"},
		},
	}

	applyHTTPSettings(otlpReceiverConfig.HTTP.ServerConfig, httpOpts)

	out := otlpReceiverConfig.HTTP

	assert.Equal(t, ":12345", out.Endpoint)
	require.NotNil(t, out.TLSSetting)
	assert.Equal(t, "ca", out.TLSSetting.CAFile)
	assert.Equal(t, "cert", out.TLSSetting.CertFile)
	assert.Equal(t, "key", out.TLSSetting.KeyFile)
	assert.Equal(t, "clientca", out.TLSSetting.ClientCAFile)
	assert.Equal(t, "1.1", out.TLSSetting.MinVersion)
	assert.Equal(t, "1.3", out.TLSSetting.MaxVersion)
	assert.Equal(t, 24*time.Hour, out.TLSSetting.ReloadInterval)
	assert.Equal(t, []string{"Content-Type", "Accept", "X-Requested-With"}, out.CORS.AllowedHeaders)
	assert.Equal(t, []string{"http://example.domain.com", "http://*.domain.com"}, out.CORS.AllowedOrigins)
}
