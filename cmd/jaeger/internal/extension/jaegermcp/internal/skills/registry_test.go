// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package skills

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	registry := NewRegistry()

	skill := Skill{
		Name:         "test-skill",
		Description:  "A test skill",
		SystemPrompt: "You are a test.",
	}

	registry.Register(skill)

	retrievedSkill, ok := registry.Get("test-skill")
	assert.True(t, ok)
	assert.Equal(t, skill, retrievedSkill)
}

func TestRegistryGetMissing(t *testing.T) {
	registry := NewRegistry()

	_, ok := registry.Get("missing-skill")
	assert.False(t, ok)
}

func TestRegistryList(t *testing.T) {
	registry := NewRegistry()

	skill1 := Skill{
		Name:         "test-skill-1",
		Description:  "A test skill 1",
		SystemPrompt: "You are a test 1.",
	}
	skill2 := Skill{
		Name:         "test-skill-2",
		Description:  "A test skill 2",
		SystemPrompt: "You are a test 2.",
	}

	registry.Register(skill1)
	registry.Register(skill2)

	skills := registry.List()
	assert.Len(t, skills, 2)

	// Registry map iteration order is not guaranteed, so just check contents
	found1 := false
	found2 := false
	for _, s := range skills {
		switch s.Name {
		case "test-skill-1":
			found1 = true
		case "test-skill-2":
			found2 = true
		default:
			// Ignore other skills
		}
	}
	assert.True(t, found1)
	assert.True(t, found2)
}

func TestRegistryRegisterEmptyName(t *testing.T) {
	registry := NewRegistry()

	skill := Skill{
		Description:  "A test skill",
		SystemPrompt: "You are a test.",
	}

	registry.Register(skill)

	skills := registry.List()
	assert.Empty(t, skills)
}
