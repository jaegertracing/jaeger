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
		_, out, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "custom/slow-db-call/SKILL.md"})
		require.NoError(t, err)
		assert.Equal(t, "# Slow DB Call", out.Instructions)
	})

	t.Run("custom entry point is served", func(t *testing.T) {
		_, out, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "custom/SKILL.md"})
		require.NoError(t, err)
		assert.Equal(t, "# Operator catalog", out.Instructions)
	})

	t.Run("built-ins still reachable at the root", func(t *testing.T) {
		_, out, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "skill-a/SKILL.md"})
		require.NoError(t, err)
		assert.Equal(t, "# Skill A\n\nContent here.", out.Instructions)
	})

	t.Run("traversal out of custom is rejected", func(t *testing.T) {
		_, _, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "custom/../etc/passwd"})
		require.Error(t, err)
	})
}

func TestReadSkillHandler_StripsFrontMatter(t *testing.T) {
	const withFrontMatter = `---
name: skill-c
description: does a thing
license: Apache-2.0
allowed-tools: get_services
---

# Skill C

Procedure goes here.`

	h := &readSkillHandler{
		builtins: fstest.MapFS{
			"skill-c/SKILL.md": &fstest.MapFile{Data: []byte(withFrontMatter)},
		},
		maxFileSize: 1024,
	}

	_, output, err := h.handle(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: "skill-c/SKILL.md"})
	require.NoError(t, err)
	assert.Equal(t, "# Skill C\n\nProcedure goes here.", output.Instructions)
	assert.NotContains(t, output.Instructions, "license")
	assert.NotContains(t, output.Instructions, "allowed-tools")
}
