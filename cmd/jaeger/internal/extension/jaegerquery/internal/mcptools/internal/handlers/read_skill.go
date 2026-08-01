// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
)

// customSkillsDir is the path prefix under which the operator's skills are
// addressed, e.g. custom/<skill-name>/SKILL.md.
const customSkillsDir = "custom"

// skillFile is the name every skill's playbook goes by.
const skillFile = "SKILL.md"

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
	// Frontmatter is validated against what was actually read, before the
	// truncation notice is appended; it sits at the head of the file, so a body
	// long enough to be truncated does not affect the check.
	if dir, ok := skillDirName(input.Path); ok {
		if err := validateSkillFrontmatter(buf[:n], dir); err != nil {
			return nil, types.ReadSkillOutput{}, fmt.Errorf("invalid skill %q: %w", input.Path, err)
		}
	}
	if n > int(h.maxFileSize) {
		content += fmt.Sprintf("\n\nfile content truncated after %d bytes\n", h.maxFileSize)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: content}},
	}, types.ReadSkillOutput{Instructions: content}, nil
}

// skillDirName returns the directory a skill at p is named after, and whether p
// names a skill at all. Only <dir>/SKILL.md is one: a tree's root SKILL.md has
// no directory to take its name from, and a SKILL.md nested deeper is
// documentation inside a skill rather than a skill of its own. The custom/
// prefix is stripped first so operator and built-in skills are judged alike.
func skillDirName(p string) (string, bool) {
	rest, _ := strings.CutPrefix(p, customSkillsDir+"/")
	dir, file := path.Split(rest)
	dir = strings.TrimSuffix(dir, "/")
	if file != skillFile || dir == "" || strings.Contains(dir, "/") {
		return "", false
	}
	return dir, true
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
