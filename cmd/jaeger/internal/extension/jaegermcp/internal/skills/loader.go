// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package skills

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// LoadSkills reads .yaml files from a configurable directory path,
// validates required fields (Name, SystemPrompt), and returns []Skill.
func LoadSkills(dir string, logger *zap.Logger) []Skill {
	if logger == nil {
		logger = zap.NewNop()
	}

	var skills []Skill

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Debug("skills directory does not exist", zap.String("dir", dir))
			return skills
		}
		logger.Error("failed to read skills directory", zap.String("dir", dir), zap.Error(err))
		return skills
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			logger.Error("failed to read skill file", zap.String("file", path), zap.Error(err))
			continue
		}

		var skill Skill
		if err := yaml.Unmarshal(data, &skill); err != nil {
			logger.Error("failed to parse skill file", zap.String("file", path), zap.Error(err))
			continue
		}

		if skill.Name == "" {
			logger.Warn("skipped invalid skill: missing Name", zap.String("file", path))
			continue
		}

		if skill.SystemPrompt == "" {
			logger.Warn("skipped invalid skill: missing SystemPrompt", zap.String("file", path), zap.String("skill_name", skill.Name))
			continue
		}

		skills = append(skills, skill)
	}

	return skills
}
