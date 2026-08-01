// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
)

const testMaxFileSize = 100

// skillDoc builds a SKILL.md with valid frontmatter naming dir.
func skillDoc(dir, body string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(
		"---\nname: " + dir + "\ndescription: When to use " + dir + ".\n---\n\n" + body,
	)}
}

func testSkillsFS() fstest.MapFS {
	return fstest.MapFS{
		"SKILL.md":         &fstest.MapFile{Data: []byte("# Skills\n\n- skill-a\n- skill-b\n")},
		"skill-a/SKILL.md": skillDoc("skill-a", "# Skill A\n\nContent here."),
		"skill-b/SKILL.md": skillDoc("skill-b", "# Skill B\n\nMore content."),
		"large.bin":        &fstest.MapFile{Data: make([]byte, testMaxFileSize+10)},
	}
}

func newTestHandler() *readSkillHandler {
	return &readSkillHandler{builtins: testSkillsFS(), maxFileSize: testMaxFileSize}
}

func TestReadSkillHandler_RootSkillMD(t *testing.T) {
	h := newTestHandler()
	_, output, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "SKILL.md"})
	require.NoError(t, err)
	assert.Contains(t, output.Instructions, "# Skills")
	assert.Contains(t, output.Instructions, "skill-a")
}

func TestReadSkillHandler_SubSkillMD(t *testing.T) {
	h := newTestHandler()
	_, output, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "skill-a/SKILL.md"})
	require.NoError(t, err)
	assert.Contains(t, output.Instructions, "# Skill A")
}

func TestReadSkillHandler_InvalidPaths(t *testing.T) {
	h := newTestHandler()
	tests := []struct {
		name string
		path string
	}{
		{"empty", ""},
		{"traversal", "../etc/passwd"},
		{"absolute", "/etc/passwd"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: tt.path})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cannot read")
		})
	}
}

func TestReadSkillHandler_FileNotFound(t *testing.T) {
	h := newTestHandler()
	_, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "nonexistent/SKILL.md"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read")
}

func TestReadSkillHandler_Directory(t *testing.T) {
	h := newTestHandler()
	_, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "skill-a"})
	require.Error(t, err)
}

func TestReadSkillHandler_FileTooLarge(t *testing.T) {
	h := newTestHandler()
	_, output, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "large.bin"})
	require.NoError(t, err)
	assert.Contains(t, output.Instructions, "truncated after")
}

func TestReadSkillHandler_RawTextInContent(t *testing.T) {
	h := newTestHandler()
	result, output, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "SKILL.md"})
	require.NoError(t, err)
	require.NotNil(t, result)
	tc, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "# Skills")
	assert.Equal(t, tc.Text, output.Instructions)
}

func TestNewReadSkillHandler(t *testing.T) {
	handler := NewReadSkillHandler(testSkillsFS(), nil, testMaxFileSize)
	assert.NotNil(t, handler)
}

func testCustomFS() fstest.MapFS {
	return fstest.MapFS{
		"SKILL.md":              &fstest.MapFile{Data: []byte("# Operator entry point")},
		"slow-db-call/SKILL.md": skillDoc("slow-db-call", "# Slow DB Call"),
	}
}

// With no skills_dir configured every custom/ path must report not-exist
// rather than falling through to the built-ins.
func TestReadSkillHandler_CustomPathWithoutOperatorFS(t *testing.T) {
	h := newTestHandler()
	for _, p := range []string{"custom/SKILL.md", "custom/slow-db-call/SKILL.md"} {
		t.Run(p, func(t *testing.T) {
			_, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: p})
			require.ErrorIs(t, err, fs.ErrNotExist)
		})
	}
}

func TestReadSkillHandler_DispatchesByPrefix(t *testing.T) {
	h := &readSkillHandler{builtins: testSkillsFS(), custom: testCustomFS(), maxFileSize: testMaxFileSize}

	t.Run("custom prefix reaches the custom tree", func(t *testing.T) {
		_, out, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "custom/slow-db-call/SKILL.md"})
		require.NoError(t, err)
		assert.Contains(t, out.Instructions, "# Slow DB Call")
	})

	t.Run("custom entry point is served", func(t *testing.T) {
		_, out, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "custom/SKILL.md"})
		require.NoError(t, err)
		assert.Equal(t, "# Operator entry point", out.Instructions)
	})

	t.Run("built-ins still reachable at the root", func(t *testing.T) {
		_, out, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "skill-a/SKILL.md"})
		require.NoError(t, err)
		assert.Contains(t, out.Instructions, "# Skill A")
	})

	t.Run("traversal out of custom is rejected", func(t *testing.T) {
		_, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "custom/../etc/passwd"})
		require.Error(t, err)
	})
}

