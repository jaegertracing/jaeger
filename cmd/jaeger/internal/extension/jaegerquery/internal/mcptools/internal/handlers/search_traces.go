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
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// queryServiceInterface defines the interface we need from QueryService for testing.
type queryServiceInterface interface {
	FindTraceSummaries(ctx context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error]
}

// searchTracesHandler implements the search_traces MCP tool.
// This tool searches for traces matching service, time, attributes, and duration criteria.
// It returns trace summaries without full span details, optimized for
// browsing and filtering large result sets.
type searchTracesHandler struct {
	queryService queryServiceInterface
	maxResults   int
}

// NewSearchTracesHandler creates a new search_traces handler and returns the handler function.
func NewSearchTracesHandler(
	queryService *querysvc.QueryService,
	maxResults int,
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
	for batch, err := range h.queryService.FindTraceSummaries(ctx, query) {
		if err != nil {
			processErrs = append(processErrs, err)
			break
		}
		for i := range batch {
			summaries = append(summaries, toMCPTraceSummary(batch[i]))
			if h.maxResults > 0 && len(summaries) >= h.maxResults {
				break outer
			}
		}
	}

	output := types.SearchTracesOutput{Traces: summaries}
	if len(processErrs) > 0 {
		output.Error = fmt.Sprintf("partial results returned due to error: %v", errors.Join(processErrs...))
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

// buildQuery converts SearchTracesInput to querysvc.TraceQueryParams.
func (h *searchTracesHandler) buildQuery(input types.SearchTracesInput) (querysvc.TraceQueryParams, error) {
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

	if !maxStartTime.IsZero() && maxStartTime.Before(minStartTime) {
		return querysvc.TraceQueryParams{}, errors.New("start_time_max must be after start_time_min")
	}

	if input.ServiceName == "" {
		return querysvc.TraceQueryParams{}, errors.New("service_name is required")
	}

	var durationMin, durationMax time.Duration
	if input.DurationMin != "" {
		durationMin, err = time.ParseDuration(input.DurationMin)
		if err != nil {
			return querysvc.TraceQueryParams{}, fmt.Errorf("invalid duration_min: %w", err)
		}
		if durationMin < 0 {
			return querysvc.TraceQueryParams{}, errors.New("invalid duration_min: duration cannot be negative")
		}
	}
	if input.DurationMax != "" {
		durationMax, err = time.ParseDuration(input.DurationMax)
		if err != nil {
			return querysvc.TraceQueryParams{}, fmt.Errorf("invalid duration_max: %w", err)
		}
		if durationMax < 0 {
			return querysvc.TraceQueryParams{}, errors.New("invalid duration_max: duration cannot be negative")
		}
	}

	if durationMin > 0 && durationMax > 0 && durationMax < durationMin {
		return querysvc.TraceQueryParams{}, errors.New("duration_max must be greater than duration_min")
	}

	const defaultSearchDepth = 10
	searchDepth := input.SearchDepth
	if searchDepth <= 0 {
		searchDepth = defaultSearchDepth
	}
	if searchDepth > h.maxResults {
		searchDepth = h.maxResults
	}

	attributes := pcommon.NewMap()
	for key, value := range input.Attributes {
		attributes.PutStr(key, value)
	}
	if input.WithErrors {
		attributes.PutStr("error", "true")
	}

	return querysvc.TraceQueryParams{
		TraceQueryParams: tracestore.TraceQueryParams{
			ServiceName:   input.ServiceName,
			OperationName: input.SpanName,
			Attributes:    attributes,
			StartTimeMin:  minStartTime,
			StartTimeMax:  maxStartTime,
			DurationMin:   durationMin,
			DurationMax:   durationMax,
			SearchDepth:   searchDepth,
		},
		RawTraces: false,
	}, nil
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

	t, err := time.Parse(time.RFC3339, input)
	if err != nil {
		return time.Time{}, fmt.Errorf("time must be RFC3339 or relative format (e.g., '-1h', 'now'): %w", err)
	}
	return t, nil
}
