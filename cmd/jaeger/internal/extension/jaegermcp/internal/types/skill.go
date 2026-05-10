// Copyright (c) The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package types defines the data structures for the Jaeger AI Skills framework.
// A Skill is a declarative configuration artifact that guides the AI agent
// over a constrained set of existing MCP tools to accomplish a specific
// observability debugging workflow.
package types

import (
	"errors"
	"fmt"
)

// Skill represents a user-defined debugging workflow that the Jaeger AI agent
// can execute. Skills are purely declarative: they compose and constrain
// existing MCP tools via prompt instructions; they do not register new runtime
// behavior.
type Skill struct {
	// Name is the unique identifier for this skill (e.g. "analyze-critical-path").
	Name string `yaml:"name" json:"name"`

	// Description is a human-readable summary shown in the UI skill picker.
	Description string `yaml:"description" json:"description"`

	// Version follows semver and is used for changelog and upgrade paths.
	Version string `yaml:"version,omitempty" json:"version,omitempty"`

	// SystemPrompt is the developer/system-level prompt material injected before
	// the user's message. It MUST NOT contain tool implementations or arbitrary
	// code; it may reference allowed_tools by name.
	SystemPrompt string `yaml:"system_prompt" json:"system_prompt"`

	// Examples are few-shot demonstrations that help the model understand the
	// expected reasoning pattern for this skill.
	Examples []Example `yaml:"examples,omitempty" json:"examples,omitempty"`

	// AllowedTools is the exhaustive list of MCP tool names this skill may
	// invoke. Any tool not listed here will be filtered out before the agent
	// reasons over this skill. References are validated at load time against
	// the live MCP server tool registry.
	AllowedTools []string `yaml:"allowed_tools" json:"allowed_tools"`

	// OutputFormat describes the expected structure of the final answer
	// (e.g. "markdown", "json", "text"). The agent is instructed to conform
	// to this format in its system prompt.
	OutputFormat string `yaml:"output_format,omitempty" json:"output_format,omitempty"`

	// Constraints are additional instructions appended to the system prompt to
	// limit agent behavior (e.g. "Do not speculate beyond the trace data").
	Constraints []string `yaml:"constraints,omitempty" json:"constraints,omitempty"`
}

// Example is a single few-shot demonstration for the skill.
type Example struct {
	// UserQuery is a representative user question for this skill.
	UserQuery string `yaml:"user_query" json:"user_query"`

	// ExpectedToolSequence is the ordered list of MCP tool names the agent is
	// expected to call for this query, used for evaluation and documentation.
	ExpectedToolSequence []string `yaml:"expected_tool_sequence,omitempty" json:"expected_tool_sequence,omitempty"`

	// AnnotatedReasoning is a human-written explanation of why these tool calls
	// are appropriate. It is included in the system prompt as a worked example.
	AnnotatedReasoning string `yaml:"annotated_reasoning,omitempty" json:"annotated_reasoning,omitempty"`
}

// Validate performs structural validation of a Skill definition.
// It does NOT validate tool references against the MCP registry; that
// is done by the Loader at load time via ValidateToolRefs.
func (s *Skill) Validate() error {
	var errs []error

	if s.Name == "" {
		errs = append(errs, errors.New("skill.name must not be empty"))
	}
	if s.Description == "" {
		errs = append(errs, fmt.Errorf("skill %q: description must not be empty", s.Name))
	}
	if s.SystemPrompt == "" {
		errs = append(errs, fmt.Errorf("skill %q: system_prompt must not be empty", s.Name))
	}
	if len(s.AllowedTools) == 0 {
		errs = append(errs, fmt.Errorf("skill %q: allowed_tools must contain at least one tool", s.Name))
	}

	seen := make(map[string]struct{})
	for _, t := range s.AllowedTools {
		if t == "" {
			errs = append(errs, fmt.Errorf("skill %q: allowed_tools contains an empty tool name", s.Name))
			continue
		}
		if _, dup := seen[t]; dup {
			errs = append(errs, fmt.Errorf("skill %q: duplicate tool %q in allowed_tools", s.Name, t))
		}
		seen[t] = struct{}{}
	}

	return errors.Join(errs...)
}
