// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package skills

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var skillNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// ValidateDefinitions validates a full set of parsed skill definitions.
func ValidateDefinitions(defs []Definition) error {
	if len(defs) == 0 {
		return errors.New("no skill definitions provided")
	}

	seen := make(map[string]struct{}, len(defs))
	for i, def := range defs {
		if err := ValidateDefinition(def); err != nil {
			return fmt.Errorf("invalid skill at index %d: %w", i, err)
		}
		if _, ok := seen[def.Name]; ok {
			return fmt.Errorf("duplicate skill name %q", def.Name)
		}
		seen[def.Name] = struct{}{}
	}
	return nil
}

// ValidateDefinition validates a single skill definition.
func ValidateDefinition(def Definition) error {
	name := strings.TrimSpace(def.Name)
	if def.Name != name {
		return fmt.Errorf("name %q must not contain leading or trailing whitespace", def.Name)
	}
	if name == "" {
		return errors.New("name is required")
	}
	if !skillNamePattern.MatchString(name) {
		return fmt.Errorf("name %q must be lowercase alphanumeric with optional '-' or '_'", def.Name)
	}
	if strings.TrimSpace(def.SystemPrompt) == "" {
		return fmt.Errorf("system_prompt is required for %q", def.Name)
	}
	if len(def.AllowedTools) == 0 {
		return fmt.Errorf("allowed_tools must include at least one tool for %q", def.Name)
	}
	if len(def.OutputContract.Fields) == 0 {
		return fmt.Errorf("output_contract.fields must include at least one field for %q", def.Name)
	}
	if strings.TrimSpace(def.OutputContract.Format) == "" {
		return fmt.Errorf("output_contract.format is required for %q", def.Name)
	}
	if def.OutputContract.MaxTokens <= 0 {
		return fmt.Errorf("output_contract.max_tokens must be greater than zero for %q", def.Name)
	}
	return nil
}
