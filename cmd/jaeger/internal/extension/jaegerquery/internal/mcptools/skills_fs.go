// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"fmt"
	"io/fs"
	"os"
)

// OpenCustomSkillsDir opens the operator's skills directory (ai.skills_dir) for
// read_skill to serve under custom/. It is opened with os.OpenRoot, which blocks ".."
// traversal and symlink escapes out of the directory at the OS level, and the
// returned fs.FS keeps that root open for its lifetime.
//
// An unusable skillsDir — missing, not a directory, unreadable, or unlistable —
// is broken configuration rather than a content problem, so it is a hard error
// that aborts startup instead of degrading to serving nothing. skillsDir == ""
// means the operator configured none: no FS, no error.
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
	// OpenRoot succeeds on a directory that cannot be listed (read permission
	// without execute, on Unix), which would then look like an empty skills
	// tree at serve time; check explicitly so it fails here instead.
	if _, err := fs.ReadDir(custom, "."); err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("cannot list skills_dir %q: %w", skillsDir, err)
	}
	return custom, nil
}
