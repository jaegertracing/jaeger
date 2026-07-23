// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"go.uber.org/zap"
	"go.yaml.in/yaml/v3"
)

// customSkillsDir is where operator-supplied skills (Config.SkillsDir) are
// mounted in the merged filesystem served by read_skill, e.g.
// custom/<skill-name>/SKILL.md.
const customSkillsDir = "custom"

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

// maxSkillValidationReadSize bounds a startup frontmatter read, independent
// of read_skill's MaxReadFileSize serve-time cap, so an oversized file fails
// only that one skill's validation instead of costing unbounded time/memory.
const maxSkillValidationReadSize = 64 * 1024

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

// buildMergedSkillsFS returns the filesystem served by read_skill: the
// built-in skills at the root, plus — when skillsDir is non-empty — the
// operator's skills mounted under custom/. Failures are two-tier:
//
//   - skillsDir itself unusable (missing, not a directory, unreadable) →
//     hard error, aborting startup: that is broken configuration.
//   - an individual operator skill invalid → fail soft: log a warning naming
//     the file and problem, hide only that skill, serve everything else.
//     jaeger_query serves the UI, trace APIs, and MCP gateway from one OTel
//     Collector extension, so one bad skill file must never take down the
//     whole process.
//
// The operator tree is opened with os.OpenRoot, which blocks ".." traversal
// and symlink escapes at the OS level; the *os.Root stays open for the life
// of the returned FS.
func buildMergedSkillsFS(builtins fs.FS, skillsDir string, registeredTools map[string]bool, logger *zap.Logger) (fs.FS, error) {
	if skillsDir == "" {
		return builtins, nil
	}
	root, err := os.OpenRoot(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("cannot open skills_dir %q: %w", skillsDir, err)
	}
	// root.FS() is a pointer type conversion of root itself, not a wrapper,
	// so keeping operator alive keeps root's directory handle open too.
	operator := root.FS()
	// OpenRoot can succeed on a directory that isn't listable (e.g. read
	// without execute permission on Unix); check explicitly so that also
	// hard-fails instead of silently validating nothing.
	if _, err := fs.ReadDir(operator, "."); err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("cannot list skills_dir %q: %w", skillsDir, err)
	}
	return &mergedSkillsFS{
		builtins: builtins,
		operator: operator,
		excluded: validateOperatorSkills(operator, registeredTools, logger),
	}, nil
}

// validateOperatorSkills walks the operator tree and validates the
// frontmatter of every top-level <dir>/SKILL.md — exactly one path segment
// below the root, matching the flat layout used by the built-in skills. A
// SKILL.md nested deeper is not treated as a skill, so it can't exclude its
// ancestor directory if invalid. The root SKILL.md is the operator's
// hand-written catalog and is served without validation, same as the
// built-in root catalog. Each invalid skill is logged and its top-level
// directory recorded in the returned excluded set; this never fails — see
// buildMergedSkillsFS for the failure-handling contract.
func validateOperatorSkills(operator fs.FS, registeredTools map[string]bool, logger *zap.Logger) map[string]bool {
	excluded := make(map[string]bool)
	// The walk function never returns anything but nil or fs.SkipDir, both of
	// which WalkDir always converts to a nil overall result, so there is no
	// error to check here.
	_ = fs.WalkDir(operator, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			logger.Warn("skipping unreadable path in skills_dir", zap.String("path", p), zap.Error(err))
			return fs.SkipDir
		}
		if d.IsDir() || d.Name() != "SKILL.md" || strings.Count(p, "/") != 1 {
			return nil
		}
		if verr := validateOperatorSkillFile(operator, p, registeredTools); verr != nil {
			logger.Warn("skipping invalid operator skill",
				zap.String("file", p),
				zap.Error(verr))
			// Exclusion is by top-level directory — the unit an agent
			// addresses as custom/<name>/.
			topDir, _, _ := strings.Cut(p, "/")
			excluded[topDir] = true
		}
		return nil
	})
	return excluded
}

// validateOperatorSkillFile reads one SKILL.md and checks its frontmatter
// against the directory that contains it.
func validateOperatorSkillFile(operator fs.FS, p string, registeredTools map[string]bool) error {
	f, err := operator.Open(p)
	if err != nil {
		return fmt.Errorf("cannot open skill file: %w", err)
	}
	defer f.Close()

	// The cap bounds only the frontmatter scan, not the whole file: a large
	// but valid body past this prefix is never read, and parseSkillFrontmatter
	// still finds the frontmatter as long as it terminates within the cap.
	data, err := io.ReadAll(io.LimitReader(f, maxSkillValidationReadSize))
	if err != nil {
		return fmt.Errorf("cannot read skill file: %w", err)
	}

	fm, err := parseSkillFrontmatter(data)
	if err != nil {
		return err
	}
	return validateSkillFrontmatter(fm, path.Base(path.Dir(p)), registeredTools)
}

// parseSkillFrontmatter extracts the leading "---" block from a SKILL.md and
// strict-decodes it, so unknown or misspelled keys are rejected.
func parseSkillFrontmatter(data []byte) (skillFrontmatter, error) {
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

// mergedSkillsFS serves the built-in skills at its root and the operator's
// skills under custom/, with invalid operator skills (excluded) hidden as if
// they did not exist.
type mergedSkillsFS struct {
	builtins fs.FS
	operator fs.FS
	excluded map[string]bool
}

func (m *mergedSkillsFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if name == customSkillsDir {
		return m.operator.Open(".")
	}
	if rest, ok := strings.CutPrefix(name, customSkillsDir+"/"); ok {
		top, _, _ := strings.Cut(rest, "/")
		if m.excluded[top] {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		}
		return m.operator.Open(rest)
	}
	return m.builtins.Open(name)
}
