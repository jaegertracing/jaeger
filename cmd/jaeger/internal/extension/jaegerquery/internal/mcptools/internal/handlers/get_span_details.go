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

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/jptrace"
)

// queryServiceGetTracesInterface defines the interface we need from QueryService for
// get_trace_errors and get_trace_topology.
type queryServiceGetTracesInterface interface {
	GetTraces(ctx context.Context, params querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error]
}

// spanDetailsQueryService defines the interface we need from QueryService for get_span_details.
// How the spans are found (the identity-filter fast path, the whole-trace read, archive storage,
// interceptors) is the query service's concern, not this tool's.
type spanDetailsQueryService interface {
	LookupSpans(ctx context.Context, params querysvc.SpanLookupParams) (querysvc.SpanLookupResult, error)
}

// getSpanDetailsHandler implements the get_span_details MCP tool.
// This tool fetches full OTLP details (attributes, events, links, status) for specific spans
// within a trace, allowing deep inspection of individual span data for debugging and analysis.
type getSpanDetailsHandler struct {
	queryService             spanDetailsQueryService
	maxSpanDetailsPerRequest int
}

// NewGetSpanDetailsHandler creates a new get_span_details handler and returns the handler function.
func NewGetSpanDetailsHandler(
	queryService *querysvc.QueryService,
	maxSpanDetailsPerRequest int,
) mcp.ToolHandlerFor[types.GetSpanDetailsInput, types.GetSpanDetailsOutput] {
	h := &getSpanDetailsHandler{
		queryService:             queryService,
		maxSpanDetailsPerRequest: maxSpanDetailsPerRequest,
	}
	return h.handle
}

// handle processes the get_span_details tool request.
func (h *getSpanDetailsHandler) handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.GetSpanDetailsInput,
) (*mcp.CallToolResult, types.GetSpanDetailsOutput, error) {
	q, err := h.buildQuery(input)
	if err != nil {
		return nil, types.GetSpanDetailsOutput{}, err
	}

	result, err := h.queryService.LookupSpans(ctx, querysvc.SpanLookupParams{
		TraceID: q.traceID,
		SpanIDs: q.spanIDs,
	})
	if err != nil {
		return nil, types.GetSpanDetailsOutput{}, err
	}
	if !result.TraceFound {
		return nil, types.GetSpanDetailsOutput{}, errors.New("trace not found")
	}

	var spanDetails []types.SpanDetail
	found := make(map[pcommon.SpanID]struct{}, len(result.Spans))
	for _, match := range result.Spans {
		spanDetails = append(spanDetails, buildSpanDetail(match.Resource, match.Span))
		found[match.Span.SpanID()] = struct{}{}
	}

	output := types.GetSpanDetailsOutput{
		TraceID: input.TraceID,
		Spans:   spanDetails,
	}

	// Report the requested span IDs that were not found, in the order they were requested.
	var missingIDs []string
	reported := make(map[pcommon.SpanID]struct{}, len(q.spanIDs))
	for _, spanID := range q.spanIDs {
		if _, ok := found[spanID]; ok {
			continue
		}
		if _, ok := reported[spanID]; ok {
			continue
		}
		reported[spanID] = struct{}{}
		missingIDs = append(missingIDs, spanID.String())
	}
	if len(missingIDs) > 0 {
		output.Error = fmt.Sprintf("spans not found: %v", missingIDs)
	}

	return nil, output, nil
}

// spanDetailsQuery is buildQuery's parsed result.
type spanDetailsQuery struct {
	traceID pcommon.TraceID
	spanIDs []pcommon.SpanID
}

// buildQuery validates GetSpanDetailsInput and parses it into a spanDetailsQuery.
func (h *getSpanDetailsHandler) buildQuery(input types.GetSpanDetailsInput) (spanDetailsQuery, error) {
	// Validate input
	if input.TraceID == "" {
		return spanDetailsQuery{}, errors.New("trace_id is required")
	}

	if len(input.SpanIDs) == 0 {
		return spanDetailsQuery{}, errors.New("span_ids is required and must not be empty")
	}

	// Validate span count against configured limit
	if len(input.SpanIDs) > h.maxSpanDetailsPerRequest {
		return spanDetailsQuery{}, fmt.Errorf(
			"span_ids exceeds maximum limit: requested %d, max allowed %d",
			len(input.SpanIDs),
			h.maxSpanDetailsPerRequest,
		)
	}

	traceID, err := jptrace.TraceIDFromString(input.TraceID)
	if err != nil {
		return spanDetailsQuery{}, fmt.Errorf("invalid trace_id: %w", err)
	}

	// Validate every span_id up front so a malformed value fails fast instead of
	// triggering a backend query that can never match a real span ID.
	spanIDs := make([]pcommon.SpanID, 0, len(input.SpanIDs))
	for _, spanIDStr := range input.SpanIDs {
		spanID, err := parseSpanID(spanIDStr)
		if err != nil {
			return spanDetailsQuery{}, fmt.Errorf("invalid span_id %q: %w", spanIDStr, err)
		}
		spanIDs = append(spanIDs, spanID)
	}

	return spanDetailsQuery{traceID: traceID, spanIDs: spanIDs}, nil
}

// buildSpanDetail constructs a SpanDetail from a span and the resource it was reported under.
func buildSpanDetail(resource pcommon.Resource, span ptrace.Span) types.SpanDetail {
	// Get service name from resource attributes
	serviceName := ""
	if svc, ok := resource.Attributes().Get("service.name"); ok {
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
		SpanName:     span.Name(),
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

// parseSpanID parses a span ID string into a pcommon.SpanID and rejects the
// all-zero ID, which can never identify a real span: SpanID.String() returns
// "" for it, so it would silently never match in the lookup.
func parseSpanID(spanIDStr string) (pcommon.SpanID, error) {
	spanID, err := jptrace.SpanIDFromString(spanIDStr)
	if err != nil {
		return pcommon.SpanID{}, err
	}
	if spanID.IsEmpty() {
		return pcommon.SpanID{}, errors.New("span ID must not be all zero")
	}
	return spanID, nil
}
