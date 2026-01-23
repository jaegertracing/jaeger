// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

const defaultSpanNameLimit = 100

// queryServiceGetOperationsInterface defines the interface we need from QueryService for testing
type queryServiceGetOperationsInterface interface {
	GetOperations(ctx context.Context, query tracestore.OperationQueryParams) ([]tracestore.Operation, error)
}

// getSpanNamesHandler implements the get_span_names MCP tool.
// This tool lists available span names for a service, optionally filtered by pattern and span kind.
type getSpanNamesHandler struct {
	queryService queryServiceGetOperationsInterface
}

// NewGetSpanNamesHandler creates a new get_span_names handler and returns the handler function.
func NewGetSpanNamesHandler(
	queryService *querysvc.QueryService,
) mcp.ToolHandlerFor[types.GetSpanNamesInput, types.GetSpanNamesOutput] {
	h := &getSpanNamesHandler{
		queryService: queryService,
	}
	return h.handle
}

// handle processes the get_span_names request.
func (h *getSpanNamesHandler) handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.GetSpanNamesInput,
) (*mcp.CallToolResult, types.GetSpanNamesOutput, error) {
	// Validate service name
	if input.ServiceName == "" {
		return nil, types.GetSpanNamesOutput{}, errors.New("service_name is required")
	}

	// Set default limit
	limit := input.Limit
	if limit <= 0 {
		limit = defaultSpanNameLimit
	}

	// Build query parameters
	query := tracestore.OperationQueryParams{
		ServiceName: input.ServiceName,
		SpanKind:    input.SpanKind,
	}

	// Get operations from storage
	operations, err := h.queryService.GetOperations(ctx, query)
	if err != nil {
		return nil, types.GetSpanNamesOutput{}, fmt.Errorf("failed to get span names: %w", err)
	}

	// Filter by pattern if provided
	var filteredOps []tracestore.Operation
	if input.Pattern != "" {
		pattern, err := regexp.Compile(input.Pattern)
		if err != nil {
			return nil, types.GetSpanNamesOutput{}, fmt.Errorf("invalid pattern: %w", err)
		}
		for _, op := range operations {
			if pattern.MatchString(op.Name) {
				filteredOps = append(filteredOps, op)
			}
		}
	} else {
		filteredOps = operations
	}

	// Sort by name for consistent results
	sort.Slice(filteredOps, func(i, j int) bool {
		return filteredOps[i].Name < filteredOps[j].Name
	})

	// Apply limit
	if len(filteredOps) > limit {
		filteredOps = filteredOps[:limit]
	}

	// Build output
	var spanNames []types.SpanNameInfo
	for _, op := range filteredOps {
		spanNames = append(spanNames, types.SpanNameInfo{
			Name:     op.Name,
			SpanKind: op.SpanKind,
		})
	}

	return nil, types.GetSpanNamesOutput{SpanNames: spanNames}, nil
}
