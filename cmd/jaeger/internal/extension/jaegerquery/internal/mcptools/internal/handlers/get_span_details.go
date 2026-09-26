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

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// queryServiceGetTracesInterface defines the interface we need from QueryService for
// get_trace_errors and get_trace_topology.
type queryServiceGetTracesInterface interface {
	GetTraces(ctx context.Context, params querysvc.GetTraceParams) iter.Seq2[[]ptrace.Traces, error]
}

// spanDetailsQueryService defines the interface we need from QueryService for get_span_details:
// GetTraces for the whole-trace fallback, FindSpans for the identity-filter fast path.
type spanDetailsQueryService interface {
	queryServiceGetTracesInterface
	FindSpans(ctx context.Context, query querysvc.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error]
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
	// Build query parameters (includes validation). canonicalSpanIDs holds the
	// requested IDs in the same canonical lowercase hex form span.SpanID().String()
	// emits, so case-insensitive requests still match.
	params, canonicalSpanIDs, err := h.buildQuery(input)
	if err != nil {
		return nil, types.GetSpanDetailsOutput{}, err
	}
	traceID := params.TraceIDs[0].TraceID

	// Create span ID set for efficient lookup
	spanIDSet := make(map[string]struct{}, len(canonicalSpanIDs))
	for _, spanID := range canonicalSpanIDs {
		spanIDSet[spanID] = struct{}{}
	}

	// The identity-filter fast path (RFC 0016 §4.3) errors out before touching spanIDSet
	// whenever it cannot run at all (ErrSpanSearchUnsupported/ErrFilterDisabled), so falling
	// back to the whole-trace path afterward is safe: nothing has been marked found yet.
	spanDetails, err := h.fetchViaFindSpans(ctx, traceID, canonicalSpanIDs, spanIDSet)
	traceFound := true
	if errors.Is(err, querysvc.ErrSpanSearchUnsupported) || errors.Is(err, querysvc.ErrFilterDisabled) {
		spanDetails, traceFound, err = h.fetchViaGetTraces(ctx, params, spanIDSet)
	}
	if err != nil {
		return nil, types.GetSpanDetailsOutput{}, err
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

// fetchViaFindSpans tries the identity-filter fast path (RFC 0016 §4.3): a backend that
// declares SpanSearch answers with exactly the matching spans, not whole traces. It uses the
// widest possible time range because the tool's input carries no time hint to narrow it with,
// and a span search requires one regardless of the filter (RFC 0016 §4.5, "a caller that knows
// only trace IDs supplies a range wide enough to contain them").
//
// A backend that cannot take this path reports querysvc.ErrSpanSearchUnsupported or
// querysvc.ErrFilterDisabled unchanged, so handle falls back to fetchViaGetTraces. That path's
// "trace not found" distinction is not available here: a filter that matches nothing is
// indistinguishable from a trace that does not exist, and telling the two apart would mean
// paying for the whole-trace read this path exists to avoid. handle folds an empty result here
// into the same "spans not found" report a partial match gets, rather than a hard error.
func (h *getSpanDetailsHandler) fetchViaFindSpans(
	ctx context.Context,
	traceID pcommon.TraceID,
	canonicalSpanIDs []string,
	spanIDSet map[string]struct{},
) ([]types.SpanDetail, error) {
	query := querysvc.SpanQueryParams{SpanQueryParams: tracestore.SpanQueryParams{
		StartTimeMin: time.Unix(0, 0),
		StartTimeMax: time.Now(),
		Filter:       buildIdentityFilter(traceID, canonicalSpanIDs),
	}}

	var spanDetails []types.SpanDetail
	for chunk, err := range h.queryService.FindSpans(ctx, query) {
		if err != nil {
			return nil, err
		}
		for pos, span := range jptrace.SpanIter(chunk.Results) {
			spanIDStr := span.SpanID().String()
			if _, found := spanIDSet[spanIDStr]; found {
				spanDetails = append(spanDetails, buildSpanDetail(pos, span))
				delete(spanIDSet, spanIDStr)
			}
		}
	}
	return spanDetails, nil
}

// fetchViaGetTraces is the pre-RFC-0016 path: fetch the whole trace and pick the requested
// spans out of it in memory. Every backend supports it, so it is what a backend that does not
// declare SpanSearch falls back to.
func (h *getSpanDetailsHandler) fetchViaGetTraces(
	ctx context.Context,
	params querysvc.GetTraceParams,
	spanIDSet map[string]struct{},
) ([]types.SpanDetail, bool, error) {
	tracesIter := h.queryService.GetTraces(ctx, params)

	// Wrap with AggregateTraces to ensure each ptrace.Traces contains a complete trace
	aggregatedIter := jptrace.AggregateTraces(tracesIter)

	var spanDetails []types.SpanDetail
	traceFound := false
	for trace, err := range aggregatedIter {
		if err != nil {
			return nil, false, fmt.Errorf("failed to get trace: %w", err)
		}
		traceFound = true
		for pos, span := range jptrace.SpanIter(trace) {
			spanIDStr := span.SpanID().String()
			if _, found := spanIDSet[spanIDStr]; found {
				spanDetails = append(spanDetails, buildSpanDetail(pos, span))
				delete(spanIDSet, spanIDStr)
			}
		}
	}
	return spanDetails, traceFound, nil
}

// buildIdentityFilter names the requested spans as a predicate (RFC 0016 §4.3): trace ID and
// span ID are intrinsic span fields, so naming a span is comparing two of its own fields. Every
// requested span shares the one trace ID this tool takes, so this is a single conjunction
// rather than the OR-of-ANDs a filter naming spans across several traces would need.
func buildIdentityFilter(traceID pcommon.TraceID, spanIDs []string) *expression.Call {
	return &expression.Call{
		Op: expression.OpAnd,
		Args: []expression.Expression{
			&expression.Call{
				Op: expression.OpEq,
				Args: []expression.Expression{
					&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldTraceID},
					&expression.StringValue{Value: traceID.String()},
				},
			},
			&expression.Call{
				Op: expression.OpIn,
				Args: []expression.Expression{
					&expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldSpanID},
					&expression.List{Type: expression.ValueTypeString, Values: spanIDs},
				},
			},
		},
	}
}

