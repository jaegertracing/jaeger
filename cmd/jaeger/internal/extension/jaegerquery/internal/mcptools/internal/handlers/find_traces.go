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
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/sqlquery"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// defaultSearchDepth is the tool's own page bound. An agent reads small pages, so it is lower
// than the query service's default.
const defaultSearchDepth = 10

// queryServiceInterface defines the interface we need from QueryService for testing.
type queryServiceInterface interface {
	FindTraceSummaries(ctx context.Context, query querysvc.TraceQueryParams) iter.Seq2[querysvc.PageChunk[[]tracestore.TraceSummary], error]
}

// findTracesHandler implements the find_traces MCP tool. The tool searches for the traces
// that one SQL statement over spans describes and returns trace summaries without span
// details, optimized for browsing and filtering large result sets.
type findTracesHandler struct {
	queryService queryServiceInterface
	maxResults   uint32
}

// NewFindTracesHandler creates a new find_traces handler and returns the handler function.
func NewFindTracesHandler(
	queryService *querysvc.QueryService,
	maxResults uint32,
) mcp.ToolHandlerFor[types.FindTracesInput, types.FindTracesOutput] {
	h := &findTracesHandler{
		queryService: queryService,
		maxResults:   maxResults,
	}
	return h.handle
}

// handle processes the find_traces tool request.
func (h *findTracesHandler) handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.FindTracesInput,
) (*mcp.CallToolResult, types.FindTracesOutput, error) {
	query, err := h.buildQuery(input)
	if err != nil {
		return nil, types.FindTracesOutput{}, err
	}

	var summaries []types.TraceSummary
	var processErrs []error

outer:
	for chunk, err := range h.queryService.FindTraceSummaries(ctx, query) {
		if err != nil {
			processErrs = append(processErrs, err)
			break
		}
		for i := range chunk.Results {
			summaries = append(summaries, toMCPTraceSummary(chunk.Results[i]))
			if h.maxResults > 0 && uint64(len(summaries)) >= uint64(h.maxResults) {
				break outer
			}
		}
	}

	output := types.FindTracesOutput{Traces: summaries}
	if len(processErrs) > 0 {
		searchErr := errors.Join(processErrs...)
		if len(summaries) == 0 {
			// Nothing came back, so the call failed: a query the storage backend refused,
			// or a transport error. The error is the result, not a note beside an empty
			// page.
			return nil, types.FindTracesOutput{}, searchErr
		}
		output.Error = fmt.Sprintf("partial results returned due to error: %v", searchErr)
	}
	return nil, output, nil
}

// toMCPTraceSummary converts a tracestore.TraceSummary to the MCP-specific types.TraceSummary.
// Services is already sorted by name in tracestore.TraceSummary.
func toMCPTraceSummary(s tracestore.TraceSummary) types.TraceSummary {
	svcNames := make([]string, len(s.Services))
	for i, svc := range s.Services {
		svcNames[i] = svc.Name
	}
	var startTime string
	if !s.MinStartTime.IsZero() {
		startTime = s.MinStartTime.Format(time.RFC3339)
	}
	var durationUs int64
	if !s.MinStartTime.IsZero() && !s.MaxEndTime.IsZero() {
		durationUs = s.MaxEndTime.Sub(s.MinStartTime).Microseconds()
	}
	return types.TraceSummary{
		TraceID:      s.TraceID.String(),
		RootService:  s.RootServiceName,
		RootSpanName: s.RootOperationName,
		StartTime:    startTime,
		DurationUs:   durationUs,
		SpanCount:    s.SpanCount,
		ServiceCount: len(s.Services),
		Services:     svcNames,
		HasErrors:    s.ErrorSpanCount > 0,
	}
}

// buildQuery compiles the statement of input.SQL (RFC 0019) into the trace search it
// describes. The statement carries the filter, the window and, through LIMIT, the search depth,
// so the tool has no other input to merge with it.
func (h *findTracesHandler) buildQuery(input types.FindTracesInput) (querysvc.TraceQueryParams, error) {
	if strings.TrimSpace(input.SQL) == "" {
		return querysvc.TraceQueryParams{}, errors.New("sql is required")
	}
	query, err := sqlquery.Compile(input.SQL, time.Now())
	if err != nil {
		return querysvc.TraceQueryParams{}, fmt.Errorf("invalid sql: %w", err)
	}
	if query.Kind == sqlquery.KindFindSpans {
		return querysvc.TraceQueryParams{}, errors.New(
			"invalid sql: find_traces takes one of: SELECT DISTINCT trace_id FROM spans WHERE between(start_time, <from>, <to>) AND ...; " +
				"or SELECT * FROM traces WHERE trace_id IN (SELECT trace_id FROM spans WHERE between(start_time, <from>, <to>) AND ...). " +
				"Both bounds on start_time are required")
	}
	return querysvc.TraceQueryParams{
		Attributes:   pcommon.NewMap(),
		StartTimeMin: query.StartTimeMin,
		StartTimeMax: query.StartTimeMax,
		SearchDepth:  h.searchDepth(query.Limit),
		Filter:       query.Filter,
		RawTraces:    false,
	}, nil
}

// searchDepth applies the tool's default and the server's cap to the depth a statement's LIMIT
// asks for; zero means the statement has no LIMIT.
func (h *findTracesHandler) searchDepth(requested uint32) uint32 {
	if requested == 0 {
		requested = defaultSearchDepth
	}
	if h.maxResults > 0 && requested > h.maxResults {
		requested = h.maxResults
	}
	return requested
}
