// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
)

// customSkillsDir is the path prefix under which operator-supplied skills
// (served from a separate fs.FS) are addressed, e.g.
// custom/<skill-name>/SKILL.md.
const customSkillsDir = "custom"

type readSkillHandler struct {
	builtins fs.FS
	// operator serves the operator's skills_dir tree; nil when the operator
	// hasn't configured one, in which case any custom/ path 404s.
	operator fs.FS
	// excluded holds the top-level directory names of operator skills whose
	// frontmatter failed validation; they're hidden as if they didn't exist.
	excluded    map[string]bool
	maxFileSize int64
}

// NewReadSkillHandler creates a handler that reads skill files, dispatching
// by path prefix: anything under custom/ comes from operator, everything
// else from builtins. operator may be nil (no skills_dir configured).
func NewReadSkillHandler(
	builtins fs.FS,
	operator fs.FS,
	excluded map[string]bool,
	maxFileSize int64,
) mcp.ToolHandlerFor[types.ReadSkillInput, types.ReadSkillOutput] {
	h := &readSkillHandler{builtins: builtins, operator: operator, excluded: excluded, maxFileSize: maxFileSize}
	return h.handle
}

func (h *readSkillHandler) handle(
	_ context.Context,
	_ *mcp.CallToolRequest,
	input types.ReadSkillInput,
) (*mcp.CallToolResult, types.ReadSkillOutput, error) {
	f, err := h.open(input.Path)
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

// open dispatches p to the operator tree when it names the custom/ prefix,
// otherwise to the built-in tree — two plain filesystems and a prefix check,
// no merged/synthetic fs.FS in between.
func (h *readSkillHandler) open(p string) (fs.File, error) {
	if !fs.ValidPath(p) {
		return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrInvalid}
	}
	if p == customSkillsDir {
		if h.operator == nil {
			return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
		}
		return h.operator.Open(".")
	}
	if rest, ok := strings.CutPrefix(p, customSkillsDir+"/"); ok {
		if h.operator == nil {
			return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
		}
		top, _, _ := strings.Cut(rest, "/")
		if h.excluded[top] {
			return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
		}
		return h.operator.Open(rest)
	}
	return h.builtins.Open(p)
}
