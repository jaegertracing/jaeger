// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"iter"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// testTraceID is a common trace ID used across tests
const testTraceID = "12345678901234567890123456789012"

// spanConfig defines the configuration for creating a test span
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

// eventConfig defines the configuration for a span event
type eventConfig struct {
	name       string
	attributes map[string]string
}

// linkConfig defines the configuration for a span link
type linkConfig struct {
	traceID string
	spanID  string
}

// mockQueryService is a unified mock implementation for both GetTraces and FindTraceSummaries
type mockQueryService struct {
	getTracesFunc          func(ctx context.Context, params querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error]
	findTraceSummariesFunc func(ctx context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error]
}

func (m *mockQueryService) GetTraces(ctx context.Context, params querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	if m.getTracesFunc != nil {
		return m.getTracesFunc(ctx, params)
	}
	return func(_ func([]ptrace.Traces, error) bool) {}
}

func (m *mockQueryService) FindTraceSummaries(ctx context.Context, query querysvc.TraceQueryParams) iter.Seq2[querysvc.PageChunk[[]tracestore.TraceSummary], error] {
	return func(yield func(querysvc.PageChunk[[]tracestore.TraceSummary], error) bool) {
		if m.findTraceSummariesFunc == nil {
			return
		}
		for summaries, err := range m.findTraceSummariesFunc(ctx, query) {
			if !yield(querysvc.PageChunk[[]tracestore.TraceSummary]{Results: summaries}, err) {
				return
			}
		}
	}
}

// newMockYieldingTraces creates a mock that yields the given traces for GetTraces calls
func newMockYieldingTraces(traces ...ptrace.Traces) *mockQueryService {
	return &mockQueryService{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield(traces, nil)
			}
		},
	}
}

// newMockYieldingError creates a mock that yields an error for GetTraces calls
func newMockYieldingError(err error) *mockQueryService {
	return &mockQueryService{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield(nil, err)
			}
		},
	}
}

// newMockYieldingEmpty creates a mock that yields no traces for GetTraces calls
func newMockYieldingEmpty() *mockQueryService {
	return &mockQueryService{
		getTracesFunc: func(_ context.Context, _ querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(_ func([]ptrace.Traces, error) bool) {
				// Don't yield anything
			}
		},
	}
}

// newMockFindTraceSummaries creates a mock for FindTraceSummaries calls that yields the given summaries
func newMockFindTraceSummaries(summaries ...tracestore.TraceSummary) *mockQueryService {
	return &mockQueryService{
		findTraceSummariesFunc: func(_ context.Context, _ querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
			return func(yield func([]tracestore.TraceSummary, error) bool) {
				yield(summaries, nil)
			}
		},
	}
}

// createTestTrace creates a simple trace with a single span for testing
func createTestTrace(traceID string, serviceName string, spanName string, hasError bool) ptrace.Traces {
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

	span.SetName(spanName)
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

// putNonFiniteDoubles adds NaN, +Inf and -Inf double attributes to attrs,
// plus a finite one that must stay a JSON number.
func putNonFiniteDoubles(attrs pcommon.Map) {
	attrs.PutDouble("nan", math.NaN())
	attrs.PutDouble("pos-inf", math.Inf(1))
	attrs.PutDouble("neg-inf", math.Inf(-1))
	attrs.PutDouble("finite", 1.5)
}

// createTraceWithNonFiniteDoubles creates a trace with one span whose span,
// event and link attributes all carry non-finite doubles. It returns the trace
// and the span ID in the hex form the tools accept.
func createTraceWithNonFiniteDoubles(code ptrace.StatusCode) (ptrace.Traces, string) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "test-service")
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()

	tid := pcommon.TraceID{}
	copy(tid[:], testTraceID)
	spanID := pcommon.SpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
	span.SetTraceID(tid)
	span.SetSpanID(spanID)
	span.SetName("op")
	span.Status().SetCode(code)

	putNonFiniteDoubles(span.Attributes())
	putNonFiniteDoubles(span.Events().AppendEmpty().Attributes())
	link := span.Links().AppendEmpty()
	link.SetTraceID(tid)
	link.SetSpanID(spanID)
	putNonFiniteDoubles(link.Attributes())

	return traces, spanID.String()
}

// assertNonFiniteDoublesMarshal marshals a tool output the way the MCP SDK
// does and checks that the span, event and link attributes created by
// createTraceWithNonFiniteDoubles came out as the expected JSON values.
func assertNonFiniteDoublesMarshal(t *testing.T, output any) {
	t.Helper()
	out, err := json.Marshal(output)
	require.NoError(t, err)

	// Keep values as raw JSON so a string can be told apart from a number.
	type attrs = map[string]json.RawMessage
	var decoded struct {
		Spans []struct {
			Attributes attrs `json:"attributes"`
			Events     []struct {
				Attributes attrs `json:"attributes"`
			} `json:"events"`
			Links []struct {
				Attributes attrs `json:"attributes"`
			} `json:"links"`
		} `json:"spans"`
	}
	require.NoError(t, json.Unmarshal(out, &decoded))
	require.Len(t, decoded.Spans, 1)
	span := decoded.Spans[0]
	require.Len(t, span.Events, 1)
	require.Len(t, span.Links, 1)

	expected := map[string]string{
		"nan":     `"NaN"`,
		"pos-inf": `"Infinity"`,
		"neg-inf": `"-Infinity"`,
		"finite":  `1.5`,
	}
	for name, got := range map[string]attrs{
		"span":  span.Attributes,
		"event": span.Events[0].Attributes,
		"link":  span.Links[0].Attributes,
	} {
		require.Len(t, got, len(expected), name)
		for key, want := range expected {
			assert.JSONEq(t, want, string(got[key]), "%s attribute %q", name, key)
		}
	}
}
