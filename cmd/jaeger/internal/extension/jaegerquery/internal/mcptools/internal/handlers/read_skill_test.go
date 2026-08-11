// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"encoding/json"
	"io/fs"
	"testing"
	"testing/fstest"

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

// skillText returns the markdown an agent actually receives: the result's
// content block, which is the only place the skill body is carried.
func skillText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotNil(t, result)
	require.Len(t, result.Content, 1)
	tc, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	return tc.Text
}

func TestReadSkillHandler_RootSkillMD(t *testing.T) {
	h := newTestHandler()
	result, output, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "SKILL.md"})
	require.NoError(t, err)
	body := skillText(t, result)
	assert.Contains(t, body, "# Skills")
	assert.Contains(t, body, "skill-a")
	assert.Equal(t, "SKILL.md", output.Path)
	assert.False(t, output.Truncated)
}

func TestReadSkillHandler_SubSkillMD(t *testing.T) {
	h := newTestHandler()
	result, output, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "skill-a/SKILL.md"})
	require.NoError(t, err)
	assert.Equal(t, "# Skill A\n\nContent here.", skillText(t, result))
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
	result, output, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "large.bin"})
	require.NoError(t, err)
	assert.Contains(t, skillText(t, result), "truncated after")
	assert.True(t, output.Truncated)
}

func TestReadSkillHandler_RawTextInContent(t *testing.T) {
	h := newTestHandler()
	result, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "SKILL.md"})
	require.NoError(t, err)
	assert.Contains(t, skillText(t, result), "# Skills")
}

// Regression test for #9290. The SDK marshals the returned output into
// StructuredContent verbatim, so a body carried there as well as in the content
// block would travel twice — once as markdown, once JSON-escaped.
func TestReadSkillHandler_BodyNotRepeatedInStructuredOutput(t *testing.T) {
	h := newTestHandler()
	result, output, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "skill-a/SKILL.md"})
	require.NoError(t, err)

	assert.Equal(t, "# Skill A\n\nContent here.", skillText(t, result))

	structured, err := json.Marshal(output)
	require.NoError(t, err)
	assert.NotContains(t, string(structured), "Content here.")
	assert.JSONEq(t, `{"path":"skill-a/SKILL.md","truncated":false}`, string(structured))
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
		result, out, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "custom/slow-db-call/SKILL.md"})
		require.NoError(t, err)
		assert.Equal(t, "# Slow DB Call", skillText(t, result))
		assert.Equal(t, "custom/slow-db-call/SKILL.md", out.Path)
	})

	t.Run("custom entry point is served", func(t *testing.T) {
		result, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "custom/SKILL.md"})
		require.NoError(t, err)
		assert.Equal(t, "# Operator catalog", skillText(t, result))
	})

	t.Run("built-ins still reachable at the root", func(t *testing.T) {
		result, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "skill-a/SKILL.md"})
		require.NoError(t, err)
		assert.Equal(t, "# Skill A\n\nContent here.", skillText(t, result))
	})

	t.Run("traversal out of custom is rejected", func(t *testing.T) {
		_, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "custom/../etc/passwd"})
		require.Error(t, err)
	})
}
