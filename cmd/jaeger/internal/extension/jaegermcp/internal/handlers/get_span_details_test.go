// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
)

// mockQueryServiceForGetTraces is a mock implementation for GetTraces
type mockQueryServiceForGetTraces struct {
	getTracesFunc func(ctx context.Context, params querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error]
}

func (m *mockQueryServiceForGetTraces) GetTraces(ctx context.Context, params querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	if m.getTracesFunc != nil {
		return m.getTracesFunc(ctx, params)
	}
	return func(_ func([]ptrace.Traces, error) bool) {}
}

// createTestTraceWithSpans creates a trace with multiple spans for testing
func createTestTraceWithSpans(traceID string, spanConfigs []spanConfig) ptrace.Traces {
	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()

	// Set service name in resource attributes
	resourceSpans.Resource().Attributes().PutStr("service.name", "test-service")

	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()

	tid := pcommon.TraceID{}
	copy(tid[:], traceID)

	for i := range spanConfigs {
		config := &spanConfigs[i]
		span := scopeSpans.Spans().AppendEmpty()

		// Set trace ID
		span.SetTraceID(tid)

		// Set span ID - convert string to bytes
		sid := pcommon.SpanID{}
		copy(sid[:], config.spanID)
		span.SetSpanID(sid)

		// Set parent span ID if provided
		if config.parentSpanID != "" {
			psid := pcommon.SpanID{}
			copy(psid[:], config.parentSpanID)
			span.SetParentSpanID(psid)
		}

		span.SetName(config.operation)
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(-5 * time.Second)))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(time.Now()))

		// Set status
		if config.hasError {
			span.Status().SetCode(ptrace.StatusCodeError)
			span.Status().SetMessage(config.errorMessage)
		} else {
			span.Status().SetCode(ptrace.StatusCodeOk)
		}

		// Add attributes if provided
		for k, v := range config.attributes {
			span.Attributes().PutStr(k, v)
		}

		// Add events if provided
		for _, evt := range config.events {
			event := span.Events().AppendEmpty()
			event.SetName(evt.name)
			event.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
			for k, v := range evt.attributes {
				event.Attributes().PutStr(k, v)
			}
		}

		// Add links if provided
		for _, lnk := range config.links {
			link := span.Links().AppendEmpty()
			linkTid := pcommon.TraceID{}
			copy(linkTid[:], lnk.traceID)
			link.SetTraceID(linkTid)
			linkSid := pcommon.SpanID{}
			copy(linkSid[:], lnk.spanID)
			link.SetSpanID(linkSid)
		}
	}

	return traces
}

// spanIDToHex converts a span ID (8 bytes copied from string) to hex string format
// This matches what pcommon.SpanID.String() returns
func spanIDToHex(spanID string) string {
	sid := pcommon.SpanID{}
	copy(sid[:], spanID)
	return sid.String()
}

type spanConfig struct {
	spanID       string
	parentSpanID string
	operation    string
	hasError     bool
	errorMessage string
	attributes   map[string]string
	events       []eventConfig
	links        []linkConfig
}

type eventConfig struct {
	name       string
	attributes map[string]string
}

type linkConfig struct {
	traceID string
	spanID  string
}

