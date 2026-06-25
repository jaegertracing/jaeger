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

type readSkillHandler struct {
	skillsFS    fs.FS
	maxFileSize int64
	gateway     string
}

// NewReadSkillHandler creates a handler that reads skills from the given FS.
// gateway is the root catalog content returned when path is empty.
func NewReadSkillHandler(
	skillsFS fs.FS,
	maxFileSize int64,
	gateway string,
) mcp.ToolHandlerFor[types.ReadSkillInput, ReadSkillOutput] {
	h := &readSkillHandler{skillsFS: skillsFS, maxFileSize: maxFileSize, gateway: gateway}
	return h.handle
}

func (h *readSkillHandler) handle(
	_ context.Context,
	_ *mcp.CallToolRequest,
	input types.ReadSkillInput,
) (*mcp.CallToolResult, ReadSkillOutput, error) {
	if input.Path == "" {
		return textResult(h.gateway), ReadSkillOutput{}, nil
	}
	if !fs.ValidPath(input.Path) {
		return nil, ReadSkillOutput{}, fmt.Errorf("invalid path: %q", input.Path)
	}

	info, err := fs.Stat(h.skillsFS, input.Path)
	if err != nil {
		return nil, ReadSkillOutput{}, fmt.Errorf("cannot stat %q: %w", input.Path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, ReadSkillOutput{}, fmt.Errorf("%q is not a regular file", input.Path)
	}
	if info.Size() > h.maxFileSize {
		return nil, ReadSkillOutput{}, fmt.Errorf(
			"%q is %d bytes, exceeds limit of %d bytes",
			input.Path, info.Size(), h.maxFileSize,
		)
	}

	data, err := fs.ReadFile(h.skillsFS, input.Path)
	if err != nil {
		return nil, ReadSkillOutput{}, fmt.Errorf("cannot read %q: %w", input.Path, err)
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
