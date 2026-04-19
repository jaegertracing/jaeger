// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	model "github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
)

// queryServiceGetDependenciesInterface defines the interface we need from QueryService for testing.
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
	endTime, err := parseDependencyTime(input.EndTime, time.Now())
	if err != nil {
		return nil, types.GetDependenciesOutput{}, fmt.Errorf("invalid end_time: %w", err)
	}

	defaultStart := endTime.Add(-24 * time.Hour)
	startTime, err := parseDependencyTime(input.StartTime, defaultStart)
	if err != nil {
		return nil, types.GetDependenciesOutput{}, fmt.Errorf("invalid start_time: %w", err)
	}

	if !startTime.Before(endTime) {
		return nil, types.GetDependenciesOutput{}, errors.New("start_time must be before end_time")
	}

	lookback := endTime.Sub(startTime)
	deps, err := h.queryService.GetDependencies(ctx, endTime, lookback)
	if err != nil {
		return nil, types.GetDependenciesOutput{}, fmt.Errorf("failed to get dependencies: %w", err)
	}

	links := make([]types.DependencyLink, 0, len(deps))
	for _, d := range deps {
		links = append(links, types.DependencyLink{
			Caller:    d.Parent,
			Callee:    d.Child,
			CallCount: d.CallCount,
		})
	}

	// Sort by caller then callee for consistent ordering
	slices.SortFunc(links, func(a, b types.DependencyLink) int {
		if c := cmp.Compare(a.Caller, b.Caller); c != 0 {
			return c
		}
		return cmp.Compare(a.Callee, b.Callee)
	})

	return nil, types.GetDependenciesOutput{
		Dependencies: links,
	}, nil
}

// parseDependencyTime parses a time string, returning the default if the input is empty.
func parseDependencyTime(input string, defaultTime time.Time) (time.Time, error) {
	if input == "" {
		return defaultTime, nil
	}
	return parseTimeParam(input)
}
