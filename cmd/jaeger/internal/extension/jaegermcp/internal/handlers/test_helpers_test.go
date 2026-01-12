// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"iter"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
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

// mockQueryService is a unified mock implementation for both GetTraces and FindTraces
type mockQueryService struct {
	getTracesFunc  func(ctx context.Context, params querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error]
	findTracesFunc func(ctx context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error]
}

func (m *mockQueryService) GetTraces(ctx context.Context, params querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	if m.getTracesFunc != nil {
		return m.getTracesFunc(ctx, params)
	}
	return func(_ func([]ptrace.Traces, error) bool) {}
}

func (m *mockQueryService) FindTraces(ctx context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	if m.findTracesFunc != nil {
		return m.findTracesFunc(ctx, query)
	}
	return func(_ func([]ptrace.Traces, error) bool) {}
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

// newMockFindTraces creates a mock for FindTraces calls that yields the given traces
func newMockFindTraces(traces ...ptrace.Traces) *mockQueryService {
	return &mockQueryService{
		findTracesFunc: func(_ context.Context, _ querysvc.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield(traces, nil)
			}
		},
	}
}

// createTestTrace creates a simple trace with a single span for testing
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
