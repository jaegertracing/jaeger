// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLoadSkills(t *testing.T) {
	logger := zap.NewNop()

	t.Run("empty directory returns empty slice", func(t *testing.T) {
		tempDir := t.TempDir()
		skills := LoadSkills(tempDir, logger)
		assert.Empty(t, skills)
	})

	t.Run("valid skill loaded", func(t *testing.T) {
		tempDir := t.TempDir()
		skillData := `
name: test-skill
description: "A test skill"
system_prompt: "You are a test."
version: "1.0"
author: "test"
`
		err := os.WriteFile(filepath.Join(tempDir, "test.yaml"), []byte(skillData), 0o644)
		require.NoError(t, err)

		skills := LoadSkills(tempDir, logger)
		assert.Len(t, skills, 1)
		assert.Equal(t, "test-skill", skills[0].Name)
		assert.Equal(t, "You are a test.", skills[0].SystemPrompt)
	})

	t.Run("missing name skipped", func(t *testing.T) {
		tempDir := t.TempDir()
		skillData := `
description: "A test skill"
system_prompt: "You are a test."
`
		err := os.WriteFile(filepath.Join(tempDir, "missing_name.yaml"), []byte(skillData), 0o644)
		require.NoError(t, err)

		skills := LoadSkills(tempDir, logger)
		assert.Empty(t, skills)
	})

	t.Run("missing system_prompt skipped", func(t *testing.T) {
		tempDir := t.TempDir()
		skillData := `
name: test-skill
description: "A test skill"
`
		err := os.WriteFile(filepath.Join(tempDir, "missing_prompt.yaml"), []byte(skillData), 0o644)
		require.NoError(t, err)

		skills := LoadSkills(tempDir, logger)
		assert.Empty(t, skills)
	})

	t.Run("invalid yaml skipped", func(t *testing.T) {
		tempDir := t.TempDir()
		skillData := `
name: test-skill
[invalid yaml
`
		err := os.WriteFile(filepath.Join(tempDir, "invalid.yaml"), []byte(skillData), 0o644)
		require.NoError(t, err)

		skills := LoadSkills(tempDir, logger)
		assert.Empty(t, skills)
	})

	t.Run("directory does not exist", func(t *testing.T) {
		skills := LoadSkills("/path/does/not/exist/surely", logger)
		assert.Empty(t, skills)
	})
}
