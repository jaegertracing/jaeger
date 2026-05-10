// Copyright (c) The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkill_Validate_Valid(t *testing.T) {
	s := &Skill{
		Name:         "analyze-critical-path",
		Description:  "Identifies the slowest path through a trace.",
		SystemPrompt: "You are an expert in distributed tracing...",
		AllowedTools: []string{"get_critical_path", "get_span_details"},
	}
	require.NoError(t, s.Validate())
}

func TestSkill_Validate_MissingName(t *testing.T) {
	s := &Skill{
		Description:  "desc",
		SystemPrompt: "prompt",
		AllowedTools: []string{"search_traces"},
	}
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skill.name must not be empty")
}

func TestSkill_Validate_MissingSystemPrompt(t *testing.T) {
	s := &Skill{
		Name:         "my-skill",
		Description:  "desc",
		AllowedTools: []string{"search_traces"},
	}
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "system_prompt must not be empty")
}

func TestSkill_Validate_EmptyAllowedTools(t *testing.T) {
	s := &Skill{
		Name:         "my-skill",
		Description:  "desc",
		SystemPrompt: "prompt",
		AllowedTools: []string{},
	}
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "allowed_tools must contain at least one tool")
}

func TestSkill_Validate_DuplicateTool(t *testing.T) {
	s := &Skill{
		Name:         "my-skill",
		Description:  "desc",
		SystemPrompt: "prompt",
		AllowedTools: []string{"search_traces", "search_traces"},
	}
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate tool")
}

func TestSkill_Validate_EmptyToolName(t *testing.T) {
	s := &Skill{
		Name:         "my-skill",
		Description:  "desc",
		SystemPrompt: "prompt",
		AllowedTools: []string{"search_traces", ""},
	}
	err := s.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty tool name")
}
