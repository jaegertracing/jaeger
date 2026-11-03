// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
)

const testMaxFileSize = 100

func testSkillsFS() fstest.MapFS {
	return fstest.MapFS{
		"SKILL.md":         &fstest.MapFile{Data: []byte("# Skills\n\n- skill-a\n- skill-b\n")},
		"skill-a/SKILL.md": &fstest.MapFile{Data: []byte("# Skill A\n\nContent here.")},
		"skill-b/SKILL.md": &fstest.MapFile{Data: []byte("# Skill B\n\nMore content.")},
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
	assert.Equal(t, "SKILL.md", output.Path)
	assert.False(t, output.Truncated)
}

func TestReadSkillHandler_SubSkillMD(t *testing.T) {
	h := newTestHandler()
	_, output, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "skill-a/SKILL.md"})
	require.NoError(t, err)
	assert.Equal(t, "skill-a/SKILL.md", output.Path)
	assert.False(t, output.Truncated)
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
	assert.True(t, output.Truncated)
}

func TestReadSkillHandler_RawTextInContent(t *testing.T) {
	h := newTestHandler()
	result, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "SKILL.md"})
	require.NoError(t, err)
	require.NotNil(t, result)
	tc, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, tc.Text, "# Skills")
}

func TestNewReadSkillHandler(t *testing.T) {
	handler := NewReadSkillHandler(testSkillsFS(), nil, testMaxFileSize)
	assert.NotNil(t, handler)
}

func testCustomFS() fstest.MapFS {
	return fstest.MapFS{
		"SKILL.md":              &fstest.MapFile{Data: []byte("# Operator catalog")},
		"slow-db-call/SKILL.md": &fstest.MapFile{Data: []byte("# Slow DB Call")},
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
		result, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "custom/slow-db-call/SKILL.md"})
		require.NoError(t, err)
		assert.Equal(t, "# Slow DB Call", result.Content[0].(*mcp.TextContent).Text)
	})

	t.Run("custom entry point is served", func(t *testing.T) {
		result, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "custom/SKILL.md"})
		require.NoError(t, err)
		assert.Equal(t, "# Operator catalog", result.Content[0].(*mcp.TextContent).Text)
	})

	t.Run("built-ins still reachable at the root", func(t *testing.T) {
		result, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "skill-a/SKILL.md"})
		require.NoError(t, err)
		assert.Equal(t, "# Skill A\n\nContent here.", result.Content[0].(*mcp.TextContent).Text)
	})

	t.Run("traversal out of custom is rejected", func(t *testing.T) {
		_, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "custom/../etc/passwd"})
		require.Error(t, err)
	})
}

func TestReadSkillHandler_SizeLimitBoundary(t *testing.T) {
	fsys := fstest.MapFS{
		"exactly.bin": &fstest.MapFile{Data: []byte(strings.Repeat("B", testMaxFileSize))},
		"over.bin":    &fstest.MapFile{Data: []byte(strings.Repeat("A", testMaxFileSize+1))},
	}
	h := &readSkillHandler{builtins: fsys, maxFileSize: testMaxFileSize}

	// Exactly at the limit: whole file served, no truncation notice.
	_, out, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "exactly.bin"})
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("B", testMaxFileSize), out.Instructions)
	assert.NotContains(t, out.Instructions, "truncated")

	// One byte over: content capped at exactly maxFileSize, then the notice.
	_, out, err = h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "over.bin"})
	require.NoError(t, err)
	idx := strings.Index(out.Instructions, "\n\nfile content truncated")
	require.NotEqual(t, -1, idx)
	assert.Equal(t, testMaxFileSize, idx)
}

// A cut landing inside a multi-byte UTF-8 rune must back off to that rune's
// start rather than split it, so truncation never serves invalid UTF-8.
func TestReadSkillHandler_TruncationBacksOffToRuneBoundary(t *testing.T) {
	const maxFileSize = 4
	// "café" is 5 bytes: "caf" (3 ASCII bytes) + the 2-byte encoding of 'é'.
	// A 4-byte cut lands on the second byte of 'é'.
	fsys := fstest.MapFS{
		"skill.md": &fstest.MapFile{Data: []byte("café")},
	}
	h := &readSkillHandler{builtins: fsys, maxFileSize: maxFileSize}

	_, out, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "skill.md"})
	require.NoError(t, err)

	require.True(t, utf8.ValidString(out.Instructions), "output must be valid UTF-8: %q", out.Instructions)
	idx := strings.Index(out.Instructions, "\n\nfile content truncated")
	require.NotEqual(t, -1, idx)
	// The rune straddling byte 4 is dropped entirely rather than split, so the
	// served content is "caf" (3 bytes), and the notice reports that count.
	assert.Equal(t, "caf", out.Instructions[:idx])
	assert.Contains(t, out.Instructions, "truncated after 3 bytes")
}
