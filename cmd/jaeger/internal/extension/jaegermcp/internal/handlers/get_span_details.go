// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/hex"
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

// getSpanDetailsHandler implements the get_span_details MCP tool.
// This tool fetches full OTLP details (attributes, events, links, status) for specific spans
// within a trace, allowing deep inspection of individual span data for debugging and analysis.
type getSpanDetailsHandler struct {
	queryService queryServiceGetTracesInterface
}

// NewGetSpanDetailsHandler creates a new get_span_details handler and returns the handler function.
func NewGetSpanDetailsHandler(
	queryService *querysvc.QueryService,
) mcp.ToolHandlerFor[types.GetSpanDetailsInput, types.GetSpanDetailsOutput] {
	h := &getSpanDetailsHandler{
		queryService: queryService,
	}
	return h.handle
}

// handle processes the get_span_details tool request.
func (h *getSpanDetailsHandler) handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.GetSpanDetailsInput,
) (*mcp.CallToolResult, types.GetSpanDetailsOutput, error) {
	// Build query parameters (includes validation)
	params, err := h.buildQuery(input)
	if err != nil {
		return nil, types.GetSpanDetailsOutput{}, err
	}

	// Create span ID set for efficient lookup
	spanIDSet := make(map[string]struct{}, len(input.SpanIDs))
	for _, spanID := range input.SpanIDs {
		spanIDSet[spanID] = struct{}{}
	}

	tracesIter := h.queryService.GetTraces(ctx, params)

	// Wrap with AggregateTraces to ensure each ptrace.Traces contains a complete trace
	aggregatedIter := jptrace.AggregateTraces(tracesIter)

	// Collect spans matching the requested span IDs
	var spanDetails []types.SpanDetail
	traceFound := false

	for trace, err := range aggregatedIter {
		if err != nil {
			// For singular lookups, return error directly
			return nil, types.GetSpanDetailsOutput{}, fmt.Errorf("failed to get trace: %w", err)
		}

		traceFound = true

		// Iterate through all spans in the trace
		for pos, span := range jptrace.SpanIter(trace) {
			spanIDStr := span.SpanID().String()

			// Check if this span ID is in the requested set
			if _, found := spanIDSet[spanIDStr]; found {
				detail := buildSpanDetail(pos, span)
				spanDetails = append(spanDetails, detail)

				// Remove from set to track which spans we've found
				delete(spanIDSet, spanIDStr)
			}
		}
	}

	if !traceFound {
		return nil, types.GetSpanDetailsOutput{}, errors.New("trace not found")
	}

	output := types.GetSpanDetailsOutput{
		TraceID: input.TraceID,
		Spans:   spanDetails,
	}

	// Report any span IDs that were not found
	if len(spanIDSet) > 0 {
		missingIDs := make([]string, 0, len(spanIDSet))
		for spanID := range spanIDSet {
			missingIDs = append(missingIDs, spanID)
		}
		output.Error = fmt.Sprintf("spans not found: %v", missingIDs)
	}

	return nil, output, nil
}

// buildQuery converts GetSpanDetailsInput to querysvc.GetTraceParams.
func (*getSpanDetailsHandler) buildQuery(input types.GetSpanDetailsInput) (querysvc.GetTraceParams, error) {
	// Validate input
	if input.TraceID == "" {
		return querysvc.GetTraceParams{}, errors.New("trace_id is required")
	}

	if len(input.SpanIDs) == 0 {
		return querysvc.GetTraceParams{}, errors.New("span_ids is required and must not be empty")
	}

	traceID, err := parseTraceID(input.TraceID)
	if err != nil {
		return querysvc.GetTraceParams{}, fmt.Errorf("invalid trace_id: %w", err)
	}

	return querysvc.GetTraceParams{
		TraceIDs: []tracestore.GetTraceParams{
			{TraceID: traceID},
		},
		RawTraces: false, // We want adjusted traces
	}, nil
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
	attributes := attributesToMap(span.Attributes())

	// Convert events
	var events []types.SpanEvent
	for _, event := range span.Events().All() {
		events = append(events, types.SpanEvent{
			Name:       event.Name(),
			Timestamp:  event.Timestamp().AsTime().Format(time.RFC3339Nano),
			Attributes: attributesToMap(event.Attributes()),
		})
	}

	// Convert links
	var links []types.SpanLink
	for _, link := range span.Links().All() {
		links = append(links, types.SpanLink{
			TraceID:    link.TraceID().String(),
			SpanID:     link.SpanID().String(),
			Attributes: attributesToMap(link.Attributes()),
		})
	}

	// Calculate duration in microseconds
	durationUs := span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime()).Microseconds()

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
		DurationUs:   durationUs,
		Status:       status,
		Attributes:   attributes,
		Events:       events,
		Links:        links,
	}
}

// attributesToMap converts pcommon.Map attributes to a Go map[string]any.
func attributesToMap(attrs pcommon.Map) map[string]any {
	result := make(map[string]any)
	for k, v := range attrs.All() {
		result[k] = convertAttributeValue(v)
	}
	return result
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
		for k, v := range m.All() {
			result[k] = convertAttributeValue(v)
		}
		return result
	default:
		return nil
	}
}

// parseTraceID parses a trace ID string into a pcommon.TraceID.
func parseTraceID(traceIDStr string) (pcommon.TraceID, error) {
	// Parse hex string - TraceID is 16 bytes (32 hex characters)
	if len(traceIDStr) != 32 {
		return pcommon.TraceID{}, fmt.Errorf("trace ID must be 32 hex characters, got %d", len(traceIDStr))
	}

	var traceID pcommon.TraceID
	bytes, err := hex.DecodeString(traceIDStr)
	if err != nil {
		return pcommon.TraceID{}, fmt.Errorf("invalid hex string: %w", err)
	}

	copy(traceID[:], bytes)
	return traceID, nil
}
