// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// ReadSkillInput defines the input parameters for the read_skill MCP tool.
type ReadSkillInput struct {
	Path string `json:"path" jsonschema:"Relative path within the skills directory (e.g. SKILL.md or detect-n-plus-one/SKILL.md)."`
}

// ReadSkillOutput defines the output of the read_skill MCP tool.
type ReadSkillOutput struct {
	Instructions string `json:"instructions"`
}
