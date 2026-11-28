// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSkill_FrontMatterAndBody(t *testing.T) {
	raw := `---
name: error-root-cause
description: Walk a failed trace to the first originating error span.
license: Apache-2.0
metadata:
  author: jaegertracing
  version: "1.0"
allowed-tools: get_trace_errors get_trace_topology get_span_details
---

# Error Root Cause Analysis

## Procedure

1. Use ` + "`get_trace_errors`" + ` to list all error spans.`

	skill, err := ParseSkill(raw)
	require.NoError(t, err)

	assert.Equal(t, "error-root-cause", skill.FrontMatter.Name)
	assert.Equal(t, "Apache-2.0", skill.FrontMatter.License)
	assert.Equal(t, "get_trace_errors get_trace_topology get_span_details", skill.FrontMatter.AllowedTools)
	assert.Equal(t, "jaegertracing", skill.FrontMatter.Metadata["author"])
	assert.Equal(t, "1.0", skill.FrontMatter.Metadata["version"])
	assert.Contains(t, skill.FrontMatter.Description, "Walk a failed trace")

	assert.NotEmpty(t, skill.Content)
	assert.Contains(t, skill.Content, "# Error Root Cause Analysis")
	assert.NotContains(t, skill.Content, "license")
	assert.NotContains(t, skill.Content, "allowed-tools")
}

func TestParseSkill(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantContent string
		wantName    string
	}{
		{
			name:        "block removed",
			raw:         "---\nname: a\n---\n# Body",
			wantContent: "# Body",
			wantName:    "a",
		},
		{
			name:        "no block is returned whole",
			raw:         "# Body\n\nNo front matter here.",
			wantContent: "# Body\n\nNo front matter here.",
		},
		{
			name:        "unterminated block is returned whole",
			raw:         "---\nname: a\n# Body",
			wantContent: "---\nname: a\n# Body",
		},
		{
			name:        "empty body",
			raw:         "---\nname: a\n---\n",
			wantContent: "",
			wantName:    "a",
		},
		{
			name:        "horizontal rule in body survives",
			raw:         "---\nname: a\n---\n# Body\n\n---\n\nMore.",
			wantContent: "# Body\n\n---\n\nMore.",
			wantName:    "a",
		},
		{
			name:        "CRLF line endings are recognized and the body keeps its own",
			raw:         "---\r\nname: a\r\n---\r\n# Body\r\n\r\nText.\r\n",
			wantContent: "# Body\r\n\r\nText.\r\n",
			wantName:    "a",
		},
		{
			name:        "a longer dash line is not a closing delimiter",
			raw:         "---\nname: a\n----\n# Body",
			wantContent: "---\nname: a\n----\n# Body",
		},
		{
			name:        "closing delimiter at the end of the file without a newline",
			raw:         "---\nname: a\n---",
			wantContent: "",
			wantName:    "a",
		},
		{
			name:        "unterminated block without a trailing newline is returned whole",
			raw:         "---\nname: a",
			wantContent: "---\nname: a",
		},
		{
			name:        "delimiter not at start is returned whole",
			raw:         "# Body\n---\nname: a\n---\n",
			wantContent: "# Body\n---\nname: a\n---\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skill, err := ParseSkill(tt.raw)
			require.NoError(t, err)
			assert.Equal(t, tt.wantContent, skill.Content)
			assert.Equal(t, tt.wantName, skill.FrontMatter.Name)
		})
	}
}

func TestParseSkill_MalformedYAML(t *testing.T) {
	_, err := ParseSkill("---\nname: [unclosed\n---\n# Body")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid front matter")
}

// A line that only starts with the delimiter belongs to the front matter block, so the
// block is parsed up to the next whole delimiter line.
func TestParseSkill_DelimiterPrefixStaysInBlock(t *testing.T) {
	_, err := ParseSkill("---\nname: a\n---not-a-delimiter\n---\n# Body")
	require.ErrorContains(t, err, "invalid front matter")

	skill, err := ParseSkill("---\nname: a\n# ---not-a-delimiter\n---\n# Body")
	require.NoError(t, err)
	assert.Equal(t, "a", skill.FrontMatter.Name)
	assert.Equal(t, "# Body", skill.Content)
}
