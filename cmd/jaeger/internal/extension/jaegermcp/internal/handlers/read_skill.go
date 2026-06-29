// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
)

type readSkillHandler struct {
	skillsFS    fs.FS
	maxFileSize int64
}

// NewReadSkillHandler creates a handler that reads skill files from the given FS.
func NewReadSkillHandler(
	skillsFS fs.FS,
	maxFileSize int64,
) mcp.ToolHandlerFor[types.ReadSkillInput, types.ReadSkillOutput] {
	h := &readSkillHandler{skillsFS: skillsFS, maxFileSize: maxFileSize}
	return h.handle
}

func (h *readSkillHandler) handle(
	_ context.Context,
	_ *mcp.CallToolRequest,
	input types.ReadSkillInput,
) (*mcp.CallToolResult, types.ReadSkillOutput, error) {
	if input.Path == "" {
		return nil, types.ReadSkillOutput{}, errors.New("path is required")
	}
	if !fs.ValidPath(input.Path) {
		return nil, types.ReadSkillOutput{}, fmt.Errorf("invalid path: %q", input.Path)
	}

	data, err := fs.ReadFile(h.skillsFS, input.Path)
	if err != nil {
		return nil, types.ReadSkillOutput{}, fmt.Errorf("cannot read %q: %w", input.Path, err)
	}

	content := string(data)
	if int64(len(data)) > h.maxFileSize {
		content = string(data[:h.maxFileSize]) +
			fmt.Sprintf("\n\n[file is too large, truncated after %d bytes]", h.maxFileSize)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: content}},
	}, types.ReadSkillOutput{Instructions: content}, nil
}
