// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
)

const testMaxFileSize = 100

func testSkillsFS() fstest.MapFS {
	return fstest.MapFS{
		"SKILL.md":         &fstest.MapFile{Data: []byte("# Skills\n\n- skill-a\n- skill-b\n")},
		"skill-a/SKILL.md": &fstest.MapFile{Data: []byte("# Skill A\n\nContent here.")},
		"skill-b/SKILL.md": &fstest.MapFile{Data: []byte("# Skill B\n\nMore content.")},
		"large.bin":        &fstest.MapFile{Data: append(make([]byte, testMaxFileSize), []byte("BEYOND_LIMIT")...)},
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

func TestReadSkillHandler_EmptyPath(t *testing.T) {
	h := newTestHandler()
	_, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path is required")
}

func TestReadSkillHandler_PathTraversal(t *testing.T) {
	h := newTestHandler()
	_, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "../etc/passwd"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid path")
}

func TestReadSkillHandler_AbsolutePath(t *testing.T) {
	h := newTestHandler()
	_, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "/etc/passwd"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid path")
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
	assert.Contains(t, err.Error(), "cannot read")
}

func TestReadSkillHandler_FileTooLarge_Truncates(t *testing.T) {
	h := newTestHandler()
	_, output, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "large.bin"})
	require.NoError(t, err)
	assert.Len(t, []byte(output.Instructions)[:testMaxFileSize], testMaxFileSize)
	assert.Contains(t, output.Instructions, "truncated after")
	assert.NotContains(t, output.Instructions, "BEYOND_LIMIT")
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
