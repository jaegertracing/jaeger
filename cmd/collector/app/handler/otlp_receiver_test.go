// Copyright (c) 2022 The Jaeger Authors.
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

package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestStartOtlpReceiver(t *testing.T) {
	spanProcessor := &mockSpanProcessor{}
	logger, _ := testutils.NewLogger()
	rec, err := StartOtelReceiver(
		OtelReceiverOptions{
			GRPCHostPort: ":0",
			HTTPHostPort: ":0",
		},
		logger,
		spanProcessor,
	)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, rec.Shutdown(context.Background()))
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
			consumer := newConsumerDelegate(logger, spanProcessor)

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
	_, err := StartOtelReceiver(
		OtelReceiverOptions{
			GRPCHostPort: ":-1",
			HTTPHostPort: ":-1",
		},
		logger,
		spanProcessor,
	)
	assert.Error(t, err)
}
func TestProtoFromTracesError(t *testing.T) {
	mockErr := errors.New("mock error")
	c := &consumerDelegate{
		protoFromTraces: func(td ptrace.Traces) ([]*model.Batch, error) {
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
	assert.Nil(t, host.GetFactory(component.KindReceiver, config.TracesDataType))
	assert.Nil(t, host.GetExtensions())
	assert.Nil(t, host.GetExporters())
}