// buildQuery converts GetSpanDetailsInput to querysvc.GetTraceParams and returns
// the requested span IDs in canonical lowercase hex form for the lookup set.
func (h *getSpanDetailsHandler) buildQuery(input types.GetSpanDetailsInput) (querysvc.GetTraceParams, []string, error) {
	// Validate input
	if input.TraceID == "" {
		return querysvc.GetTraceParams{}, nil, errors.New("trace_id is required")
	}

	if len(input.SpanIDs) == 0 {
		return querysvc.GetTraceParams{}, nil, errors.New("span_ids is required and must not be empty")
	}

	// Validate span count against configured limit
	if len(input.SpanIDs) > h.maxSpanDetailsPerRequest {
		return querysvc.GetTraceParams{}, nil, fmt.Errorf(
			"span_ids exceeds maximum limit: requested %d, max allowed %d",
			len(input.SpanIDs),
			h.maxSpanDetailsPerRequest,
		)
	}

	traceID, err := jptrace.TraceIDFromString(input.TraceID)
	if err != nil {
		return querysvc.GetTraceParams{}, nil, fmt.Errorf("invalid trace_id: %w", err)
	}

	// Validate every span_id up front so a malformed value fails fast instead of
	// triggering a backend query that can never match a real span ID. Collect the
	// canonical lowercase hex form so the lookup set agrees with the ID the trace
	// iterator emits regardless of the request's casing.
	canonicalSpanIDs := make([]string, 0, len(input.SpanIDs))
	for _, spanIDStr := range input.SpanIDs {
		spanID, err := parseSpanID(spanIDStr)
		if err != nil {
			return querysvc.GetTraceParams{}, nil, fmt.Errorf("invalid span_id %q: %w", spanIDStr, err)
		}
		canonicalSpanIDs = append(canonicalSpanIDs, spanID.String())
	}

	return querysvc.GetTraceParams{
		TraceIDs: []tracestore.GetTraceParams{
			{TraceID: traceID},
		},
		RawTraces: false, // We want adjusted traces
	}, canonicalSpanIDs, nil
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
