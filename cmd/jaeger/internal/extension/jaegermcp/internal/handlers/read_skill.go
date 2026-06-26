// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
)

// ReadSkillOutput is a placeholder; the handler returns raw text via *mcp.CallToolResult.
type ReadSkillOutput struct{}

const rootSkillFile = "SKILL.md"

type readSkillHandler struct {
	skillsFS     fs.FS
	maxFileSize  int64
	fallbackText string
}

// NewReadSkillHandler creates a handler that reads skills from the given FS.
// fallbackText is returned when the root SKILL.md is requested but does not
// exist on disk (auto-generated catalog).
func NewReadSkillHandler(
	skillsFS fs.FS,
	maxFileSize int64,
	fallbackText string,
) mcp.ToolHandlerFor[types.ReadSkillInput, ReadSkillOutput] {
	h := &readSkillHandler{skillsFS: skillsFS, maxFileSize: maxFileSize, fallbackText: fallbackText}
	return h.handle
}

func (h *readSkillHandler) handle(
	_ context.Context,
	_ *mcp.CallToolRequest,
	input types.ReadSkillInput,
) (*mcp.CallToolResult, ReadSkillOutput, error) {
	path := input.Path
	if path == "" {
		path = rootSkillFile
	}
	if !fs.ValidPath(path) {
		return nil, ReadSkillOutput{}, fmt.Errorf("invalid path: %q", input.Path)
	}

	info, err := fs.Stat(h.skillsFS, path)
	if err != nil {
		if path == rootSkillFile && h.fallbackText != "" {
			return textResult(h.fallbackText), ReadSkillOutput{}, nil
		}
		return nil, ReadSkillOutput{}, fmt.Errorf("file not found: %q", path)
	}
	if !info.Mode().IsRegular() {
		return nil, ReadSkillOutput{}, fmt.Errorf("%q is not a regular file", path)
	}
	if info.Size() > h.maxFileSize {
		return nil, ReadSkillOutput{}, fmt.Errorf(
			"%q is %d bytes, exceeds limit of %d bytes",
			path, info.Size(), h.maxFileSize,
		)
	}

	data, err := fs.ReadFile(h.skillsFS, path)
	if err != nil {
		return nil, ReadSkillOutput{}, fmt.Errorf("cannot read %q: %w", path, err)
	}

	return textResult(string(data)), ReadSkillOutput{}, nil
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}
