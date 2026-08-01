// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
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

// skillNamePattern enforces the spec's naming rules: lowercase letters, digits,
// and hyphens only, with no leading or trailing hyphen and no doubled hyphen.
var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// skillFrontmatter is the YAML frontmatter of a SKILL.md, per
// https://agentskills.io/specification.
type skillFrontmatter struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	License       string            `yaml:"license"`
	Metadata      map[string]string `yaml:"metadata"`
	Compatibility string            `yaml:"compatibility"`
	AllowedTools  string            `yaml:"allowed-tools"`
}

// validateSkillFrontmatter decodes a SKILL.md's frontmatter and checks it
// against the directory holding it. Skills are validated on every read rather
// than once at startup, so a skills_dir edited while Jaeger runs takes effect
// without a restart.
func validateSkillFrontmatter(data []byte, dirName string) error {
	fm, err := decodeSkillFrontmatter(data)
	if err != nil {
		return err
	}
	return fm.validate(dirName)
}

// decodeSkillFrontmatter extracts the leading "---" block and strict-decodes it,
// so an unknown or misspelled key is an error rather than a silently ignored
// field.
func decodeSkillFrontmatter(data []byte) (skillFrontmatter, error) {
	var fm skillFrontmatter
	// Normalize CRLF so a Windows-authored file still matches the delimiters.
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	after, ok := bytes.CutPrefix(data, []byte("---\n"))
	if !ok {
		return fm, errors.New(`missing "---" frontmatter block`)
	}
	// Require a full delimiter line; a bare "\n---" only ends the block at end
	// of file, so a YAML value whose own text contains a line starting with
	// "---" cannot truncate the frontmatter early.
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
	// KnownFields guards only the first document. A "..." terminator inside the
	// block starts a second one whose keys strict decoding would never see, so
	// anything a typo'd key was meant to catch could ride along in it. Decoding
	// again therefore has to reach end of stream.
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return fm, errors.New("frontmatter must be a single YAML document")
	}
	return fm, nil
}

// validate checks fm against the spec's rules, joining every violation so one
// error names everything wrong with a skill instead of only the first thing.
func (fm skillFrontmatter) validate(dirName string) error {
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
	return errors.Join(errs...)
}
