// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// frontMatterDelimiter opens and closes the YAML block at the top of a skill file.
const frontMatterDelimiter = "---"

// ReadSkillInput defines the input parameters for the read_skill MCP tool.
type ReadSkillInput struct {
	Path string `json:"path" jsonschema:"Relative path within the skills directory (e.g. SKILL.md or detect-n-plus-one/SKILL.md)."`
}

// ReadSkillOutput defines the output of the read_skill MCP tool.
type ReadSkillOutput struct {
	Instructions string `json:"instructions"`
}

// SkillFrontMatter holds the attributes a skill file declares in its leading
// YAML block, named as the agentskills.io specification names them.
type SkillFrontMatter struct {
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description"`
	License      string            `yaml:"license"`
	AllowedTools string            `yaml:"allowed-tools"`
	Metadata     map[string]string `yaml:"metadata"`
}

// Skill is a skill file split into its declared attributes and its body.
type Skill struct {
	FrontMatter SkillFrontMatter
	Content     string
}

// ParseSkill splits a skill file into its front matter attributes and its body.
// A delimiter is a whole line holding only "---", with either line ending. A file
// with no leading delimiter line, or with a block that is never closed, is returned
// whole as Content. Malformed YAML inside a closed block is an error.
func ParseSkill(raw string) (Skill, error) {
	open := delimiterLineEnd(raw)
	if open < 0 {
		return Skill{Content: raw}, nil
	}
	for pos := open; pos < len(raw); {
		if end := delimiterLineEnd(raw[pos:]); end >= 0 {
			var fm SkillFrontMatter
			if err := yaml.Unmarshal([]byte(raw[open:pos]), &fm); err != nil {
				return Skill{}, fmt.Errorf("invalid front matter: %w", err)
			}
			return Skill{
				FrontMatter: fm,
				Content:     strings.TrimLeft(raw[pos+end:], "\r\n"),
			}, nil
		}
		next := strings.IndexByte(raw[pos:], '\n')
		if next < 0 {
			break
		}
		pos += next + 1
	}
	return Skill{Content: raw}, nil
}

// delimiterLineEnd returns the offset just past the first line of s when that
// line is a front matter delimiter, and -1 otherwise.
func delimiterLineEnd(s string) int {
	line, _, hasNewline := strings.Cut(s, "\n")
	if strings.TrimSuffix(line, "\r") != frontMatterDelimiter {
		return -1
	}
	if hasNewline {
		return len(line) + 1
	}
	return len(line)
}
