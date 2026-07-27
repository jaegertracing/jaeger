// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeSkillFrontmatter(t *testing.T) {
	t.Run("valid with all spec fields", func(t *testing.T) {
		fm, err := decodeSkillFrontmatter([]byte(`---
name: slow-db-call
description: Finds slow database calls.
license: Apache-2.0
metadata:
  author: jaeger
compatibility: requires Jaeger v2 with MCP enabled
allowed-tools: search_traces read_skill
---

# Body
`))
		require.NoError(t, err)
		assert.Equal(t, "slow-db-call", fm.Name)
		assert.Equal(t, "Finds slow database calls.", fm.Description)
		assert.Equal(t, "Apache-2.0", fm.License)
		assert.Equal(t, map[string]string{"author": "jaeger"}, fm.Metadata)
		assert.Equal(t, "requires Jaeger v2 with MCP enabled", fm.Compatibility)
		assert.Equal(t, "search_traces read_skill", fm.AllowedTools)
	})

	t.Run("missing frontmatter block", func(t *testing.T) {
		_, err := decodeSkillFrontmatter([]byte("# Just markdown\n"))
		require.ErrorContains(t, err, "missing")
	})

	t.Run("unterminated frontmatter", func(t *testing.T) {
		_, err := decodeSkillFrontmatter([]byte("---\nname: x\n"))
		require.ErrorContains(t, err, "not terminated")
	})

	t.Run("unknown key rejected by strict decoding", func(t *testing.T) {
		_, err := decodeSkillFrontmatter([]byte("---\nname: x\nnot_a_real_field: oops\n---\nbody\n"))
		require.ErrorContains(t, err, "not_a_real_field")
	})

	t.Run("CRLF line endings are normalized", func(t *testing.T) {
		fm, err := decodeSkillFrontmatter([]byte("---\r\nname: crlf-skill\r\ndescription: Authored on Windows.\r\n---\r\n\r\nBody\r\n"))
		require.NoError(t, err)
		assert.Equal(t, "crlf-skill", fm.Name)
		assert.Equal(t, "Authored on Windows.", fm.Description)
	})

	t.Run("embedded bare dashes mid-value do not truncate the block early", func(t *testing.T) {
		// A folded double-quoted YAML string can legitimately span raw lines;
		// one of those lines starts with "---" but isn't a full delimiter
		// line (it has trailing content), so it must not be mistaken for the
		// real terminator that follows.
		fm, err := decodeSkillFrontmatter([]byte("---\nname: x\ndescription: \"line1\n---trailing\nline2\"\n---\n\nbody\n"))
		require.NoError(t, err)
		assert.Equal(t, "x", fm.Name)
		assert.Equal(t, "line1 ---trailing line2", fm.Description)
	})

	// KnownFields(true) only guards the first Decode call. A "..." document-end
	// marker inside the block starts a second document whose keys strict
	// decoding would otherwise never see — here "unknown" would be silently
	// accepted. (A "---" separator cannot do this: it terminates the
	// frontmatter block itself, so what follows is body, not frontmatter.)
	t.Run("trailing document after ... terminator is rejected", func(t *testing.T) {
		_, err := decodeSkillFrontmatter([]byte("---\nname: good\ndescription: good\n...\nunknown: ignored\n---\nbody\n"))
		require.Error(t, err)
	})
}

func TestParseSkillFrontmatter(t *testing.T) {
	registered := map[string]bool{"read_skill": true}

	t.Run("valid frontmatter passes both decode and validation", func(t *testing.T) {
		fm, err := parseSkillFrontmatter([]byte(validSkillMD("good-skill")), "good-skill", registered)
		require.NoError(t, err)
		assert.Equal(t, "good-skill", fm.Name)
	})

	// parse must not be callable without validation happening — a decodable
	// but invalid skill has to fail here, not slip through to a caller who
	// forgot a separate validate step.
	t.Run("decodable but invalid frontmatter still fails", func(t *testing.T) {
		_, err := parseSkillFrontmatter([]byte(validSkillMD("good-skill")), "other-dir", registered)
		require.ErrorContains(t, err, `must match its directory name "other-dir"`)
	})

	t.Run("undecodable frontmatter fails before validation", func(t *testing.T) {
		_, err := parseSkillFrontmatter([]byte("no frontmatter here\n"), "any", registered)
		require.ErrorContains(t, err, "missing")
	})
}

