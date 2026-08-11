// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

// ReadSkillInput defines the input parameters for the read_skill MCP tool.
type ReadSkillInput struct {
	Path string `json:"path" jsonschema:"Relative path within the skills directory (e.g. SKILL.md or detect-n-plus-one/SKILL.md)."`
}

// ReadSkillOutput defines the output of the read_skill MCP tool. It carries
// only metadata: the skill body travels in the result's content block as
// unescaped markdown, which is the form an agent reads. Repeating it here would
// put the same bytes on the wire a second time, JSON-escaped, because the SDK
// marshals this struct into StructuredContent verbatim.
type ReadSkillOutput struct {
	// Path echoes the skill that was served, so a caller reading several skills
	// can attribute a result without tracking request ids.
	Path string `json:"path"`
	// Truncated reports that the file exceeded the server's max_read_file_size
	// and the content block holds only its leading bytes.
	Truncated bool `json:"truncated"`
}
