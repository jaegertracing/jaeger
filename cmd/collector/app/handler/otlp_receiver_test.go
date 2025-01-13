// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore/mocks"
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

func TestOTLPReceiverWithV2Storage(t *testing.T) {
	// Setup mock writer and expectations
	mockWriter := mocks.NewWriter(t)
	expectedTraces := makeTracesOneSpan()
	mockWriter.On("WriteTraces", mock.Anything, mock.Anything).Return(func(ctx context.Context, td ptrace.Traces) error {
		if tenancy.GetTenant(ctx) != "test-tenant" {
			return errors.New("unexpected tenant in context")
		}

		if !assert.ObjectsAreEqual(expectedTraces, td) {
			return errors.New("unexpected trace")
		}

		return nil
	})

	// Create span processor and start receiver
	spanProcessor := &mockSpanProcessor{}
	logger, _ := testutils.NewLogger()

	port := GetAvailablePort(t)

	// Create and start receiver
	rec, err := StartOTLPReceiver(optionsWithPorts(fmt.Sprintf(":%d", port)), logger, spanProcessor, &tenancy.Manager{})
	require.NoError(t, err)

	ctx := context.Background()
	defer rec.Shutdown(ctx)

	// Give the receiver a moment to start up
	time.Sleep(100 * time.Millisecond)

	// Send trace via HTTP
	url := fmt.Sprintf("http://localhost:%d/v1/traces", port)
	client := &http.Client{}

	marshaler := ptrace.JSONMarshaler{}
	data, err := marshaler.MarshalTraces(expectedTraces)
	require.NoError(t, err)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("jaeger-tenant", "test-tenant")

	resp, err := client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify expectations
	mockWriter.AssertExpectations(t)
}

// Helper function if not already available in testutils
func GetAvailablePort(t *testing.T) int {
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}
