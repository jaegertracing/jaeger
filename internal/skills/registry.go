// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package skills

import (
	"fmt"
	"sort"
	"sync"
)

// Registry stores runtime-discoverable skills that can be updated without recompilation.
type Registry struct {
	mu     sync.RWMutex
	skills map[string]Definition
}

// NewRegistry creates an empty runtime registry.
func NewRegistry() *Registry {
	return &Registry{
		skills: make(map[string]Definition),
	}
}

// Register adds or replaces a skill by name.
func (r *Registry) Register(def Definition) error {
	if err := ValidateDefinition(def); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[def.Name] = def
	return nil
}

// RegisterAll registers all skills from a collection.
func (r *Registry) RegisterAll(defs []Definition) error {
	for _, def := range defs {
		if err := r.Register(def); err != nil {
			return fmt.Errorf("register %q: %w", def.Name, err)
		}
	}
	return nil
}

// Unregister removes a skill by name.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.skills, name)
}

// Get returns a skill definition by name.
func (r *Registry) Get(name string) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.skills[name]
	return def, ok
}

// List returns all registered skills sorted by name.
func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Definition, 0, len(r.skills))
	for _, def := range r.skills {
		out = append(out, def)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}
