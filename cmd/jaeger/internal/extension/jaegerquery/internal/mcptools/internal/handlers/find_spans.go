// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"math"
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

// findSpansQueryService defines the interface we need from QueryService for find_spans.
type findSpansQueryService interface {
	FindSpans(ctx context.Context, query querysvc.SpanQueryParams) iter.Seq2[tracestore.PageChunk[ptrace.Traces], error]
}

// findSpansHandler implements the find_spans MCP tool (RFC 0016 M7, RFC 0019). Unlike
// get_span_details, which names spans it already knows the IDs of, this tool discovers spans
// matching whatever a statement over spans describes, across however many traces in one call,
// which before this tool took a search_traces call followed by one or more get_span_details
// calls.
type findSpansHandler struct {
	queryService findSpansQueryService
	maxResults   int
}

// NewFindSpansHandler creates a new find_spans handler and returns the handler function.
func NewFindSpansHandler(
	queryService *querysvc.QueryService,
	maxResults int,
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

	var spans []types.SpanDetail
	var processErrs []error

outer:
	for chunk, err := range h.queryService.FindSpans(ctx, query) {
		if err != nil {
			processErrs = append(processErrs, err)
			break
		}
		for pos, span := range jptrace.SpanIter(chunk.Results) {
			spans = append(spans, buildSpanDetail(pos, span))
			if limit > 0 && len(spans) >= clampToInt32(limit) {
				break outer
			}
		}
	}

	output := types.FindSpansOutput{Spans: spans}
	if len(processErrs) > 0 {
		searchErr := errors.Join(processErrs...)
		if len(spans) == 0 {
			// Nothing came back, so calling it partial would send an agent looking for
			// the missing part instead of reading why the search could not run: a
			// backend refusing the query (ErrSpanSearchUnsupported, ErrFilterDisabled)
			// reports the same way search_traces reports FindTraceSummaries refusing.
			output.Error = searchErr.Error()
		} else {
			output.Error = fmt.Sprintf("partial results returned due to error: %v", searchErr)
		}
	}
	return nil, output, nil
}

// buildQuery compiles input.SQL (RFC 0019) into the span search it describes, and the
// effective result limit: the statement's own LIMIT when present, else input.MaxResults,
// clamped either way to the server's configured cap.
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
			"invalid sql: the statement selects traces, and find_spans returns spans. " +
				"Send it to search_traces instead, or write SELECT * FROM spans WHERE ... " +
				"to search spans")
	}

	limit := h.effectiveLimit(input.MaxResults, query.Limit)
	return querysvc.SpanQueryParams{
		StartTimeMin: query.StartTimeMin,
		StartTimeMax: query.StartTimeMax,
		Filter:       query.Filter,
		OrderBy:      query.OrderBy,
		Pagination:   querysvc.Pagination{PageSize: limit},
	}, limit, nil
}

// effectiveLimit is the page size the backend is asked for: the statement's own LIMIT when
// present, else requested (input.MaxResults), with the server's configured cap applied either
// way. A zero result here is passed through as a zero PageSize, which the query service reads
// as its own default, rather than this handler guessing at one.
func (h *findSpansHandler) effectiveLimit(requested int, compiledLimit uint32) uint32 {
	limit := requested
	if compiledLimit > 0 {
		limit = clampToInt32(compiledLimit)
	}
	if limit < 0 {
		limit = 0
	}
	if h.maxResults > 0 && (limit == 0 || limit > h.maxResults) {
		limit = h.maxResults
	}
	return uint32(limit)
}

// clampToInt32 converts a uint32 to int with an explicit upper bound, so the conversion stays
// in range regardless of whether int is 32 or 64 bits on the build platform.
func clampToInt32(v uint32) int {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(v)
}
