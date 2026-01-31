// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

func TestWriteTraces(t *testing.T) {
	// Create a temporary badger factory for testing
	f := badger.NewFactory()
	telset := telemetry.NoopSettings()
	err := f.Initialize(telset.Metrics, telset.Logger)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, f.Close())
	}()

	// Create span writer and reader
	spanWriter, err := f.CreateSpanWriter()
	require.NoError(t, err)
	spanReader, err := f.CreateSpanReader()
	require.NoError(t, err)

	// Create the trace writer
	traceWriter := NewTraceWriter(spanWriter)

	// Create test traces
	td := makeTestTraces()

	// Write traces
	err = traceWriter.WriteTraces(context.Background(), td)
	require.NoError(t, err)

	// Verify the trace was written by reading it back
	// Get the trace ID from the written trace
	traceID := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).TraceID()

	// Convert to v1 trace ID for reading
	v1TraceID := v1adapter.ToV1TraceID(traceID)

	// Use the v1 adapter to read back (since we're only implementing writer now)
	trace, err := spanReader.GetTrace(context.Background(), spanstore.GetTraceParameters{
		TraceID: v1TraceID,
	})
	require.NoError(t, err)
	require.NotNil(t, trace)
	assert.Len(t, trace.Spans, 1)
	assert.Equal(t, "test-operation", trace.Spans[0].OperationName)
}

func TestWriteTracesMultipleSpans(t *testing.T) {
	// Create a temporary badger factory for testing
	f := badger.NewFactory()
	telset := telemetry.NoopSettings()
	err := f.Initialize(telset.Metrics, telset.Logger)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, f.Close())
	}()

	// Create span writer
	spanWriter, err := f.CreateSpanWriter()
	require.NoError(t, err)

	// Create the trace writer
	traceWriter := NewTraceWriter(spanWriter)

	// Create test traces with multiple spans
	td := makeTestTracesWithMultipleSpans()

	// Write traces
	err = traceWriter.WriteTraces(context.Background(), td)
	require.NoError(t, err)
}

// makeTestTraces creates a simple test trace for testing
func makeTestTraces() ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()

	// Set resource attributes
	rs.Resource().Attributes().PutStr("service.name", "test-service")

	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()

	// Set span properties
	span.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	span.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
	span.SetName("test-operation")
	span.SetKind(ptrace.SpanKindServer)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(time.Millisecond * 100)))

	return td
}

// makeTestTracesWithMultipleSpans creates test traces with multiple spans
func makeTestTracesWithMultipleSpans() ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()

	// Set resource attributes
	rs.Resource().Attributes().PutStr("service.name", "test-service")

	ss := rs.ScopeSpans().AppendEmpty()

	// Create multiple spans
	for i := 0; i < 3; i++ {
		span := ss.Spans().AppendEmpty()
		span.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
		span.SetSpanID([8]byte{byte(i + 1), 2, 3, 4, 5, 6, 7, 8})
		span.SetName("test-operation")
		span.SetKind(ptrace.SpanKindServer)
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(time.Millisecond * 100)))
	}

	return td
}
