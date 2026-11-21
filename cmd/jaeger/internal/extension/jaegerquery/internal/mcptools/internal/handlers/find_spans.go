// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/sqlquery"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// findSpansQueryService is the part of QueryService that find_spans calls.
type findSpansQueryService interface {
	FindSpans(ctx context.Context, query querysvc.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error]
}

// findSpansHandler implements the find_spans MCP tool (RFC 0016 M7, RFC 0019). Unlike
// get_span_details, which names spans by IDs the agent already holds, this tool returns the
// spans that a statement over spans describes, across however many traces, in one call.
type findSpansHandler struct {
	queryService findSpansQueryService
	maxResults   uint32
}

// NewFindSpansHandler creates a new find_spans handler and returns the handler function.
func NewFindSpansHandler(
	queryService *querysvc.QueryService,
	maxResults uint32,
) mcp.ToolHandlerFor[types.FindSpansInput, types.FindSpansOutput] {
	h := &findSpansHandler{
		queryService: queryService,
		maxResults:   maxResults,
	}
	return h.handle
}

// handle processes the find_spans tool request.
func (h *findSpansHandler) handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.FindSpansInput,
) (*mcp.CallToolResult, types.FindSpansOutput, error) {
	query, limit, err := h.buildQuery(input)
	if err != nil {
		return nil, types.FindSpansOutput{}, err
	}

	var output types.FindSpansOutput
	for chunk, err := range h.queryService.FindSpans(ctx, query) {
		if err != nil {
			if len(output.Spans) == 0 {
				// Nothing came back, so the call failed: a backend refusing the query
				// (ErrSpanSearchUnsupported, ErrFilterDisabled) or a transport error. The
				// error is the result, not a note beside an empty page.
				return nil, types.FindSpansOutput{}, err
			}
			output.Error = fmt.Sprintf("partial results returned due to error: %v", err)
			return nil, output, nil
		}
		for pos, span := range jptrace.SpanIter(chunk.Results) {
			output.Spans = append(output.Spans, buildSpanSummary(pos, span))
			if limit > 0 && uint64(len(output.Spans)) >= uint64(limit) {
				return nil, output, nil
			}
		}
	}
	return nil, output, nil
}

// buildQuery compiles input.SQL (RFC 0019) into the span search it describes, and the
// effective result limit: the statement's own LIMIT, clamped to the server's configured cap.
func (h *findSpansHandler) buildQuery(input types.FindSpansInput) (querysvc.SpanQueryParams, uint32, error) {
	if strings.TrimSpace(input.SQL) == "" {
		return querysvc.SpanQueryParams{}, 0, errors.New("sql is required")
	}
	query, err := sqlquery.Compile(input.SQL, time.Now())
	if err != nil {
		return querysvc.SpanQueryParams{}, 0, fmt.Errorf("invalid sql: %w", err)
	}
	if query.Kind != sqlquery.KindFindSpans {
		return querysvc.SpanQueryParams{}, 0, errors.New(
			"invalid sql: find_spans takes SELECT * FROM spans WHERE between(start_time, <from>, <to>) AND ... " +
				"[ORDER BY ...] [LIMIT n]. Both bounds on start_time are required")
	}

	limit := h.effectiveLimit(query.Limit)
	return querysvc.SpanQueryParams{
		StartTimeMin: query.StartTimeMin,
		StartTimeMax: query.StartTimeMax,
		Filter:       query.Filter,
		OrderBy:      query.OrderBy,
		Pagination:   querysvc.Pagination{PageSize: limit},
		Projection:   query.Projection,
	}, limit, nil
}

// effectiveLimit is the page size the backend is asked for: the statement's own LIMIT,
// clamped to the server's configured cap, which also stands in for an absent LIMIT.
func (h *findSpansHandler) effectiveLimit(compiledLimit uint32) uint32 {
	if h.maxResults > 0 && (compiledLimit == 0 || compiledLimit > h.maxResults) {
		return h.maxResults
	}
	return compiledLimit
}

// buildSpanSummary is the identity and shape of one span, the same fields that
// get_trace_topology reports per span, with the IDs an agent needs to fetch it.
func buildSpanSummary(pos jptrace.SpanIterPos, span ptrace.Span) types.SpanSummary {
	serviceName := ""
	if svc, ok := pos.Resource.Resource().Attributes().Get("service.name"); ok {
		serviceName = svc.Str()
	}
	parentSpanID := ""
	if !span.ParentSpanID().IsEmpty() {
		parentSpanID = span.ParentSpanID().String()
	}
	return types.SpanSummary{
		SpanID:       span.SpanID().String(),
		TraceID:      span.TraceID().String(),
		ParentSpanID: parentSpanID,
		Service:      serviceName,
		SpanName:     span.Name(),
		StartTime:    span.StartTimestamp().AsTime().Format(time.RFC3339Nano),
		Duration:     span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime()).String(),
		Status:       span.Status().Code().String(),
	}
}
