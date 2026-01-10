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
	queryService queryServiceGetTracesInterface
}

// NewGetTraceErrorsHandler creates a new get_trace_errors handler and returns the handler function.
func NewGetTraceErrorsHandler(
	queryService *querysvc.QueryService,
) mcp.ToolHandlerFor[types.GetTraceErrorsInput, types.GetTraceErrorsOutput] {
	h := &getTraceErrorsHandler{
		queryService: queryService,
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

	// Wrap with AggregateTraces to ensure each ptrace.Traces contains a complete trace
	aggregatedIter := jptrace.AggregateTraces(tracesIter)

	// Collect spans with error status
	var errorSpans []types.SpanDetail
	var processErr error
	traceFound := false

	for trace, err := range aggregatedIter {
		if err != nil {
			// For singular lookups, store error and break
			processErr = err
			break
		}

		traceFound = true

		// Iterate through all spans in the trace
		for pos, span := range jptrace.SpanIter(trace) {
			// Check if span has error status
			if span.Status().Code() == ptrace.StatusCodeError {
				detail := buildSpanDetail(pos, span)
				errorSpans = append(errorSpans, detail)
			}
		}
	}

	// If we encountered an error, return it directly
	if processErr != nil {
		return nil, types.GetTraceErrorsOutput{}, fmt.Errorf("failed to get trace: %w", processErr)
	}

	if !traceFound {
		return nil, types.GetTraceErrorsOutput{}, errors.New("trace not found")
	}

	output := types.GetTraceErrorsOutput{
		TraceID:    input.TraceID,
		ErrorCount: len(errorSpans),
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