func TestValidateSkillFrontmatter(t *testing.T) {
	registered := map[string]bool{"search_traces": true, "read_skill": true}
	valid := skillFrontmatter{Name: "good-skill", Description: "d"}

	tests := []struct {
		name    string
		mutate  func(*skillFrontmatter)
		dirName string
		wantErr string
	}{
		{name: "valid", mutate: func(*skillFrontmatter) {}, dirName: "good-skill"},
		{
			name:    "missing name",
			mutate:  func(fm *skillFrontmatter) { fm.Name = "" },
			dirName: "good-skill", wantErr: "name is required",
		},
		{
			name:    "missing description",
			mutate:  func(fm *skillFrontmatter) { fm.Description = "" },
			dirName: "good-skill", wantErr: "description is required",
		},
		{
			name:    "oversize name",
			mutate:  func(fm *skillFrontmatter) { fm.Name = strings.Repeat("a", maxSkillNameLen+1) },
			dirName: "good-skill", wantErr: "name exceeds",
		},
		{
			name:    "uppercase in name",
			mutate:  func(fm *skillFrontmatter) { fm.Name = "Good-Skill" },
			dirName: "good-skill", wantErr: "lowercase",
		},
		{
			name:    "leading hyphen",
			mutate:  func(fm *skillFrontmatter) { fm.Name = "-good" },
			dirName: "good-skill", wantErr: "lowercase",
		},
		{
			name:    "trailing hyphen",
			mutate:  func(fm *skillFrontmatter) { fm.Name = "good-" },
			dirName: "good-skill", wantErr: "lowercase",
		},
		{
			name:    "consecutive hyphens",
			mutate:  func(fm *skillFrontmatter) { fm.Name = "good--skill" },
			dirName: "good-skill", wantErr: "lowercase",
		},
		{
			name:    "name differs from directory",
			mutate:  func(*skillFrontmatter) {},
			dirName: "other-dir", wantErr: `must match its directory name "other-dir"`,
		},
		{
			name:    "oversize description",
			mutate:  func(fm *skillFrontmatter) { fm.Description = strings.Repeat("d", maxSkillDescriptionLen+1) },
			dirName: "good-skill", wantErr: "description exceeds",
		},
		{
			// Multi-byte UTF-8: well within the character limit, but over it
			// in bytes, so this must not be rejected by a byte-length check.
			name:    "multi-byte description within character limit",
			mutate:  func(fm *skillFrontmatter) { fm.Description = strings.Repeat("日", maxSkillDescriptionLen/2) },
			dirName: "good-skill",
		},
		{
			name:    "compatibility at limit accepted",
			mutate:  func(fm *skillFrontmatter) { fm.Compatibility = strings.Repeat("c", maxSkillCompatibilityLen) },
			dirName: "good-skill",
		},
		{
			name:    "oversize compatibility",
			mutate:  func(fm *skillFrontmatter) { fm.Compatibility = strings.Repeat("c", maxSkillCompatibilityLen+1) },
			dirName: "good-skill", wantErr: "compatibility exceeds",
		},
		{
			name:    "allowed-tools with registered tools",
			mutate:  func(fm *skillFrontmatter) { fm.AllowedTools = "search_traces read_skill" },
			dirName: "good-skill",
		},
		{
			name:    "allowed-tools with unregistered tool",
			mutate:  func(fm *skillFrontmatter) { fm.AllowedTools = "search_traces no_such_tool" },
			dirName: "good-skill", wantErr: `unregistered tool "no_such_tool"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fm := valid
			tc.mutate(&fm)
			err := validateSkillFrontmatter(fm, tc.dirName, registered)
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
		})
	}

	t.Run("multiple problems aggregated", func(t *testing.T) {
		err := validateSkillFrontmatter(skillFrontmatter{AllowedTools: "ghost"}, "d", registered)
		require.ErrorContains(t, err, "name is required")
		require.ErrorContains(t, err, "description is required")
		require.ErrorContains(t, err, `unregistered tool "ghost"`)
	})
}
