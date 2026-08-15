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
// YAML block. They describe the skill for callers deciding whether to open it;
// an agent that has already opened it does not need them again.
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
// A file with no leading block, or with one that is never closed, is returned
// whole as Content rather than guessed at. Malformed YAML inside a well-formed
// block is an error, so a broken skill fails loudly instead of being served
// with its attributes showing.
func ParseSkill(raw string) (Skill, error) {
	if !strings.HasPrefix(raw, frontMatterDelimiter+"\n") {
		return Skill{Content: raw}, nil
	}
	rest := raw[len(frontMatterDelimiter)+1:]
	end := strings.Index(rest, "\n"+frontMatterDelimiter)
	if end < 0 {
		return Skill{Content: raw}, nil
	}

	var fm SkillFrontMatter
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		return Skill{}, fmt.Errorf("invalid front matter: %w", err)
	}
	return Skill{
		FrontMatter: fm,
		Content:     strings.TrimLeft(rest[end+len(frontMatterDelimiter)+1:], "\n"),
	}, nil
}
