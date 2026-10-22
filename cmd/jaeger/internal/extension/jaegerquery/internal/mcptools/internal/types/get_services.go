// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// GetServicesInput defines the input parameters for the get_services MCP tool.
type GetServicesInput struct {
	// Pattern is an optional regex pattern to filter services by name.
	// If empty, all services are returned.
	Pattern string `json:"pattern,omitempty" jsonschema:"Optional regex pattern to filter service names"`

	// Limit is the maximum number of services to return (default: 100).
	Limit int `json:"limit,omitempty" jsonschema:"Maximum number of services to return (default: 100)"`
}

// GetServicesOutput defines the output of the get_services MCP tool.
type GetServicesOutput struct {
	Services []string `json:"services"`

	// TotalCount is the number of services that matched before the limit was applied.
	TotalCount int `json:"total_count"`

	// Truncated is true when the limit dropped some matching services from the result.
	Truncated bool `json:"truncated"`
}
