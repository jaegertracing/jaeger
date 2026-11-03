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

// searchTracesHandler implements the search_traces MCP tool.
// This tool searches for traces matching service, time, attributes, and duration criteria.
// It returns trace summaries without full span details, optimized for
// browsing and filtering large result sets.
type searchTracesHandler struct {
	queryService queryServiceInterface
	maxResults   uint32
}

// NewSearchTracesHandler creates a new search_traces handler and returns the handler function.
func NewSearchTracesHandler(
	queryService *querysvc.QueryService,
	maxResults uint32,
) mcp.ToolHandlerFor[types.SearchTracesInput, types.SearchTracesOutput] {
	h := &searchTracesHandler{
		queryService: queryService,
		maxResults:   maxResults,
	}
	return h.handle
}

// handle processes the search_traces tool request.
func (h *searchTracesHandler) handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.SearchTracesInput,
) (*mcp.CallToolResult, types.SearchTracesOutput, error) {
	query, err := h.buildQuery(input)
	if err != nil {
		return nil, types.SearchTracesOutput{}, err
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

	output := types.SearchTracesOutput{Traces: summaries}
	if len(processErrs) > 0 {
		searchErr := errors.Join(processErrs...)
		if len(summaries) == 0 {
			// Nothing came back — a query the storage backend refused, for one — so calling
			// it partial would send an agent looking for the missing part instead of
			// reading why the search could not run.
			output.Error = searchErr.Error()
		} else {
			output.Error = fmt.Sprintf("partial results returned due to error: %v", searchErr)
		}
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
		startTime = s.MinStartTime.Format(time.RFC3339Nano)
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

// buildQuery converts SearchTracesInput to querysvc.TraceQueryParams.
func (h *searchTracesHandler) buildQuery(input types.SearchTracesInput) (querysvc.TraceQueryParams, error) {
	if strings.TrimSpace(input.SQL) != "" {
		return h.buildSQLQuery(input)
	}
	startTimeMinInput := input.StartTimeMin
	if startTimeMinInput == "" {
		startTimeMinInput = "-1h"
	}

	minStartTime, err := parseTimeParam(startTimeMinInput)
	if err != nil {
		return querysvc.TraceQueryParams{}, fmt.Errorf("invalid start_time_min: %w", err)
	}

	var maxStartTime time.Time
	if input.StartTimeMax != "" {
		maxStartTime, err = parseTimeParam(input.StartTimeMax)
		if err != nil {
			return querysvc.TraceQueryParams{}, fmt.Errorf("invalid start_time_max: %w", err)
		}
	} else {
		maxStartTime = time.Now()
	}

	// Whether the time range and the duration bounds are ordered is the query service's
	// decision, so only what this tool cannot hand over unparsed is checked here.
	var durationMin, durationMax time.Duration
	if input.DurationMin != "" {
		durationMin, err = time.ParseDuration(input.DurationMin)
		if err != nil {
			return querysvc.TraceQueryParams{}, fmt.Errorf("invalid duration_min: %w", err)
		}
	}
	if input.DurationMax != "" {
		durationMax, err = time.ParseDuration(input.DurationMax)
		if err != nil {
			return querysvc.TraceQueryParams{}, fmt.Errorf("invalid duration_max: %w", err)
		}
	}

	attributes := pcommon.NewMap()
	for key, value := range input.Attributes {
		attributes.PutStr(key, value)
	}
	if input.WithErrors {
		attributes.PutStr("error", "true")
	}

	return querysvc.TraceQueryParams{
		ServiceName:   input.ServiceName,
		OperationName: input.SpanName,
		Attributes:    attributes,
		StartTimeMin:  minStartTime,
		StartTimeMax:  maxStartTime,
		DurationMin:   durationMin,
		DurationMax:   durationMax,
		SearchDepth:   h.searchDepth(input.SearchDepth),
		RawTraces:     false,
	}, nil
}

// searchDepth applies the tool's default and the server's cap to a requested depth.
func (h *searchTracesHandler) searchDepth(requested uint32) uint32 {
	if requested == 0 {
		requested = defaultSearchDepth
	}
	if h.maxResults > 0 && requested > h.maxResults {
		requested = h.maxResults
	}
	return requested
}

// buildSQLQuery compiles the statement of input.SQL into the trace search it describes. The
// statement carries the filter and the window itself, so every other filter input is refused
// beside it rather than merged, which keeps one description of the search per request.
func (h *searchTracesHandler) buildSQLQuery(input types.SearchTracesInput) (querysvc.TraceQueryParams, error) {
	if set := structuredInputsSet(input); len(set) > 0 {
		return querysvc.TraceQueryParams{}, fmt.Errorf(
			"sql replaces the other filter inputs; remove %s or write the condition into the statement",
			strings.Join(set, ", "))
	}
	query, err := sqlquery.Compile(input.SQL, time.Now())
	if err != nil {
		return querysvc.TraceQueryParams{}, fmt.Errorf("invalid sql: %w", err)
	}
	if query.Kind == sqlquery.KindFindSpans {
		return querysvc.TraceQueryParams{}, errors.New(
			"invalid sql: the statement selects spans, and search_traces returns traces. " +
				"Write SELECT DISTINCT trace_id FROM spans WHERE ..., " +
				"or SELECT * FROM traces WHERE trace_id IN (SELECT trace_id FROM spans WHERE ...)")
	}
	searchDepth := input.SearchDepth
	if query.Limit > 0 {
		searchDepth = query.Limit
	}
	return querysvc.TraceQueryParams{
		Attributes:   pcommon.NewMap(),
		StartTimeMin: query.StartTimeMin,
		StartTimeMax: query.StartTimeMax,
		SearchDepth:  h.searchDepth(searchDepth),
		Filter:       query.Filter,
		RawTraces:    false,
	}, nil
}

// structuredInputsSet names the filter inputs of input that are set, in the order the type
// declares them. search_depth is not among them because a statement without LIMIT takes it.
func structuredInputsSet(input types.SearchTracesInput) []string {
	var set []string
	add := func(name string, isSet bool) {
		if isSet {
			set = append(set, name)
		}
	}
	add("start_time_min", input.StartTimeMin != "")
	add("start_time_max", input.StartTimeMax != "")
	add("service_name", input.ServiceName != "")
	add("span_name", input.SpanName != "")
	add("attributes", len(input.Attributes) > 0)
	add("with_errors", input.WithErrors)
	add("duration_min", input.DurationMin != "")
	add("duration_max", input.DurationMax != "")
	return set
}

// parseTimeParam parses time parameters supporting RFC3339 and relative time formats.
// Relative formats: "-1h", "-30m", "-5s", "now"
func parseTimeParam(input string) (time.Time, error) {
	input = strings.TrimSpace(input)

	if input == "now" {
		return time.Now(), nil
	}

	if strings.HasPrefix(input, "-") {
		duration, err := time.ParseDuration(input[1:])
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid relative time format: %w", err)
		}
		return time.Now().Add(-duration), nil
	}

	t, err := time.Parse(time.RFC3339Nano, input)
	if err != nil {
		return time.Time{}, fmt.Errorf("time must be RFC3339 or relative format (e.g., '-1h', 'now'): %w", err)
	}
	return t, nil
}
