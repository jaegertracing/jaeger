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

// customSkillsDir is the path prefix under which the operator's skills are
// addressed, e.g. custom/<skill-name>/SKILL.md.
const customSkillsDir = "custom"

type readSkillHandler struct {
	builtins fs.FS
	// custom serves the skills_dir tree; nil when none is configured, in which
	// case every custom/ path reports not-exist.
	custom      fs.FS
	maxFileSize int64
}

// NewReadSkillHandler creates a handler that reads skill files, choosing the
// tree by path prefix: custom/ comes from custom, everything else from
// builtins. custom may be nil (no skills_dir configured).
func NewReadSkillHandler(
	builtins fs.FS,
	custom fs.FS,
	maxFileSize int64,
) mcp.ToolHandlerFor[types.ReadSkillInput, types.ReadSkillOutput] {
	h := &readSkillHandler{builtins: builtins, custom: custom, maxFileSize: maxFileSize}
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

	// The attributes describe the skill for a caller choosing between skills.
	// This caller has already chosen, so it gets the body only.
	skill, err := types.ParseSkill(content)
	if err != nil {
		return nil, types.ReadSkillOutput{}, fmt.Errorf("cannot read %q: %w", input.Path, err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: skill.Content}},
	}, types.ReadSkillOutput{Instructions: skill.Content}, nil
}

// open routes p to the custom tree when it names the custom/ prefix and to the
// built-in tree otherwise — two filesystems and a prefix check, rather than a
// merged view over both.
func (h *readSkillHandler) open(p string) (fs.File, error) {
	if !fs.ValidPath(p) {
		return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrInvalid}
	}
	rest, isCustom := strings.CutPrefix(p, customSkillsDir+"/")
	if !isCustom {
		return h.builtins.Open(p)
	}
	if h.custom == nil {
		return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
	}
	return h.custom.Open(rest)
}
