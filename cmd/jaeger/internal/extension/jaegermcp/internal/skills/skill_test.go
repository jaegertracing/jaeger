// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package skills

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_HappyPath(t *testing.T) {
	data := []byte(`---
name: analyze-critical-path
description: Find the critical latency path through a trace.
version: "1.0"
author: jaeger-team
category: performance
allowed_tools:
  - get_trace_topology
  - get_critical_path
model_hints:
  temperature: "0.2"
references:
  - reference.md
---
# Analyze Critical Path

1. Call ` + "`get_trace_topology`" + ` to get the trace structure.
2. Call ` + "`get_critical_path`" + ` to find the bottleneck.

- bullet one
- bullet two

` + "```go\nfmt.Println(\"example\")\n```\n")

	skill, err := Parse(data, "skills/analyze-critical-path/SKILL.md")
	require.NoError(t, err)

	assert.Equal(t, "analyze-critical-path", skill.Name)
	assert.Equal(t, "Find the critical latency path through a trace.", skill.Description)
	assert.Equal(t, "1.0", skill.Version)
	assert.Equal(t, "jaeger-team", skill.Author)
	assert.Equal(t, "performance", skill.Category)
	assert.Equal(t, []string{"get_trace_topology", "get_critical_path"}, skill.AllowedTools)
	assert.Equal(t, map[string]string{"temperature": "0.2"}, skill.ModelHints)
	assert.Equal(t, []string{"reference.md"}, skill.References)
	assert.Equal(t, "skills/analyze-critical-path/SKILL.md", skill.SourcePath)

	assert.Contains(t, skill.Body, "# Analyze Critical Path")
	assert.Contains(t, skill.Body, "1. Call `get_trace_topology`")
	assert.Contains(t, skill.Body, "- bullet one")
	assert.Contains(t, skill.Body, "```go")
	assert.Contains(t, skill.Body, `fmt.Println("example")`)
	assert.NotContains(t, skill.Body, "\n\n\n")
}

func TestParse_EmptyBody(t *testing.T) {
	data := []byte(`---
name: noop-skill
description: A skill with no body.
allowed_tools:
  - get_services
---
`)

	skill, err := Parse(data, "skills/noop-skill/SKILL.md")
	require.NoError(t, err)
	assert.Equal(t, "noop-skill", skill.Name)
	assert.Empty(t, skill.Body)
}

func TestParse_MissingOpeningDelimiter(t *testing.T) {
	data := []byte(`name: missing-opening
description: No opening delimiter.
allowed_tools:
  - get_services
---
Body text.
`)

	_, err := Parse(data, "skills/bad/SKILL.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skills/bad/SKILL.md")
	assert.Contains(t, err.Error(), "frontmatter delimiter")
}

func TestParse_MissingClosingDelimiter(t *testing.T) {
	data := []byte(`---
name: missing-closing
description: No closing delimiter.
allowed_tools:
  - get_services
Body text.
`)

	_, err := Parse(data, "skills/bad/SKILL.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skills/bad/SKILL.md")
	assert.Contains(t, err.Error(), "closing")
}

func TestParse_FrontmatterSyntaxError(t *testing.T) {
	data := []byte(`---
name: bad-yaml
description: [unterminated
allowed_tools:
  - get_services
---
Body text.
`)

	_, err := Parse(data, "skills/bad/SKILL.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skills/bad/SKILL.md")
}

func TestParse_MissingName(t *testing.T) {
	data := []byte(`---
description: Missing the name field.
allowed_tools:
  - get_services
---
Body text.
`)

	_, err := Parse(data, "skills/bad/SKILL.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skills/bad/SKILL.md")
	assert.Contains(t, err.Error(), "name")
}

func TestParse_MissingDescription(t *testing.T) {
	data := []byte(`---
name: missing-description
allowed_tools:
  - get_services
---
Body text.
`)

	_, err := Parse(data, "skills/bad/SKILL.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skills/bad/SKILL.md")
	assert.Contains(t, err.Error(), "description")
}

func TestParse_MissingAllowedTools(t *testing.T) {
	data := []byte(`---
name: missing-allowed-tools
description: Missing allowed_tools entirely.
---
Body text.
`)

	_, err := Parse(data, "skills/bad/SKILL.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skills/bad/SKILL.md")
	assert.Contains(t, err.Error(), "missing required field")
	assert.Contains(t, err.Error(), "allowed_tools")
}

func TestParse_EmptyAllowedTools(t *testing.T) {
	data := []byte(`---
name: empty-allowed-tools
description: allowed_tools is present but empty.
allowed_tools: []
---
Body text.
`)

	_, err := Parse(data, "skills/bad/SKILL.md")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skills/bad/SKILL.md")
	assert.Contains(t, err.Error(), "must contain at least one entry")
	assert.Contains(t, err.Error(), "allowed_tools")
}
