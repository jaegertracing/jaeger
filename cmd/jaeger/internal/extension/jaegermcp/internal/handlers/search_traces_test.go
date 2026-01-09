// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
)

// createTestTrace creates a sample trace for testing
func createTestTrace(traceID string, serviceName string, operationName string, hasError bool) ptrace.Traces {
	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()

	// Set service name in resource attributes
	resourceSpans.Resource().Attributes().PutStr("service.name", serviceName)

	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
	span := scopeSpans.Spans().AppendEmpty()

	// Set trace ID
	tid := pcommon.TraceID{}
	copy(tid[:], traceID)
	span.SetTraceID(tid)

	// Set span ID (root span has empty parent)
	span.SetSpanID(pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8}))
	span.SetParentSpanID(pcommon.SpanID{}) // Empty parent = root span

	span.SetName(operationName)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(-5 * time.Second)))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now()))

	if hasError {
		span.Status().SetCode(ptrace.StatusCodeError)
		span.Status().SetMessage("Test error")
	} else {
		span.Status().SetCode(ptrace.StatusCodeOk)
	}

	return traces
}

func TestSearchTracesHandler_Handle_Success(t *testing.T) {
	// Create test data
	testTrace := createTestTrace("trace123", "frontend", "/api/checkout", false)

	// Test buildTraceSummary directly
	summary, err := buildTraceSummary(testTrace, false)
	require.NoError(t, err)

	assert.Equal(t, "frontend", summary.RootService)
	assert.Equal(t, "/api/checkout", summary.RootOperation)
	assert.Equal(t, 1, summary.SpanCount)
	assert.Equal(t, 1, summary.ServiceCount)
	assert.False(t, summary.HasErrors)
}

func TestSearchTracesHandler_BuildSummary_WithErrors(t *testing.T) {
	testTrace := createTestTrace("trace456", "payment", "/process", true)

	summary, err := buildTraceSummary(testTrace, false)

	require.NoError(t, err)
	assert.Equal(t, "payment", summary.RootService)
	assert.Equal(t, "/process", summary.RootOperation)
	assert.True(t, summary.HasErrors)
}

func TestSearchTracesHandler_Handle_MissingServiceName(t *testing.T) {
	handler := NewSearchTracesHandler(nil)

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		// Missing ServiceName
	}

	_, _, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "service_name is required")
}

func TestSearchTracesHandler_Handle_InvalidTimeFormat(t *testing.T) {
	handler := NewSearchTracesHandler(nil)

	input := types.SearchTracesInput{
		StartTimeMin: "invalid-time",
		ServiceName:  "test",
	}

	_, _, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid start_time_min")
}

func TestSearchTracesHandler_Handle_InvalidDuration(t *testing.T) {
	handler := NewSearchTracesHandler(nil)

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
		DurationMin:  "invalid-duration",
	}

	_, _, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid duration_min")
}

func TestSearchTracesHandler_Handle_DurationMaxLessThanMin(t *testing.T) {
	handler := NewSearchTracesHandler(nil)

	input := types.SearchTracesInput{
		StartTimeMin: "-1h",
		ServiceName:  "test",
		DurationMin:  "10s",
		DurationMax:  "5s",
	}

	_, _, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "duration_max must be greater than duration_min")
}

func TestParseTimeParam(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "now",
			input:     "now",
			wantError: false,
		},
		{
			name:      "relative hours",
			input:     "-1h",
			wantError: false,
		},
		{
			name:      "relative minutes",
			input:     "-30m",
			wantError: false,
		},
		{
			name:      "relative seconds",
			input:     "-5s",
			wantError: false,
		},
		{
			name:      "RFC3339",
			input:     "2024-01-15T10:30:00Z",
			wantError: false,
		},
		{
			name:      "invalid format",
			input:     "invalid",
			wantError: true,
		},
		{
			name:      "invalid relative",
			input:     "-invalid",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTimeParam(tt.input)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.False(t, result.IsZero())
			}
		})
	}
}
