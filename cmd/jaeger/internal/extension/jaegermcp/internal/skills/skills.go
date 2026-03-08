// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package skills

// SkillTool represents a tool that a skill can use
type SkillTool struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// Skill represents an AI skill configuration
type Skill struct {
	Name         string      `yaml:"name"`
	Description  string      `yaml:"description"`
	SystemPrompt string      `yaml:"system_prompt"`
	Tools        []SkillTool `yaml:"tools"`
	Version      string      `yaml:"version"`
	Author       string      `yaml:"author"`
}
