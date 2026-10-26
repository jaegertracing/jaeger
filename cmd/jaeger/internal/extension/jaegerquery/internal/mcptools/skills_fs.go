// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"fmt"
	"io"
	"io/fs"
	"os"
)

// entryPointSkill is the skill an agent reads first in any skills tree; it
// links to the rest.
const entryPointSkill = "SKILL.md"

// SkillsFS is a skills tree backed by an open OS handle. Whoever mounts it owns
// closing it when the server it was mounted on shuts down.
type SkillsFS interface {
	fs.FS
	io.Closer
}

// rootSkillsFS adapts os.Root, whose Open returns *os.File and so does not
// satisfy fs.FS on its own: root.FS() supplies the FS view over the same handle
// and root supplies Close.
type rootSkillsFS struct {
	fs.FS
	root *os.Root
}

func (r rootSkillsFS) Close() error { return r.root.Close() }

// OpenCustomSkillsDir opens the operator's skills directory (ai.skills_dir) for
// read_skill to serve under custom/. It is opened with os.OpenRoot, which blocks ".."
// traversal and symlink escapes out of the directory at the OS level.
//
// An unusable skillsDir — missing, not a directory, or without a readable
// entry-point skill — is broken configuration rather than a content problem, so
// it is a hard error that aborts startup instead of degrading to serving
// nothing. skillsDir == "" means the operator configured none: no FS, no error.
func OpenCustomSkillsDir(skillsDir string) (SkillsFS, error) {
	if skillsDir == "" {
		return nil, nil
	}
	root, err := os.OpenRoot(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("cannot open skills_dir %q: %w", skillsDir, err)
	}
	custom := rootSkillsFS{FS: root.FS(), root: root}
	// The entry point is what an agent reads first; without it nothing below is
	// reachable. Reading it also catches a directory OpenRoot could open but
	// whose contents are unreadable, which would otherwise only show up as an
	// empty skills tree at serve time.
	if _, err := fs.ReadFile(custom, entryPointSkill); err != nil {
		_ = custom.Close()
		return nil, fmt.Errorf("cannot read %s in skills_dir %q: %w", entryPointSkill, skillsDir, err)
	}
	return custom, nil
}
