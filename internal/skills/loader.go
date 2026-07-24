// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package skills

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FileLoader loads and validates skill definitions from a directory.
type FileLoader struct {
	Dir string
}

// Load discovers .json/.yaml/.yml skill files and validates merged definitions.
func (l FileLoader) Load(ctx context.Context) ([]Definition, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if strings.TrimSpace(l.Dir) == "" {
		return nil, fmt.Errorf("skills directory must not be empty")
	}

	entries, err := os.ReadDir(l.Dir)
	if err != nil {
		return nil, fmt.Errorf("read skills directory %q: %w", l.Dir, err)
	}

	defs := make([]Definition, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isSupportedSkillsConfig(name) {
			continue
		}

		filePath := filepath.Join(l.Dir, name)
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			return nil, fmt.Errorf("read skills file %q: %w", filePath, readErr)
		}

		parsed, parseErr := ParseDefinitions(name, data)
		if parseErr != nil {
			return nil, fmt.Errorf("parse skills file %q: %w", filePath, parseErr)
		}
		defs = append(defs, parsed...)
	}

	if err := ValidateDefinitions(defs); err != nil {
		return nil, err
	}
	return defs, nil
}

// WatchEvents is a lightweight event channel type for future hot-reload integration.
type WatchEvents <-chan fs.FileInfo

func isSupportedSkillsConfig(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".json" || ext == ".yaml" || ext == ".yml"
}
