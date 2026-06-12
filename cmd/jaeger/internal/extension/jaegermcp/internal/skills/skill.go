// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package skills

import (
	"bytes"
	"fmt"

	"go.yaml.in/yaml/v3"
)

const frontmatterDelimiter = "---\n"

type Skill struct {
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description"`
	Version      string            `yaml:"version,omitempty"`
	Author       string            `yaml:"author,omitempty"`
	Category     string            `yaml:"category,omitempty"`
	AllowedTools []string          `yaml:"allowed_tools"`
	ModelHints   map[string]string `yaml:"model_hints,omitempty"`
	References   []string          `yaml:"references,omitempty"`

	Body string `yaml:"-"`

	SourcePath string `yaml:"-"`
}

// A SKILL.md file must start with a YAML frontmatter block delimited by
// "---" lines. Everything after the closing delimiter is treated as the
// skill's Markdown Body.
func Parse(data []byte, sourcePath string) (*Skill, error) {
	if !bytes.HasPrefix(data, []byte(frontmatterDelimiter)) {
		return nil, fmt.Errorf("%s: must start with %q frontmatter delimiter", sourcePath, "---")
	}

	rest := data[len(frontmatterDelimiter):]
	closingIdx := bytes.Index(rest, []byte(frontmatterDelimiter))
	if closingIdx == -1 {
		return nil, fmt.Errorf("%s: missing closing %q frontmatter delimiter", sourcePath, "---")
	}

	frontmatter := rest[:closingIdx]
	body := rest[closingIdx+len(frontmatterDelimiter):]

	var skill Skill
	if err := yaml.Unmarshal(frontmatter, &skill); err != nil {
		return nil, fmt.Errorf("%s: parsing frontmatter: %w", sourcePath, err)
	}

	if skill.Name == "" {
		return nil, fmt.Errorf("%s: missing required field %q", sourcePath, "name")
	}
	if skill.Description == "" {
		return nil, fmt.Errorf("%s: missing required field %q", sourcePath, "description")
	}
	if len(skill.AllowedTools) == 0 {
		return nil, fmt.Errorf("%s: missing required field %q", sourcePath, "allowed_tools")
	}

	skill.Body = string(bytes.TrimRight(body, " \t\r\n"))
	skill.SourcePath = sourcePath

	return &skill, nil
}
