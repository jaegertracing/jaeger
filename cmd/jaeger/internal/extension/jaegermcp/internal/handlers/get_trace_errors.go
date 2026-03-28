// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
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

// handle processes the get_trace_errors tool request.
func (h *getTraceErrorsHandler) handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.GetTraceErrorsInput,
) (*mcp.CallToolResult, types.GetTraceErrorsOutput, error) {
	// Build query parameters (includes validation)
	params, err := h.buildQuery(input)
	if err != nil {
		return nil, types.GetTraceErrorsOutput{}, err
	}

	tracesIter := h.queryService.GetTraces(ctx, params)

	// AggregateTraces reassembles the full trace so ErrorCount reflects every error span.
	// The Spans slice is capped inside the iteration loop below.
	aggregatedIter := jptrace.AggregateTraces(tracesIter)

	// Collect spans with error status
	var errorSpans []types.SpanDetail
	totalErrors := 0
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
				totalErrors++
				// Only build and collect detail up to the limit
				if h.maxSpanDetailsPerRequest == 0 || len(errorSpans) < h.maxSpanDetailsPerRequest {
					detail := buildSpanDetail(pos, span)
					errorSpans = append(errorSpans, detail)
				}
			}
		}
	}

	if !traceFound {
		return nil, types.GetTraceErrorsOutput{}, errors.New("trace not found")
	}

	output := types.GetTraceErrorsOutput{
		TraceID:    input.TraceID,
		ErrorCount: totalErrors,
		Spans:      errorSpans,
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
