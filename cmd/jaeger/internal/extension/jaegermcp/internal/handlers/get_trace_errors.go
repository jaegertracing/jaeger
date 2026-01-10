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

// GetTraceErrorsHandler implements the get_trace_errors MCP tool.
type GetTraceErrorsHandler struct {
	queryService queryServiceGetTracesInterface
}

// NewGetTraceErrorsHandler creates a new get_trace_errors handler.
func NewGetTraceErrorsHandler(queryService *querysvc.QueryService) *GetTraceErrorsHandler {
	return &GetTraceErrorsHandler{
		queryService: queryService,
	}
}

// Handle processes the get_trace_errors tool request.
func (h *GetTraceErrorsHandler) Handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.GetTraceErrorsInput,
) (*mcp.CallToolResult, types.GetTraceErrorsOutput, error) {
	// Validate input
	if input.TraceID == "" {
		return nil, types.GetTraceErrorsOutput{}, errors.New("trace_id is required")
	}

	// Parse trace ID
	traceID, err := parseTraceID(input.TraceID)
	if err != nil {
		return nil, types.GetTraceErrorsOutput{}, fmt.Errorf("invalid trace_id: %w", err)
	}

	// Fetch trace
	// RawTraces=false means each returned ptrace.Traces contains a complete, aggregated trace
	// (not split across multiple consecutive ptrace.Traces objects).
	params := querysvc.GetTraceParams{
		TraceIDs: []tracestore.GetTraceParams{
			{TraceID: traceID},
		},
		RawTraces: false, // We want adjusted traces
	}

	tracesIter := h.queryService.GetTraces(ctx, params)

	// Collect spans with error status
	var errorSpans []types.SpanDetail
	var processErr error
	traceFound := false
	aggregatedTrace := ptrace.NewTraces()

	tracesIter(func(traces []ptrace.Traces, err error) bool {
		if err != nil {
			// For singular lookups, store error and abort
			processErr = err
			return false
		}

		for _, trace := range traces {
			traceFound = true
			// Merge this trace into our aggregated trace in case of multiple iterations
			if aggregatedTrace.SpanCount() == 0 {
				trace.CopyTo(aggregatedTrace)
			} else {
				jptrace.MergeTraces(aggregatedTrace, trace)
			}
		}

		return true
	})

	// If we encountered an error, return it directly
	if processErr != nil {
		return nil, types.GetTraceErrorsOutput{}, fmt.Errorf("failed to get trace: %w", processErr)
	}

	if !traceFound {
		return nil, types.GetTraceErrorsOutput{}, errors.New("trace not found")
	}

	// Iterate through all spans in the aggregated trace
	jptrace.SpanIter(aggregatedTrace)(func(pos jptrace.SpanIterPos, span ptrace.Span) bool {
		// Check if span has error status
		if span.Status().Code() == ptrace.StatusCodeError {
			detail := buildSpanDetail(pos, span)
			errorSpans = append(errorSpans, detail)
		}

		return true
	})

	output := types.GetTraceErrorsOutput{
		TraceID:    input.TraceID,
		ErrorCount: len(errorSpans),
		Spans:      errorSpans,
	}

	return nil, output, nil
}
