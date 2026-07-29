// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"strings"
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
	return &readSkillHandler{skillsFS: testSkillsFS(), maxFileSize: testMaxFileSize}
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
	assert.Equal(t, "# Skill A\n\nContent here.", output.Instructions)
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
	handler := NewReadSkillHandler(testSkillsFS(), testMaxFileSize)
	assert.NotNil(t, handler)
}

func TestReadSkillHandler_SizeLimitBoundary(t *testing.T) {
	fsys := fstest.MapFS{
		"exactly.bin": &fstest.MapFile{Data: []byte(strings.Repeat("B", testMaxFileSize))},
		"over.bin":    &fstest.MapFile{Data: []byte(strings.Repeat("A", testMaxFileSize+1))},
	}
	h := &readSkillHandler{skillsFS: fsys, maxFileSize: testMaxFileSize}

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