// An invalid skill must fail the read that asked for it, not the whole tree:
// every other skill keeps serving.
func TestReadSkillHandler_RejectsInvalidSkillOnRead(t *testing.T) {
	custom := fstest.MapFS{
		"SKILL.md":                &fstest.MapFile{Data: []byte("# Operator entry point")},
		"good/SKILL.md":           skillDoc("good", "# Good"),
		"mismatched/SKILL.md":     skillDoc("something-else", "# Mismatched"),
		"no-frontmatter/SKILL.md": &fstest.MapFile{Data: []byte("# No frontmatter")},
		"typo/SKILL.md":           &fstest.MapFile{Data: []byte("---\nname: typo\nunknown_key: oops\n---\n")},
	}
	h := &readSkillHandler{builtins: testSkillsFS(), custom: custom, maxFileSize: 1 << 16}

	tests := []struct {
		path    string
		wantErr string
	}{
		{"custom/mismatched/SKILL.md", `must match its directory name "mismatched"`},
		{"custom/no-frontmatter/SKILL.md", `missing "---" frontmatter block`},
		{"custom/typo/SKILL.md", "unknown_key"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			_, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: tt.path})
			require.ErrorContains(t, err, tt.wantErr)
			require.ErrorContains(t, err, "invalid skill")
		})
	}

	t.Run("the valid skill beside them still serves", func(t *testing.T) {
		_, out, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "custom/good/SKILL.md"})
		require.NoError(t, err)
		assert.Contains(t, out.Instructions, "# Good")
	})
}

// Only <dir>/SKILL.md is a skill in its own right. An entry point has no
// directory to be named after, and a file nested deeper is documentation
// inside a skill — neither carries the name/directory rule.
func TestReadSkillHandler_ValidatesOnlyTopLevelSkills(t *testing.T) {
	custom := fstest.MapFS{
		// No frontmatter at all: fine for both, an error for a skill.
		"SKILL.md":                    &fstest.MapFile{Data: []byte("# Entry point")},
		"a-skill/SKILL.md":            skillDoc("a-skill", "# A"),
		"a-skill/reference/SKILL.md":  &fstest.MapFile{Data: []byte("# Nested reference")},
		"a-skill/examples/queries.md": &fstest.MapFile{Data: []byte("# Examples")},
	}
	h := &readSkillHandler{builtins: testSkillsFS(), custom: custom, maxFileSize: 1 << 16}

	for _, p := range []string{
		"custom/SKILL.md",
		"custom/a-skill/reference/SKILL.md",
		"custom/a-skill/examples/queries.md",
	} {
		t.Run(p, func(t *testing.T) {
			_, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: p})
			require.NoError(t, err)
		})
	}
}

// Skills are read from disk per request, so an edit takes effect without a
// restart — the reason validation lives here rather than in a startup scan.
func TestReadSkillHandler_PicksUpEditsWithoutRestart(t *testing.T) {
	file := skillDoc("live", "# Before")
	h := &readSkillHandler{
		builtins:    testSkillsFS(),
		custom:      fstest.MapFS{"live/SKILL.md": file},
		maxFileSize: 1 << 16,
	}

	_, out, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "custom/live/SKILL.md"})
	require.NoError(t, err)
	assert.Contains(t, out.Instructions, "# Before")

	file.Data = skillDoc("live", "# After").Data
	_, out, err = h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "custom/live/SKILL.md"})
	require.NoError(t, err)
	assert.Contains(t, out.Instructions, "# After")

	// An edit that breaks the frontmatter is caught on the next read too.
	file.Data = skillDoc("renamed", "# Broken").Data
	_, _, err = h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "custom/live/SKILL.md"})
	require.ErrorContains(t, err, "invalid skill")
}

func TestSkillDirName(t *testing.T) {
	tests := []struct {
		path    string
		wantDir string
		wantOK  bool
	}{
		{"skill-a/SKILL.md", "skill-a", true},
		{"custom/skill-a/SKILL.md", "skill-a", true},
		{"SKILL.md", "", false},
		{"custom/SKILL.md", "", false},
		{"skill-a/nested/SKILL.md", "", false},
		{"skill-a/notes.md", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			dir, ok := skillDirName(tt.path)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantDir, dir)
		})
	}
}