func TestGetSpanDetailsHandler_Handle_Success(t *testing.T) {
	traceID := "12345678901234567890123456789012"
	spanID1 := "span001"
	spanID2 := "span002"

	spanConfigs := []spanConfig{
		{
			spanID:    spanID1,
			operation: "/api/test1",
			attributes: map[string]string{
				"http.method": "GET",
				"http.url":    "/api/test1",
			},
		},
		{
			spanID:       spanID2,
			parentSpanID: spanID1,
			operation:    "/api/test2",
			hasError:     true,
			errorMessage: "Test error",
			attributes: map[string]string{
				"http.status_code": "500",
			},
			events: []eventConfig{
				{
					name: "error_event",
					attributes: map[string]string{
						"error.type": "TestError",
					},
				},
			},
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := &mockQueryServiceForGetTraces{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	}

	handler := &GetSpanDetailsHandler{queryService: mock}

	input := types.GetSpanDetailsInput{
		TraceID: traceID,
		SpanIDs: []string{spanIDToHex(spanID1), spanIDToHex(spanID2)},
	}

	_, output, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, traceID, output.TraceID)
	assert.Len(t, output.Spans, 2)

	// Verify span details
	var span1, span2 *types.SpanDetail
	for i := range output.Spans {
		switch output.Spans[i].Operation {
		case "/api/test1":
			span1 = &output.Spans[i]
		case "/api/test2":
			span2 = &output.Spans[i]
		default:
			// Other operations not relevant to this test
		}
	}

	require.NotNil(t, span1)
	assert.Equal(t, "/api/test1", span1.Operation)
	assert.Equal(t, "Ok", span1.Status.Code)
	assert.Equal(t, "GET", span1.Attributes["http.method"])
	assert.Equal(t, "/api/test1", span1.Attributes["http.url"])

	require.NotNil(t, span2)
	assert.Equal(t, "/api/test2", span2.Operation)
	assert.Equal(t, "Error", span2.Status.Code)
	assert.Equal(t, "Test error", span2.Status.Message)
	assert.Equal(t, "500", span2.Attributes["http.status_code"])
	assert.Len(t, span2.Events, 1)
	assert.Equal(t, "error_event", span2.Events[0].Name)
}

func TestGetSpanDetailsHandler_Handle_SingleSpan(t *testing.T) {
	traceID := "12345678901234567890123456789012"
	spanID := "span001"

	spanConfigs := []spanConfig{
		{
			spanID:    spanID,
			operation: "/api/test",
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := &mockQueryServiceForGetTraces{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	}

	handler := &GetSpanDetailsHandler{queryService: mock}

	input := types.GetSpanDetailsInput{
		TraceID: traceID,
		SpanIDs: []string{spanIDToHex(spanID)},
	}

	_, output, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, traceID, output.TraceID)
	assert.Len(t, output.Spans, 1)
	assert.Equal(t, "/api/test", output.Spans[0].Operation)
}

func TestGetSpanDetailsHandler_Handle_FiltersBySpanIDs(t *testing.T) {
	traceID := "12345678901234567890123456789012"

	spanConfigs := []spanConfig{
		{spanID: "span001", operation: "/api/test1"},
		{spanID: "span002", operation: "/api/test2"},
		{spanID: "span003", operation: "/api/test3"},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := &mockQueryServiceForGetTraces{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	}

	handler := &GetSpanDetailsHandler{queryService: mock}

	input := types.GetSpanDetailsInput{
		TraceID: traceID,
		SpanIDs: []string{spanIDToHex("span001"), spanIDToHex("span003")}, // Only request span001 and span003
	}

	_, output, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Len(t, output.Spans, 2)

	operations := make(map[string]bool)
	for _, span := range output.Spans {
		operations[span.Operation] = true
	}

	assert.True(t, operations["/api/test1"])
	assert.False(t, operations["/api/test2"]) // span002 should not be included
	assert.True(t, operations["/api/test3"])
}

func TestGetSpanDetailsHandler_Handle_MissingTraceID(t *testing.T) {
	handler := NewGetSpanDetailsHandler(nil)

	input := types.GetSpanDetailsInput{
		SpanIDs: []string{spanIDToHex("span001")},
	}

	_, _, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "trace_id is required")
}

func TestGetSpanDetailsHandler_Handle_MissingSpanIDs(t *testing.T) {
	handler := NewGetSpanDetailsHandler(nil)

	input := types.GetSpanDetailsInput{
		TraceID: "12345678901234567890123456789012",
		SpanIDs: []string{},
	}

	_, _, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "span_ids is required")
}

func TestGetSpanDetailsHandler_Handle_TooManySpanIDs(t *testing.T) {
	handler := NewGetSpanDetailsHandler(nil)

	// Create 21 span IDs (exceeds max of 20)
	spanIDs := make([]string, 21)
	for i := 0; i < 21; i++ {
		spanIDs[i] = "span" + string(rune(i))
	}

	input := types.GetSpanDetailsInput{
		TraceID: "12345678901234567890123456789012",
		SpanIDs: spanIDs,
	}

	_, _, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "span_ids must not exceed 20 spans")
}

func TestGetSpanDetailsHandler_Handle_InvalidTraceID(t *testing.T) {
	handler := NewGetSpanDetailsHandler(nil)

	input := types.GetSpanDetailsInput{
		TraceID: "invalid-trace-id",
		SpanIDs: []string{spanIDToHex("span001")},
	}

	_, _, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid trace_id")
}

