// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"fmt"
	"io/fs"
	"os"
)

// entryPointSkill is the skill an agent reads first in any skills tree; it
// links to the rest.
const entryPointSkill = "SKILL.md"

// OpenCustomSkillsDir opens the operator's skills directory (ai.skills_dir) for
// read_skill to serve under custom/. It is opened with os.OpenRoot, which blocks ".."
// traversal and symlink escapes out of the directory at the OS level, and the
// returned fs.FS keeps that root open for its lifetime.
//
// An unusable skillsDir — missing, not a directory, or without a readable
// entry-point skill — is broken configuration rather than a content problem, so
// it is a hard error that aborts startup instead of degrading to serving
// nothing. skillsDir == "" means the operator configured none: no FS, no error.
func OpenCustomSkillsDir(skillsDir string) (fs.FS, error) {
	if skillsDir == "" {
		return nil, nil
	}
	root, err := os.OpenRoot(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("cannot open skills_dir %q: %w", skillsDir, err)
	}
	// root.FS() is a pointer type conversion of root itself, not a wrapper, so
	// keeping the returned FS alive keeps root's directory handle open too.
	custom := root.FS()
	// The entry point is what an agent reads first; without it nothing below is
	// reachable. Reading it also catches a directory OpenRoot could open but
	// whose contents are unreadable, which would otherwise only show up as an
	// empty skills tree at serve time.
	if _, err := fs.ReadFile(custom, entryPointSkill); err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("cannot read %s in skills_dir %q: %w", entryPointSkill, skillsDir, err)
	}
	return custom, nil
}
