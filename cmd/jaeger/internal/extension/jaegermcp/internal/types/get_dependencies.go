// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// GetDependenciesInput defines the input parameters for the get_service_dependencies MCP tool.
type GetDependenciesInput struct {
	// StartTime is the start of the time range (optional, defaults to "-24h").
	// Supports RFC3339 or relative time (e.g., "-1h", "-24h").
	StartTime string `json:"start_time,omitempty" jsonschema:"Start of time range (RFC3339 or relative like -24h). Default: -24h"`

	// EndTime is the end of the time range (optional, defaults to "now").
	// Supports RFC3339 or relative time (e.g., "now", "-1h").
	EndTime string `json:"end_time,omitempty" jsonschema:"End of time range (RFC3339 or relative like now). Default: now"`
}

// GetDependenciesOutput defines the output of the get_service_dependencies MCP tool.
type GetDependenciesOutput struct {
	Dependencies []DependencyLink `json:"dependencies" jsonschema:"Service-to-service dependency edges with call counts"`
}

// DependencyLink represents a dependency between two services.
type DependencyLink struct {
	Caller    string `json:"caller" jsonschema:"Calling service name"`
	Callee    string `json:"callee" jsonschema:"Called service name"`
	CallCount uint64 `json:"call_count" jsonschema:"Number of calls from caller to callee in the time window"`
}
