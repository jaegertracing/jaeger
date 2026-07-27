// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// Frontmatter limits from the agent skills specification,
// https://agentskills.io/specification.
const (
	maxSkillNameLen          = 64
	maxSkillDescriptionLen   = 1024
	maxSkillCompatibilityLen = 500
)

// skillNamePattern enforces the spec's naming rules: lowercase letters,
// digits, and hyphens only, with no leading/trailing hyphen and no
// consecutive hyphens.
var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// skillFrontmatter is the YAML frontmatter schema of a SKILL.md file per
// https://agentskills.io/specification. It is decoded strictly
// (KnownFields), so an unknown key fails that skill's validation.
type skillFrontmatter struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	License       string            `yaml:"license"`
	Metadata      map[string]string `yaml:"metadata"`
	Compatibility string            `yaml:"compatibility"`
	// AllowedTools is space-separated. Beyond the base spec, each entry must
	// name a tool registered on this MCP server.
	AllowedTools string `yaml:"allowed-tools"`
}

// parseSkillFrontmatter decodes a SKILL.md's frontmatter and validates it
// against the directory that contains it — the only entry point a caller
// needs; there is no separate validate step to remember to call.
func parseSkillFrontmatter(data []byte, dirName string, registeredTools map[string]bool) (skillFrontmatter, error) {
	fm, err := decodeSkillFrontmatter(data)
	if err != nil {
		return fm, err
	}
	if err := validateSkillFrontmatter(fm, dirName, registeredTools); err != nil {
		return fm, err
	}
	return fm, nil
}

// decodeSkillFrontmatter extracts the leading "---" block from a SKILL.md and
// strict-decodes it, so unknown or misspelled keys are rejected.
func decodeSkillFrontmatter(data []byte) (skillFrontmatter, error) {
	var fm skillFrontmatter
	// Normalize CRLF so Windows-authored files still match the delimiters.
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	after, ok := bytes.CutPrefix(data, []byte("---\n"))
	if !ok {
		return fm, errors.New(`missing "---" frontmatter block`)
	}
	// Require a full delimiter line ("\n---\n"); a bare "\n---" is only
	// accepted when it's the very end of the file, so a YAML block scalar
	// that happens to contain a line starting with "---" doesn't truncate
	// the frontmatter early.
	block, _, ok := bytes.Cut(after, []byte("\n---\n"))
	if !ok {
		var rest []byte
		block, rest, ok = bytes.Cut(after, []byte("\n---"))
		if !ok || len(rest) != 0 {
			return fm, errors.New(`frontmatter not terminated by "---"`)
		}
	}
	dec := yaml.NewDecoder(bytes.NewReader(block))
	dec.KnownFields(true)
	if err := dec.Decode(&fm); err != nil {
		return fm, fmt.Errorf("invalid frontmatter: %w", err)
	}
	// KnownFields only guards this first Decode call: a block containing a
	// valid document followed by an explicit "..." (or "---") terminator and
	// more YAML would otherwise decode cleanly, silently ignoring whatever
	// follows — including fields KnownFields was supposed to catch. Decoding
	// once more must reach end of stream.
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fm, errors.New("frontmatter contains more than one YAML document")
		}
		return fm, fmt.Errorf("invalid frontmatter: %w", err)
	}
	return fm, nil
}

// validateSkillFrontmatter checks fm against the agentskills.io rules (and
// our stricter allowed-tools check), aggregating every violation with
// errors.Join so the warning for a skill names all of its problems at once.
func validateSkillFrontmatter(fm skillFrontmatter, dirName string, registeredTools map[string]bool) error {
	var errs []error
	switch {
	case fm.Name == "":
		errs = append(errs, errors.New("name is required"))
	case utf8.RuneCountInString(fm.Name) > maxSkillNameLen:
		errs = append(errs, fmt.Errorf("name exceeds %d characters", maxSkillNameLen))
	case !skillNamePattern.MatchString(fm.Name):
		errs = append(errs, fmt.Errorf("name %q must be lowercase letters, digits, and single hyphens with no leading/trailing hyphen", fm.Name))
	case fm.Name != dirName:
		errs = append(errs, fmt.Errorf("name %q must match its directory name %q", fm.Name, dirName))
	default:
	}
	if fm.Description == "" {
		errs = append(errs, errors.New("description is required"))
	} else if utf8.RuneCountInString(fm.Description) > maxSkillDescriptionLen {
		errs = append(errs, fmt.Errorf("description exceeds %d characters", maxSkillDescriptionLen))
	}
	if utf8.RuneCountInString(fm.Compatibility) > maxSkillCompatibilityLen {
		errs = append(errs, fmt.Errorf("compatibility exceeds %d characters", maxSkillCompatibilityLen))
	}
	for _, tool := range strings.Fields(fm.AllowedTools) {
		if !registeredTools[tool] {
			errs = append(errs, fmt.Errorf("allowed-tools references unregistered tool %q", tool))
		}
	}
	return errors.Join(errs...)
}
