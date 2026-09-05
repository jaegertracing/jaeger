// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// getTraceErrorsHandler implements the get_trace_errors MCP tool.
// This tool retrieves all spans with error status from a specific trace, returning full
// OTLP span details including attributes, events, and links for error analysis.
type getTraceErrorsHandler struct {
	queryService             queryServiceGetTracesInterface
	maxSpanDetailsPerRequest int
}

// NewGetTraceErrorsHandler creates a new get_trace_errors handler and returns the handler function.
func NewGetTraceErrorsHandler(
	queryService *querysvc.QueryService,
	maxSpanDetailsPerRequest int,
) mcp.ToolHandlerFor[types.GetTraceErrorsInput, types.GetTraceErrorsOutput] {
	h := &getTraceErrorsHandler{
		queryService:             queryService,
		maxSpanDetailsPerRequest: maxSpanDetailsPerRequest,
	}
	return h.handle
}

type errorSpanItem struct {
	detail    types.SpanDetail
	startTime pcommon.Timestamp
	spanID    string
}

// handle processes the get_trace_errors tool request.
func (h *getTraceErrorsHandler) handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.GetTraceErrorsInput,
) (*mcp.CallToolResult, types.GetTraceErrorsOutput, error) {
	if input.Offset < 0 {
		return nil, types.GetTraceErrorsOutput{}, errors.New("offset cannot be negative")
	}

	// Build query parameters (includes validation)
	params, err := h.buildQuery(input)
	if err != nil {
		return nil, types.GetTraceErrorsOutput{}, err
	}

	tracesIter := h.queryService.GetTraces(ctx, params)

	// AggregateTraces reassembles the full trace so TotalErrorCount reflects every error span.
	aggregatedIter := jptrace.AggregateTraces(tracesIter)

	var allErrors []errorSpanItem
	traceFound := false

	for trace, err := range aggregatedIter {
		if err != nil {
			return nil, types.GetTraceErrorsOutput{}, err
		}

		traceFound = true

		// Iterate through all spans in the trace
		for pos, span := range jptrace.SpanIter(trace) {
			// Check if span has error status
			if span.Status().Code() == ptrace.StatusCodeError {
				detail := buildSpanDetail(pos, span)
				allErrors = append(allErrors, errorSpanItem{
					detail:    detail,
					startTime: span.StartTimestamp(),
					spanID:    span.SpanID().String(),
				})
			}
		}
	}

	if !traceFound {
		return nil, types.GetTraceErrorsOutput{}, errors.New("trace not found")
	}

	// Sort error spans deterministically by start time, then by SpanID
	sort.Slice(allErrors, func(i, j int) bool {
		if allErrors[i].startTime != allErrors[j].startTime {
			return allErrors[i].startTime < allErrors[j].startTime
		}
		return allErrors[i].spanID < allErrors[j].spanID
	})

	totalErrors := len(allErrors)
	limit := h.maxSpanDetailsPerRequest
	if input.Limit > 0 && (limit == 0 || input.Limit < limit) {
		limit = input.Limit
	}

	var errorSpans []types.SpanDetail
	if input.Offset < totalErrors {
		end := totalErrors
		if limit > 0 && input.Offset+limit < end {
			end = input.Offset + limit
		}
		for i := input.Offset; i < end; i++ {
			errorSpans = append(errorSpans, allErrors[i].detail)
		}
	}

	output := types.GetTraceErrorsOutput{
		TraceID:         input.TraceID,
		TotalErrorCount: totalErrors,
		Offset:          input.Offset,
		Spans:           errorSpans,
	}

	return nil, output, nil
}

// buildQuery converts GetTraceErrorsInput to querysvc.GetTraceParams.
func (*getTraceErrorsHandler) buildQuery(input types.GetTraceErrorsInput) (querysvc.GetTraceParams, error) {
	// Validate input
	if input.TraceID == "" {
		return querysvc.GetTraceParams{}, errors.New("trace_id is required")
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
