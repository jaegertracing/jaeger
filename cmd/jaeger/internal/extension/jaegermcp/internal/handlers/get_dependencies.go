// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	model "github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
)

const defaultDependencyLookback = 24 * time.Hour

// queryServiceGetDependenciesInterface defines the interface needed from QueryService for testing.
type queryServiceGetDependenciesInterface interface {
	GetDependencies(ctx context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error)
}

type getDependenciesHandler struct {
	queryService queryServiceGetDependenciesInterface
}

// NewGetDependenciesHandler creates a new get_service_dependencies handler.
func NewGetDependenciesHandler(
	queryService *querysvc.QueryService,
) mcp.ToolHandlerFor[types.GetDependenciesInput, types.GetDependenciesOutput] {
	h := &getDependenciesHandler{
		queryService: queryService,
	}
	return h.handle
}

func (h *getDependenciesHandler) handle(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input types.GetDependenciesInput,
) (*mcp.CallToolResult, types.GetDependenciesOutput, error) {
	lookback := defaultDependencyLookback
	if input.Lookback != "" {
		parsed, err := time.ParseDuration(input.Lookback)
		if err != nil {
			return nil, types.GetDependenciesOutput{}, fmt.Errorf("invalid lookback: %w", err)
		}
		if parsed <= 0 {
			return nil, types.GetDependenciesOutput{}, fmt.Errorf("lookback must be positive, got %s", parsed)
		}
		lookback = parsed
	}

	endTs := time.Now()
	deps, err := h.queryService.GetDependencies(ctx, endTs, lookback)
	if err != nil {
		return nil, types.GetDependenciesOutput{}, fmt.Errorf("failed to get dependencies: %w", err)
	}

	links := make([]types.DependencyLink, 0, len(deps))
	for _, d := range deps {
		links = append(links, types.DependencyLink{
			Parent:    d.Parent,
			Child:     d.Child,
			CallCount: d.CallCount,
			Source:    d.Source,
		})
	}

	return nil, types.GetDependenciesOutput{
		Dependencies: links,
	}, nil
}
