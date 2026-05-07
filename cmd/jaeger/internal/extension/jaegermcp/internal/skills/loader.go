// Copyright (c) The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package skills implements the Jaeger AI Skills Engine: the component
// responsible for discovering, validating, and serving user-defined Skill
// definitions at runtime without requiring a Jaeger binary recompile.
//
// Design constraints (per maintainer guidance on issue #8440):
//   - Skills are purely declarative (system prompt + allowed_tools list).
//   - Skills MUST NOT register new MCP tool behavior at runtime.
//   - All tool references are validated against a snapshot of the live MCP
//     registry at load time so failures are caught eagerly.
package skills

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
)

// ToolLister is a narrow interface over the MCP server's tool registry,
// used to validate allowed_tools at skill load time.
type ToolLister interface {
	// ListTools returns the names of all currently registered MCP tools.
	ListTools(ctx context.Context) ([]string, error)
}

// Loader discovers, parses, and validates Skill YAML files from a directory.
// It is safe for concurrent reads after Load() completes.
type Loader struct {
	dir    string
	logger *zap.Logger

	mu     sync.RWMutex
	skills map[string]*types.Skill // keyed by Skill.Name
}

// NewLoader creates a Loader that reads skills from dir.
func NewLoader(dir string, logger *zap.Logger) *Loader {
	return &Loader{
		dir:    dir,
		logger: logger,
		skills: make(map[string]*types.Skill),
	}
}

// Load walks dir, parses every *.yaml / *.yml file as a Skill, validates each
// one structurally, then validates all tool references against the live MCP
// registry snapshot returned by lister. Errors for individual files are
// collected and returned as a joined error; a single bad file does not prevent
// valid skills from being loaded.
func (l *Loader) Load(ctx context.Context, lister ToolLister) error {
	knownTools, err := lister.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("skills loader: cannot fetch tool registry: %w", err)
	}
	toolSet := make(map[string]struct{}, len(knownTools))
	for _, t := range knownTools {
		toolSet[t] = struct{}{}
	}

	entries, err := os.ReadDir(l.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			l.logger.Info("skills directory does not exist, no skills loaded", zap.String("dir", l.dir))
			return nil
		}
		return fmt.Errorf("skills loader: reading directory %q: %w", l.dir, err)
	}

	loaded := make(map[string]*types.Skill)
	var errs []error

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isYAML(name) {
			continue
		}

		path := filepath.Join(l.dir, name)
		skill, err := l.loadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("skill file %q: %w", path, err))
			continue
		}

		if err := validateToolRefs(skill, toolSet); err != nil {
			errs = append(errs, fmt.Errorf("skill %q: %w", skill.Name, err))
			continue
		}

		if _, dup := loaded[skill.Name]; dup {
			errs = append(errs, fmt.Errorf("skill name %q is defined more than once (file: %s)", skill.Name, path))
			continue
		}

		loaded[skill.Name] = skill
		l.logger.Info("loaded skill",
			zap.String("name", skill.Name),
			zap.String("file", path),
			zap.Strings("allowed_tools", skill.AllowedTools),
		)
	}

	l.mu.Lock()
	l.skills = loaded
	l.mu.Unlock()

	return errors.Join(errs...)
}

// Get returns the named skill, or (nil, false) if not found.
func (l *Loader) Get(name string) (*types.Skill, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	s, ok := l.skills[name]
	return s, ok
}

// All returns a snapshot of all loaded skills, keyed by name.
func (l *Loader) All() map[string]*types.Skill {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make(map[string]*types.Skill, len(l.skills))
	for k, v := range l.skills {
		out[k] = v
	}
	return out
}

// loadFile reads and parses a single YAML file into a Skill.
func (l *Loader) loadFile(path string) (*types.Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	var skill types.Skill
	if err := yaml.Unmarshal(data, &skill); err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}

	if err := skill.Validate(); err != nil {
		return nil, fmt.Errorf("validation: %w", err)
	}

	return &skill, nil
}

// validateToolRefs ensures every tool listed in skill.AllowedTools exists in
// the MCP registry snapshot. This is an eager check so bad references are
// caught at startup rather than at agent request time.
func validateToolRefs(skill *types.Skill, toolSet map[string]struct{}) error {
	var errs []error
	for _, tool := range skill.AllowedTools {
		if _, ok := toolSet[tool]; !ok {
			errs = append(errs, fmt.Errorf("allowed_tools references unknown MCP tool %q", tool))
		}
	}
	return errors.Join(errs...)
}

func isYAML(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}
