// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package skills

import (
	"embed"
	"io/fs"
	"strings"
	"sync"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

//go:embed builtin/*/SKILL.md
var builtinFS embed.FS

// Skill is a built-in agent skill loaded from an embedded SKILL.md file.
type Skill struct {
	Name        string
	Description string
	// Body is the full SKILL.md content including frontmatter, returned verbatim on resources/read.
	Body string
}

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

var (
	builtinOnce   sync.Once
	builtinCached []Skill
)

// BuiltinSkills returns all embedded skills. Validation is lenient: name/dir mismatches
// and names > 64 chars produce warnings but still load; missing description skips the skill.
// The FS walk runs once; subsequent calls return the cached result.
func BuiltinSkills(logger *zap.Logger) []Skill {
	builtinOnce.Do(func() {
		var result []Skill
		err := fs.WalkDir(builtinFS, "builtin", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || d.Name() != "SKILL.md" {
				return nil
			}
			data, readErr := builtinFS.ReadFile(path)
			if readErr != nil {
				logger.Warn("cannot read skill file", zap.String("path", path), zap.Error(readErr))
				return nil
			}
			skill, ok := parseSkill(path, data, logger)
			if ok {
				result = append(result, skill)
			}
			return nil
		})
		if err != nil {
			logger.Error("failed to walk built-in skill FS", zap.Error(err))
		}
		builtinCached = result
	})
	// Return a copy so callers cannot mutate the global cache across goroutines.
	out := make([]Skill, len(builtinCached))
	copy(out, builtinCached)
	return out
}

// parseSkill parses one SKILL.md. Body content is not validated; empty bodies are accepted.
func parseSkill(path string, data []byte, logger *zap.Logger) (Skill, bool) {
	// Normalize so CRLF files from Windows or copy-paste parse correctly.
	body := strings.ReplaceAll(string(data), "\r\n", "\n")

	if !strings.HasPrefix(body, "---\n") {
		logger.Warn("missing opening frontmatter delimiter", zap.String("path", path))
		return Skill{}, false
	}
	rest := body[4:]

	// Use "\n---\n" not "\n---" to avoid false matches on body lines beginning with "---".
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		// Accept closing delimiter at EOF (no trailing newline) per common editor behaviour.
		if !strings.HasSuffix(rest, "\n---") {
			logger.Warn("unclosed frontmatter (no closing ---)", zap.String("path", path))
			return Skill{}, false
		}
		end = len(rest) - 4
	}

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		logger.Warn("cannot parse YAML frontmatter", zap.String("path", path), zap.Error(err))
		return Skill{}, false
	}

	if fm.Description == "" {
		logger.Warn("required 'description' field is missing; skipping skill", zap.String("path", path))
		return Skill{}, false
	}

	dirName := dirFromPath(path)
	if fm.Name == "" {
		logger.Warn("'name' field is absent; using directory name", zap.String("path", path), zap.String("dir", dirName))
		fm.Name = dirName
	} else if fm.Name != dirName {
		// agentskills.io spec requires name == parent dir; warn but load so the skill is not silently dropped.
		logger.Warn("name does not match parent directory", zap.String("path", path), zap.String("name", fm.Name), zap.String("dir", dirName))
	}
	if len(fm.Name) > 64 {
		logger.Warn("name exceeds the 64-char agentskills.io limit", zap.String("path", path), zap.String("name", fm.Name))
	}

	return Skill{Name: fm.Name, Description: fm.Description, Body: body}, true
}

func dirFromPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return ""
}
