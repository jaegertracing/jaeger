// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// queryServiceGetTracesInterface defines the interface we need from QueryService for get_span_details
type queryServiceGetTracesInterface interface {
	GetTraces(ctx context.Context, params querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error]
}

// GetSpanDetailsHandler implements the get_span_details MCP tool.
type GetSpanDetailsHandler struct {
	queryService queryServiceGetTracesInterface
}

// NewGetSpanDetailsHandler creates a new get_span_details handler.
func NewGetSpanDetailsHandler(queryService *querysvc.QueryService) *GetSpanDetailsHandler {
	return &GetSpanDetailsHandler{
		queryService: queryService,
	}
}

// Handle processes the get_span_details tool request.
func (h *GetSpanDetailsHandler) Handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.GetSpanDetailsInput,
) (*mcp.CallToolResult, types.GetSpanDetailsOutput, error) {
	// Validate input
	if input.TraceID == "" {
		return nil, types.GetSpanDetailsOutput{}, errors.New("trace_id is required")
	}

	if len(input.SpanIDs) == 0 {
		return nil, types.GetSpanDetailsOutput{}, errors.New("span_ids is required and must not be empty")
	}

	// Parse trace ID
	traceID, err := parseTraceID(input.TraceID)
	if err != nil {
		return nil, types.GetSpanDetailsOutput{}, fmt.Errorf("invalid trace_id: %w", err)
	}

	// Create span ID set for efficient lookup
	spanIDSet := make(map[string]struct{}, len(input.SpanIDs))
	for _, spanID := range input.SpanIDs {
		spanIDSet[spanID] = struct{}{}
	}

	// Fetch trace
	// RawTraces=false means each returned ptrace.Traces contains a complete, aggregated trace
	// (not split across multiple consecutive ptrace.Traces objects).
	params := querysvc.GetTraceParams{
		TraceIDs: []tracestore.GetTraceParams{
			{TraceID: traceID},
		},
		RawTraces: false, // We want adjusted traces
	}

	tracesIter := h.queryService.GetTraces(ctx, params)

	// Collect spans matching the requested span IDs
	var spanDetails []types.SpanDetail
	var processErr error
	traceFound := false

	tracesIter(func(traces []ptrace.Traces, err error) bool {
		if err != nil {
			// Store error but continue processing to return partial results
			// Returning true allows the iterator to continue with remaining items
			processErr = err
			return true
		}

		for _, trace := range traces {
			traceFound = true
			// Iterate through all spans in the trace
			jptrace.SpanIter(trace)(func(pos jptrace.SpanIterPos, span ptrace.Span) bool {
				spanIDStr := span.SpanID().String()

				// Check if this span ID is in the requested set
				if _, found := spanIDSet[spanIDStr]; found {
					detail := buildSpanDetail(pos, span)
					spanDetails = append(spanDetails, detail)

					// Remove from set to track which spans we've found
					delete(spanIDSet, spanIDStr)
				}

				return true
			})
		}

		return true
	})

	if !traceFound {
		return nil, types.GetSpanDetailsOutput{}, errors.New("trace not found")
	}

	output := types.GetSpanDetailsOutput{
		TraceID: input.TraceID,
		Spans:   spanDetails,
	}

	// If we encountered an error during processing, include it in the output
	if processErr != nil {
		output.Error = fmt.Sprintf("partial results returned due to error: %v", processErr)
	}

	return nil, output, nil
}

// buildSpanDetail constructs a SpanDetail from a ptrace.Span.
func buildSpanDetail(pos jptrace.SpanIterPos, span ptrace.Span) types.SpanDetail {
	// Get service name from resource attributes
	serviceName := ""
	if svc, ok := pos.Resource.Resource().Attributes().Get("service.name"); ok {
		serviceName = svc.Str()
	}

	// Convert status
	status := types.SpanStatus{
		Code:    span.Status().Code().String(),
		Message: span.Status().Message(),
	}

	// Convert attributes
	attributes := make(map[string]any)
	span.Attributes().Range(func(k string, v pcommon.Value) bool {
		attributes[k] = convertAttributeValue(v)
		return true
	})

	// Convert events
	var events []types.SpanEvent
	for i := 0; i < span.Events().Len(); i++ {
		event := span.Events().At(i)
		eventAttrs := make(map[string]any)
		event.Attributes().Range(func(k string, v pcommon.Value) bool {
			eventAttrs[k] = convertAttributeValue(v)
			return true
		})

		events = append(events, types.SpanEvent{
			Name:       event.Name(),
			Timestamp:  event.Timestamp().AsTime().Format(time.RFC3339Nano),
			Attributes: eventAttrs,
		})
	}

	// Convert links
	var links []types.SpanLink
	for i := 0; i < span.Links().Len(); i++ {
		link := span.Links().At(i)
		linkAttrs := make(map[string]any)
		link.Attributes().Range(func(k string, v pcommon.Value) bool {
			linkAttrs[k] = convertAttributeValue(v)
			return true
		})

		links = append(links, types.SpanLink{
			TraceID:    link.TraceID().String(),
			SpanID:     link.SpanID().String(),
			Attributes: linkAttrs,
		})
	}

	// Calculate duration
	durationMs := span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime()).Milliseconds()

	// Get parent span ID
	parentSpanID := ""
	if !span.ParentSpanID().IsEmpty() {
		parentSpanID = span.ParentSpanID().String()
	}

	return types.SpanDetail{
		SpanID:       span.SpanID().String(),
		TraceID:      span.TraceID().String(),
		ParentSpanID: parentSpanID,
		Service:      serviceName,
		Operation:    span.Name(),
		StartTime:    span.StartTimestamp().AsTime().Format(time.RFC3339Nano),
		DurationMs:   durationMs,
		Status:       status,
		Attributes:   attributes,
		Events:       events,
		Links:        links,
	}
}

// convertAttributeValue converts a pcommon.Value to a Go any type.
func convertAttributeValue(v pcommon.Value) any {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		return v.Str()
	case pcommon.ValueTypeInt:
		return v.Int()
	case pcommon.ValueTypeDouble:
		return v.Double()
	case pcommon.ValueTypeBool:
		return v.Bool()
	case pcommon.ValueTypeBytes:
		return v.Bytes().AsRaw()
	case pcommon.ValueTypeSlice:
		slice := v.Slice()
		result := make([]any, slice.Len())
		for i := 0; i < slice.Len(); i++ {
			result[i] = convertAttributeValue(slice.At(i))
		}
		return result
	case pcommon.ValueTypeMap:
		m := v.Map()
		result := make(map[string]any)
		m.Range(func(k string, v pcommon.Value) bool {
			result[k] = convertAttributeValue(v)
			return true
		})
		return result
	default:
		return nil
	}
}

// parseTraceID parses a trace ID string into a pcommon.TraceID.
func parseTraceID(traceIDStr string) (pcommon.TraceID, error) {
	var traceID pcommon.TraceID
	// Parse hex string - TraceID is 16 bytes (32 hex characters)
	if len(traceIDStr) != 32 {
		return pcommon.TraceID{}, fmt.Errorf("trace ID must be 32 hex characters, got %d", len(traceIDStr))
	}

	// Convert hex string to bytes
	for i := 0; i < 16; i++ {
		var b byte
		_, err := fmt.Sscanf(traceIDStr[i*2:i*2+2], "%02x", &b)
		if err != nil {
			return pcommon.TraceID{}, fmt.Errorf("invalid hex character at position %d: %w", i*2, err)
		}
		traceID[i] = b
	}

	return traceID, nil
}
