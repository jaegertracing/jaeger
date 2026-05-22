// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package skills

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ParseDefinitions parses a skills config payload from JSON or YAML.
func ParseDefinitions(fileName string, data []byte) ([]Definition, error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".json":
		return parseJSON(data)
	case ".yaml", ".yml":
		return parseYAML(data)
	default:
		return nil, fmt.Errorf("unsupported skills config extension %q", ext)
	}
}

func parseJSON(data []byte) ([]Definition, error) {
	var defs []Definition
	if err := json.Unmarshal(data, &defs); err == nil {
		return defs, nil
	}

	var wrapped struct {
		Skills []Definition `json:"skills"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, fmt.Errorf("parse json skills config: %w", err)
	}
	return wrapped.Skills, nil
}

func parseYAML(data []byte) ([]Definition, error) {
	var defs []Definition
	if err := yaml.Unmarshal(data, &defs); err == nil {
		return defs, nil
	}

	var wrapped struct {
		Skills []Definition `yaml:"skills"`
	}
	if err := yaml.Unmarshal(data, &wrapped); err != nil {
		return nil, fmt.Errorf("parse yaml skills config: %w", err)
	}
	return wrapped.Skills, nil
}
