// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

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

	t.Run("a --- inside a value does not end the block early", func(t *testing.T) {
		// A double-quoted YAML string folds across raw lines; one of them starts
		// with "---" but carries trailing content, so it is not a delimiter line
		// and must not be mistaken for the real terminator that follows.
		fm, err := decodeSkillFrontmatter([]byte("---\nname: x\ndescription: \"line1\n---trailing\nline2\"\n---\n\nbody\n"))
		require.NoError(t, err)
		assert.Equal(t, "line1 ---trailing line2", fm.Description)
	})

	// KnownFields guards only the first document, so a "..." terminator inside
	// the block would otherwise let a second document carry keys strict decoding
	// never sees — here "unknown" would be silently accepted.
	t.Run("second YAML document after ... is rejected", func(t *testing.T) {
		_, err := decodeSkillFrontmatter([]byte("---\nname: good\ndescription: good\n...\nunknown: ignored\n---\nbody\n"))
		require.Error(t, err)
	})

	t.Run("unparseable second document is reported", func(t *testing.T) {
		_, err := decodeSkillFrontmatter([]byte("---\nname: good\ndescription: good\n...\n[unbalanced\n---\nbody\n"))
		require.ErrorContains(t, err, "single YAML document")
	})
}

func TestValidateSkillFrontmatter(t *testing.T) {
	doc := string(skillDoc("good-skill", "# Body\n").Data)

	t.Run("valid frontmatter passes decode and validation", func(t *testing.T) {
		require.NoError(t, validateSkillFrontmatter([]byte(doc), "good-skill"))
	})

	// Decoding alone is not enough: the same document is invalid under a
	// different directory, and that has to surface.
	t.Run("decodable but invalid frontmatter still fails", func(t *testing.T) {
		err := validateSkillFrontmatter([]byte(doc), "other-dir")
		require.ErrorContains(t, err, `must match its directory name "other-dir"`)
	})

	t.Run("undecodable frontmatter fails before validation", func(t *testing.T) {
		err := validateSkillFrontmatter([]byte("no frontmatter here\n"), "any")
		require.ErrorContains(t, err, "missing")
	})
}

func TestSkillFrontmatterValidate(t *testing.T) {
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
			name:    "doubled hyphen",
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
			// Comfortably inside the character limit but past it in bytes, so a
			// byte-length check would wrongly reject it.
			name:    "multi-byte description within the character limit",
			mutate:  func(fm *skillFrontmatter) { fm.Description = strings.Repeat("日", maxSkillDescriptionLen/2) },
			dirName: "good-skill",
		},
		{
			name:    "compatibility at the limit",
			mutate:  func(fm *skillFrontmatter) { fm.Compatibility = strings.Repeat("c", maxSkillCompatibilityLen) },
			dirName: "good-skill",
		},
		{
			name:    "oversize compatibility",
			mutate:  func(fm *skillFrontmatter) { fm.Compatibility = strings.Repeat("c", maxSkillCompatibilityLen+1) },
			dirName: "good-skill", wantErr: "compatibility exceeds",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fm := valid
			tc.mutate(&fm)
			err := fm.validate(tc.dirName)
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
		})
	}

	t.Run("every problem is reported, not just the first", func(t *testing.T) {
		err := skillFrontmatter{Compatibility: strings.Repeat("c", maxSkillCompatibilityLen+1)}.validate("d")
		require.ErrorContains(t, err, "name is required")
		require.ErrorContains(t, err, "description is required")
		require.ErrorContains(t, err, "compatibility exceeds")
	})
}
