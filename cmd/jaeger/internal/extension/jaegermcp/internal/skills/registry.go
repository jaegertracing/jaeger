// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package skills

import (
	"sync"
)

// Registry holds a collection of loaded skills
type Registry struct {
	mu     sync.RWMutex
	skills map[string]Skill
}

// NewRegistry creates a new, empty Registry
func NewRegistry() *Registry {
	return &Registry{
		skills: make(map[string]Skill),
	}
}

// Register adds a skill to the registry
func (r *Registry) Register(skill Skill) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if skill.Name == "" {
		return
	}
	r.skills[skill.Name] = skill
}

// Get retrieves a skill from the registry by name
func (r *Registry) Get(name string) (Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skill, ok := r.skills[name]
	return skill, ok
}

// List returns all registered skills
func (r *Registry) List() []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var skills []Skill
	for _, skill := range r.skills {
		skills = append(skills, skill)
	}
	return skills
}
