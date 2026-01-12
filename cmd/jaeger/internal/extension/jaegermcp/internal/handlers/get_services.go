// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"fmt"
	"regexp"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
)

const defaultServiceLimit = 100

// queryServiceGetServicesInterface defines the interface we need from QueryService for testing
type queryServiceGetServicesInterface interface {
	GetServices(ctx context.Context) ([]string, error)
}

// getServicesHandler implements the get_services MCP tool.
// This tool lists available service names, optionally filtered by a regex pattern.
type getServicesHandler struct {
	queryService queryServiceGetServicesInterface
}

// NewGetServicesHandler creates a new get_services handler and returns the handler function.
func NewGetServicesHandler(
	queryService *querysvc.QueryService,
) mcp.ToolHandlerFor[types.GetServicesInput, types.GetServicesOutput] {
	h := &getServicesHandler{
		queryService: queryService,
	}
	return h.handle
}

// handle processes the get_services tool request.
func (h *getServicesHandler) handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.GetServicesInput,
) (*mcp.CallToolResult, types.GetServicesOutput, error) {
	// Get all services from storage
	services, err := h.queryService.GetServices(ctx)
	if err != nil {
		return nil, types.GetServicesOutput{}, fmt.Errorf("failed to get services: %w", err)
	}

	// Apply pattern filter if provided
	if input.Pattern != "" {
		re, err := regexp.Compile(input.Pattern)
		if err != nil {
			return nil, types.GetServicesOutput{}, fmt.Errorf("invalid pattern: %w", err)
		}

		filtered := make([]string, 0, len(services))
		for _, service := range services {
			if re.MatchString(service) {
				filtered = append(filtered, service)
			}
		}
		services = filtered
	}

	// Sort services for consistent ordering
	sort.Strings(services)

	// Apply limit
	limit := input.Limit
	if limit <= 0 {
		limit = defaultServiceLimit
	}
	if len(services) > limit {
		services = services[:limit]
	}

	return nil, types.GetServicesOutput{
		Services: services,
	}, nil
}
