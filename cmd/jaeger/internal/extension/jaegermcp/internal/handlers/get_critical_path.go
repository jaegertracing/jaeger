// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/criticalpath"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
)

// GetCriticalPathHandler handles the get_critical_path MCP tool
type GetCriticalPathHandler struct {
	queryService *querysvc.QueryService
}

// NewGetCriticalPathHandler creates a new handler for get_critical_path tool
func NewGetCriticalPathHandler(queryService *querysvc.QueryService) *GetCriticalPathHandler {
	return &GetCriticalPathHandler{
		queryService: queryService,
	}
}

// Handle processes the get_critical_path tool request
func (h *GetCriticalPathHandler) Handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.GetCriticalPathInput,
) (*mcp.CallToolResult, types.GetCriticalPathOutput, error) {
	// Validate input
	if input.TraceID == "" {
		return nil, types.GetCriticalPathOutput{}, errors.New("trace_id is required")
	}

	// Parse trace ID using v1 model parser
	v1TraceID, err := model.TraceIDFromString(input.TraceID)
	if err != nil {
		return nil, types.GetCriticalPathOutput{}, fmt.Errorf("invalid trace_id: %w", err)
	}

	// Convert to v2 TraceID
	traceID := v1adapter.FromV1TraceID(v1TraceID)

	// Fetch the trace
	getTraceParams := querysvc.GetTraceParams{
		TraceIDs: []tracestore.GetTraceParams{
			{TraceID: traceID},
		},
		RawTraces: false, // We want adjusted traces
	}

	// Get the trace iterator
	var trace ptrace.Traces
	var found bool

	for traces, err := range h.queryService.GetTraces(ctx, getTraceParams) {
		if err != nil {
			return nil, types.GetCriticalPathOutput{}, fmt.Errorf("failed to get trace: %w", err)
		}

		// We expect only one trace since we're querying by a single trace ID
		if len(traces) > 0 {
			trace = traces[0]
			found = true
			break
		}
	}

	if !found {
		return nil, types.GetCriticalPathOutput{}, fmt.Errorf("trace not found: %s", input.TraceID)
	}

	// Compute critical path
	criticalPathSections, err := criticalpath.ComputeCriticalPath(trace)
	if err != nil {
		return nil, types.GetCriticalPathOutput{}, fmt.Errorf("failed to compute critical path: %w", err)
	}

	// Build a map of spans for quick lookup and collect service names
	spanMap := jptrace.SpanMap(trace, func(span ptrace.Span) string {
		return span.SpanID().String()
	})

	// Build a map of span ID to service name
	serviceMap := make(map[string]string)
	var traceStartTime uint64
	var traceEndTime uint64

	for i := 0; i < trace.ResourceSpans().Len(); i++ {
		rs := trace.ResourceSpans().At(i)
		serviceName := "unknown"
		if serviceNameAttr, ok := rs.Resource().Attributes().Get("service.name"); ok {
			serviceName = serviceNameAttr.Str()
		}

		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				serviceMap[span.SpanID().String()] = serviceName

				// Track trace start and end times
				startTime := uint64(span.StartTimestamp()) / 1000 // Convert to microseconds
				endTime := uint64(span.EndTimestamp()) / 1000

				if traceStartTime == 0 || startTime < traceStartTime {
					traceStartTime = startTime
				}
				if endTime > traceEndTime {
					traceEndTime = endTime
				}
			}
		}
	}

	// Convert critical path sections to output format
	path := make([]types.CriticalPathSpan, 0, len(criticalPathSections))
	var criticalPathDuration uint64

	for _, section := range criticalPathSections {
		span, ok := spanMap[section.SpanID]
		if !ok {
			continue // Skip if span not found
		}

		// Get service name from service map
		serviceName := serviceMap[section.SpanID]

		selfTime := section.SectionEnd - section.SectionStart
		criticalPathDuration += selfTime

		path = append(path, types.CriticalPathSpan{
			SpanID:         section.SpanID,
			Service:        serviceName,
			Operation:      span.Name(),
			SelfTimeMs:     selfTime / 1000, // Convert microseconds to milliseconds
			SectionStartMs: (section.SectionStart - traceStartTime) / 1000,
			SectionEndMs:   (section.SectionEnd - traceStartTime) / 1000,
		})
	}

	output := types.GetCriticalPathOutput{
		TraceID:                input.TraceID,
		TotalDurationMs:        (traceEndTime - traceStartTime) / 1000,
		CriticalPathDurationMs: criticalPathDuration / 1000,
		Path:                   path,
	}

	return nil, output, nil
}
