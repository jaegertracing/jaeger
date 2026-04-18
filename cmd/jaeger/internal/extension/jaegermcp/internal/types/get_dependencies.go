// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// GetDependenciesInput defines the input parameters for the get_service_dependencies MCP tool.
type GetDependenciesInput struct {
	// Lookback defines the time window to query for dependencies (e.g., "1h", "24h").
	// Defaults to "24h" if not specified.
	Lookback string `json:"lookback,omitempty" jsonschema:"Time window for dependency data (e.g. 1h, 24h). Default: 24h"`
}

// GetDependenciesOutput defines the output of the get_service_dependencies MCP tool.
type GetDependenciesOutput struct {
	Dependencies []DependencyLink `json:"dependencies" jsonschema:"Service-to-service dependency edges with call counts"`
}

// DependencyLink represents a dependency between two services.
type DependencyLink struct {
	Parent    string `json:"parent" jsonschema:"Calling service name"`
	Child     string `json:"child" jsonschema:"Called service name"`
	CallCount uint64 `json:"call_count" jsonschema:"Number of calls from parent to child in the time window"`
	Source    string `json:"source,omitempty" jsonschema:"Source of the dependency data"`
}
