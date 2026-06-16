// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package skills

import (
	"embed"
	"io/fs"
	"log"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed builtin/*/SKILL.md
var builtinFS embed.FS

// Skill represents a single built-in agent skill loaded from an embedded SKILL.md file.
type Skill struct {
	// Name is the skill identifier (matches the parent directory name).
	Name string
	// Description is the short imperative summary used for discovery.
	Description string
	// Body is the full SKILL.md content, including frontmatter, returned verbatim on resources/read.
	Body string
}

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// BuiltinSkills loads and parses all embedded SKILL.md files, returning one Skill per valid file.
// Validation is lenient: minor issues (name > 64 chars, name/dir mismatch) produce a log warning
// but the skill is still included. A skill is skipped only if its YAML is unparseable or its
// description field is absent.
func BuiltinSkills() []Skill {
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
			log.Printf("skills: cannot read %s: %v", path, readErr)
			return nil
		}
		skill, ok := parseSkill(path, data)
		if ok {
			result = append(result, skill)
		}
		return nil
	})
	if err != nil {
		log.Printf("skills: failed to walk builtin FS: %v", err)
	}
	return result
}

// parseSkill parses a SKILL.md file at path and returns the resulting Skill.
// Returns (Skill{}, false) if the file cannot be parsed or is missing required fields.
func parseSkill(path string, data []byte) (Skill, bool) {
	body := string(data)

	// Frontmatter must begin with exactly "---\n".
	if !strings.HasPrefix(body, "---\n") {
		log.Printf("skills: %s: missing opening frontmatter delimiter", path)
		return Skill{}, false
	}
	rest := body[4:] // skip the opening "---\n"

	// The closing delimiter is "\n---" (newline + three dashes).
	end := strings.Index(rest, "\n---")
	if end < 0 {
		log.Printf("skills: %s: unclosed frontmatter (no closing ---)", path)
		return Skill{}, false
	}
	yamlBlock := rest[:end]

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
		log.Printf("skills: %s: cannot parse YAML frontmatter: %v", path, err)
		return Skill{}, false
	}

	if fm.Description == "" {
		log.Printf("skills: %s: required 'description' field is missing; skipping skill", path)
		return Skill{}, false
	}

	// Derive the expected skill name from the parent directory.
	dirName := dirFromPath(path)

	if fm.Name == "" {
		log.Printf("skills: %s: 'name' field is absent; using directory name %q", path, dirName)
		fm.Name = dirName
	} else if fm.Name != dirName {
		// agentskills.io spec: name must match parent dir name; warn but load.
		log.Printf("skills: %s: name %q does not match parent directory %q", path, fm.Name, dirName)
	}
	if len(fm.Name) > 64 {
		log.Printf("skills: %s: name %q exceeds the 64-char agentskills.io limit", path, fm.Name)
	}

	return Skill{
		Name:        fm.Name,
		Description: fm.Description,
		Body:        body,
	}, true
}

// dirFromPath extracts the parent directory name from an embedded path like
// "builtin/greet-user/SKILL.md" → "greet-user".
func dirFromPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return ""
}
