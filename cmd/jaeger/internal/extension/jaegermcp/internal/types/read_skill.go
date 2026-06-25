// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// ReadSkillInput defines the input parameters for the read_skill MCP tool.
type ReadSkillInput struct {
	Path string `json:"path,omitempty" jsonschema:"Relative path within the skills directory. Empty returns a catalog of all available skills."`
}