func TestGetSpanDetailsHandler_Handle_TraceNotFound(t *testing.T) {
	mock := &mockQueryServiceForGetTraces{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(_ func([]ptrace.Traces, error) bool) {
				// Don't yield any traces
			}
		},
	}

	handler := &GetSpanDetailsHandler{queryService: mock}

	input := types.GetSpanDetailsInput{
		TraceID: "12345678901234567890123456789012",
		SpanIDs: []string{spanIDToHex("span001")},
	}

	_, _, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "trace not found")
}

func TestGetSpanDetailsHandler_Handle_QueryError(t *testing.T) {
	mock := &mockQueryServiceForGetTraces{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield(nil, errors.New("database connection failed"))
			}
		},
	}

	handler := &GetSpanDetailsHandler{queryService: mock}

	input := types.GetSpanDetailsInput{
		TraceID: "12345678901234567890123456789012",
		SpanIDs: []string{spanIDToHex("span001")},
	}

	_, _, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get trace")
	assert.Contains(t, err.Error(), "database connection failed")
}

func TestGetSpanDetailsHandler_Handle_WithParentSpanID(t *testing.T) {
	traceID := "12345678901234567890123456789012"
	parentSpanID := "parent001"
	childSpanID := "child001"

	spanConfigs := []spanConfig{
		{
			spanID:    parentSpanID,
			operation: "/parent",
		},
		{
			spanID:       childSpanID,
			parentSpanID: parentSpanID,
			operation:    "/child",
		},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := &mockQueryServiceForGetTraces{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	}

	handler := &GetSpanDetailsHandler{queryService: mock}

	input := types.GetSpanDetailsInput{
		TraceID: traceID,
		SpanIDs: []string{spanIDToHex(childSpanID)},
	}

	_, output, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Len(t, output.Spans, 1)
	assert.NotEmpty(t, output.Spans[0].ParentSpanID)
}

func TestGetSpanDetailsHandler_Handle_NoMatchingSpans(t *testing.T) {
	traceID := "12345678901234567890123456789012"

	spanConfigs := []spanConfig{
		{spanID: "span001", operation: "/api/test"},
	}

	testTrace := createTestTraceWithSpans(traceID, spanConfigs)

	mock := &mockQueryServiceForGetTraces{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	}

	handler := &GetSpanDetailsHandler{queryService: mock}

	input := types.GetSpanDetailsInput{
		TraceID: traceID,
		SpanIDs: []string{spanIDToHex("nonexistent_span")}, // Request a span that doesn't exist
	}

	_, output, err := handler.Handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Empty(t, output.Spans) // No spans should be returned
}

func TestConvertAttributeValue(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() pcommon.Value
		expected any
	}{
		{
			name: "string value",
			setup: func() pcommon.Value {
				v := pcommon.NewValueStr("test")
				return v
			},
			expected: "test",
		},
		{
			name: "int value",
			setup: func() pcommon.Value {
				v := pcommon.NewValueInt(42)
				return v
			},
			expected: int64(42),
		},
		{
			name: "double value",
			setup: func() pcommon.Value {
				v := pcommon.NewValueDouble(3.14)
				return v
			},
			expected: 3.14,
		},
		{
			name: "bool value",
			setup: func() pcommon.Value {
				v := pcommon.NewValueBool(true)
				return v
			},
			expected: true,
		},
		{
			name: "slice value",
			setup: func() pcommon.Value {
				v := pcommon.NewValueSlice()
				slice := v.Slice()
				slice.AppendEmpty().SetStr("item1")
				slice.AppendEmpty().SetStr("item2")
				return v
			},
			expected: []any{"item1", "item2"},
		},
		{
			name: "map value",
			setup: func() pcommon.Value {
				v := pcommon.NewValueMap()
				m := v.Map()
				m.PutStr("key1", "value1")
				m.PutInt("key2", 123)
				return v
			},
			expected: map[string]any{
				"key1": "value1",
				"key2": int64(123),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := tt.setup()
			result := convertAttributeValue(value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseTraceID(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "valid trace ID",
			input:     "00000000000000000000000000000001",
			wantError: false,
		},
		{
			name:      "invalid trace ID",
			input:     "invalid",
			wantError: true,
		},
		{
			name:      "empty trace ID",
			input:     "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTraceID(tt.input)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.False(t, result.IsEmpty())
			}
		})
	}
}
