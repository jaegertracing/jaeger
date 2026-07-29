// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
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
	f, err := h.skillsFS.Open(input.Path)
	if err != nil {
		return nil, types.ReadSkillOutput{}, fmt.Errorf("cannot read %q: %w", input.Path, err)
	}
	defer f.Close()

	buf := make([]byte, h.maxFileSize+1)
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, types.ReadSkillOutput{}, fmt.Errorf("cannot read %q: %w", input.Path, err)
	}
	content := string(buf[:n])
	if n > int(h.maxFileSize) {
		content += fmt.Sprintf("\n\nfile content truncated after %d bytes\n", h.maxFileSize)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: content}},
	}, types.ReadSkillOutput{Instructions: content}, nil
}
