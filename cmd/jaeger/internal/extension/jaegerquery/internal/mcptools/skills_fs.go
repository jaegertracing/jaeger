// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	"go.uber.org/zap"
)

// maxSkillValidationReadSize bounds a startup frontmatter read, independent
// of read_skill's MaxReadFileSize serve-time cap, so an oversized file fails
// only that one skill's validation instead of costing unbounded time/memory.
const maxSkillValidationReadSize = 64 * 1024

// openOperatorSkillsDir opens skillsDir (via os.OpenRoot, which blocks ".."
// traversal and symlink escapes at the OS level) and validates every
// top-level skill's frontmatter. Failures are two-tier:
//
//   - skillsDir itself unusable (missing, not a directory, unreadable or
//     unlistable) → hard error, aborting startup: that is broken
//     configuration.
//   - an individual operator skill invalid → fail soft: log a warning naming
//     the file and problem, exclude only that skill's top-level directory
//     from the returned set, leave the rest servable. jaeger_query serves
//     the UI, trace APIs, and MCP gateway from one OTel Collector extension,
//     so one bad skill file must never take down the whole process.
//
// skillsDir == "" means the operator has not configured one; both return
// values are then nil, and the caller serves built-ins only.
func openOperatorSkillsDir(skillsDir string, registeredTools map[string]bool, logger *zap.Logger) (fs.FS, map[string]bool, error) {
	if skillsDir == "" {
		return nil, nil, nil
	}
	root, err := os.OpenRoot(skillsDir)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot open skills_dir %q: %w", skillsDir, err)
	}
	// root.FS() is a pointer type conversion of root itself, not a wrapper,
	// so keeping operator alive keeps root's directory handle open too.
	operator := root.FS()
	// OpenRoot can succeed on a directory that isn't listable (e.g. read
	// without execute permission on Unix); check explicitly so that also
	// hard-fails instead of silently validating nothing.
	if _, err := fs.ReadDir(operator, "."); err != nil {
		_ = root.Close()
		return nil, nil, fmt.Errorf("cannot list skills_dir %q: %w", skillsDir, err)
	}
	return operator, validateOperatorSkills(operator, registeredTools, logger), nil
}

// validateOperatorSkills walks the operator tree and validates the
// frontmatter of every top-level <dir>/SKILL.md — exactly one path segment
// below the root, matching the flat layout used by the built-in skills. A
// SKILL.md nested deeper is not treated as a skill, so it can't exclude its
// ancestor directory if invalid. The root SKILL.md is the operator's
// hand-written catalog and is served without validation, same as the
// built-in root catalog. Each invalid skill is logged and its top-level
// directory recorded in the returned excluded set; this never fails — see
// openOperatorSkillsDir for the failure-handling contract.
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
		if verr := validateSkillFile(operator, p, registeredTools); verr != nil {
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

// validateSkillFile reads one SKILL.md from fsys and checks its frontmatter
// against the directory that contains it. Not operator-specific: it takes an
// arbitrary fs.FS, so the same function could validate a built-in skill tree
// too, given one to check.
func validateSkillFile(fsys fs.FS, p string, registeredTools map[string]bool) error {
	f, err := fsys.Open(p)
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

	_, err = parseSkillFrontmatter(data, path.Base(path.Dir(p)), registeredTools)
	return err
}
