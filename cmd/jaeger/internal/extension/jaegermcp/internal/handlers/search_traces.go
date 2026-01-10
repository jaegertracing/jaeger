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
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

const (
	defaultSearchLimit = 10
	maxSearchLimit     = 100
)

// queryServiceInterface defines the interface we need from QueryService for testing
type queryServiceInterface interface {
	FindTraces(ctx context.Context, query querysvc.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error]
}

// SearchTracesHandler implements the search_traces MCP tool.
type SearchTracesHandler struct {
	queryService queryServiceInterface
}

// NewSearchTracesHandler creates a new search_traces handler.
func NewSearchTracesHandler(queryService *querysvc.QueryService) *SearchTracesHandler {
	return &SearchTracesHandler{
		queryService: queryService,
	}
}

// Handle processes the search_traces tool request.
func (h *SearchTracesHandler) Handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.SearchTracesInput,
) (*mcp.CallToolResult, types.SearchTracesOutput, error) {
	// Build trace query parameters
	query, err := buildTraceQueryParams(input)
	if err != nil {
		return nil, types.SearchTracesOutput{}, err
	}

	// Execute search
	tracesIter := h.queryService.FindTraces(ctx, query)

	// Wrap with AggregateTraces to ensure each ptrace.Traces contains a complete trace
	aggregatedIter := jptrace.AggregateTraces(tracesIter)

	// Process results
	var summaries []types.TraceSummary
	var processErr error

	aggregatedIter(func(trace ptrace.Traces, err error) bool {
		if err != nil {
			// Store error but continue processing to return partial results
			// Returning true allows the iterator to continue with remaining items
			processErr = err
			return true
		}

		summary := buildTraceSummary(trace, input.WithErrors)

		// Filter by error status if requested
		if input.WithErrors && !summary.HasErrors {
			return true
		}

		summaries = append(summaries, summary)
		return true
	})

	output := types.SearchTracesOutput{Traces: summaries}

	// If we encountered an error during processing, include it in the output
	if processErr != nil {
		output.Error = fmt.Sprintf("partial results returned due to error: %v", processErr)
	}

	return nil, output, nil
}

// buildTraceQueryParams converts SearchTracesInput to querysvc.TraceQueryParams.
func buildTraceQueryParams(input types.SearchTracesInput) (querysvc.TraceQueryParams, error) {
	// Parse and validate input
	startTimeMin, err := parseTimeParam(input.StartTimeMin)
	if err != nil {
		return querysvc.TraceQueryParams{}, fmt.Errorf("invalid start_time_min: %w", err)
	}

	var startTimeMax time.Time
	if input.StartTimeMax != "" {
		startTimeMax, err = parseTimeParam(input.StartTimeMax)
		if err != nil {
			return querysvc.TraceQueryParams{}, fmt.Errorf("invalid start_time_max: %w", err)
		}
	} else {
		startTimeMax = time.Now()
	}

	if input.ServiceName == "" {
		return querysvc.TraceQueryParams{}, errors.New("service_name is required")
	}

	// Parse duration parameters
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

	// Validate duration range
	if durationMin > 0 && durationMax > 0 && durationMax < durationMin {
		return querysvc.TraceQueryParams{}, errors.New("duration_max must be greater than duration_min")
	}

	// Set default and max search depth
	searchDepth := input.SearchDepth
	if searchDepth <= 0 {
		searchDepth = defaultSearchLimit
	}
	if searchDepth > maxSearchLimit {
		searchDepth = maxSearchLimit
	}

	// Convert attributes map to pcommon.Map
	attributes := pcommon.NewMap()
	for key, value := range input.Attributes {
		attributes.PutStr(key, value)
	}

	return querysvc.TraceQueryParams{
		TraceQueryParams: tracestore.TraceQueryParams{
			ServiceName:   input.ServiceName,
			OperationName: input.OperationName,
			Attributes:    attributes,
			StartTimeMin:  startTimeMin,
			StartTimeMax:  startTimeMax,
			DurationMin:   durationMin,
			DurationMax:   durationMax,
			SearchDepth:   searchDepth,
		},
		RawTraces: false, // We want adjusted traces
	}, nil
}

// buildTraceSummary constructs a TraceSummary from ptrace.Traces.
func buildTraceSummary(trace ptrace.Traces, _ bool) types.TraceSummary {
	var summary types.TraceSummary
	var rootSpan ptrace.Span
	var rootServiceName string
	var traceID pcommon.TraceID
	services := make(map[string]struct{})
	hasErrors := false
	var minStartTime, maxEndTime time.Time
	spanCount := 0

	// Iterate through all spans in the trace
	jptrace.SpanIter(trace)(func(pos jptrace.SpanIterPos, span ptrace.Span) bool {
		spanCount++
		traceID = span.TraceID()

		// Track unique services from resource attributes
		if serviceName, ok := pos.Resource.Resource().Attributes().Get("service.name"); ok {
			services[serviceName.Str()] = struct{}{}
		}

		// Find root span (span with no parent)
		if span.ParentSpanID().IsEmpty() {
			rootSpan = span
			// Get service name for root span
			if serviceName, ok := pos.Resource.Resource().Attributes().Get("service.name"); ok {
				rootServiceName = serviceName.Str()
			}
		}

		// Track start and end times
		startTime := span.StartTimestamp().AsTime()
		endTime := span.EndTimestamp().AsTime()
		if minStartTime.IsZero() || startTime.Before(minStartTime) {
			minStartTime = startTime
		}
		if maxEndTime.IsZero() || endTime.After(maxEndTime) {
			maxEndTime = endTime
		}

		// Check for errors
		if span.Status().Code() == ptrace.StatusCodeError {
			hasErrors = true
		}

		return true
	})

	// Calculate duration in microseconds
	var durationUs int64
	if !minStartTime.IsZero() && !maxEndTime.IsZero() {
		durationUs = maxEndTime.Sub(minStartTime).Microseconds()
	}

	// Extract root span information
	if rootSpan != (ptrace.Span{}) {
		summary.RootOperation = rootSpan.Name()
		summary.RootService = rootServiceName
	}

	// Build summary
	summary.TraceID = traceID.String()
	summary.StartTime = minStartTime.Format(time.RFC3339)
	summary.DurationUs = durationUs
	summary.SpanCount = spanCount
	summary.ServiceCount = len(services)
	summary.HasErrors = hasErrors

	return summary
}

// parseTimeParam parses time parameters supporting RFC3339 and relative time formats.
// Relative formats: "-1h", "-30m", "-5s", "now"
func parseTimeParam(input string) (time.Time, error) {
	input = strings.TrimSpace(input)

	// Handle "now"
	if input == "now" {
		return time.Now(), nil
	}

	// Handle relative time (e.g., "-1h", "-30m")
	if strings.HasPrefix(input, "-") {
		duration, err := time.ParseDuration(input[1:]) // Remove the "-" prefix
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid relative time format: %w", err)
		}
		return time.Now().Add(-duration), nil
	}

	// Try RFC3339 format
	t, err := time.Parse(time.RFC3339, input)
	if err != nil {
		return time.Time{}, fmt.Errorf("time must be RFC3339 or relative format (e.g., '-1h', 'now'): %w", err)
	}

	return t, nil
}
