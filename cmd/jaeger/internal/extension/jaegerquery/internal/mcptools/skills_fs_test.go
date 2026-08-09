// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenCustomSkillsDir_EmptyPathOpensNothing(t *testing.T) {
	custom, err := OpenCustomSkillsDir("")
	require.NoError(t, err)
	assert.Nil(t, custom, "no skills_dir configured means no custom FS")
}

func TestOpenCustomSkillsDir_ServesDirectoryContents(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("operator catalog"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "slow-db-call"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "slow-db-call", "SKILL.md"), []byte("skill body"), 0o600))

	custom, err := OpenCustomSkillsDir(dir)
	require.NoError(t, err)
	// The tree holds an open OS handle. Leaving it open makes t.TempDir's
	// cleanup fail on Windows, which cannot remove a directory that is still
	// in use; on Linux the unlink succeeds and the leak goes unnoticed.
	t.Cleanup(func() { require.NoError(t, custom.Close()) })

	catalog, err := fs.ReadFile(custom, "SKILL.md")
	require.NoError(t, err)
	assert.Equal(t, "operator catalog", string(catalog))

	skill, err := fs.ReadFile(custom, "slow-db-call/SKILL.md")
	require.NoError(t, err)
	assert.Equal(t, "skill body", string(skill))
}

func TestOpenCustomSkillsDir_CloseReleasesTheRoot(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("catalog"), 0o600))

	custom, err := OpenCustomSkillsDir(dir)
	require.NoError(t, err)
	require.NoError(t, custom.Close())

	_, err = fs.ReadFile(custom, "SKILL.md")
	require.Error(t, err, "the tree must not still be readable once closed")
}

func TestOpenCustomSkillsDir_HardFailsOnUnusablePath(t *testing.T) {
	t.Run("nonexistent directory", func(t *testing.T) {
		_, err := OpenCustomSkillsDir(filepath.Join(t.TempDir(), "no-such-dir"))
		require.ErrorContains(t, err, "cannot open skills_dir")
	})

	t.Run("path is a file", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "not-a-dir")
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
		_, err := OpenCustomSkillsDir(f)
		require.ErrorContains(t, err, "cannot open skills_dir")
	})

	t.Run("no entry-point skill", func(t *testing.T) {
		_, err := OpenCustomSkillsDir(t.TempDir())
		require.ErrorContains(t, err, "cannot read SKILL.md in skills_dir")
	})

	t.Run("entry-point skill is unreadable", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("permission bits are not enforced the same way on Windows")
		}
		if os.Geteuid() == 0 {
			t.Skip("running as root ignores permission bits")
		}
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o000))

		_, err := OpenCustomSkillsDir(dir)
		require.ErrorContains(t, err, "cannot read SKILL.md in skills_dir")
	})
}

func TestOpenCustomSkillsDir_BlocksSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600))

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("catalog"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "evil")))

	custom, err := OpenCustomSkillsDir(dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, custom.Close()) })

	// os.OpenRoot refuses to follow a symlink out of skills_dir.
	_, err = custom.Open("evil/secret.txt")
	require.Error(t, err, "a symlink pointing outside skills_dir must not be followed")

	var pathErr *fs.PathError
	require.ErrorAs(t, err, &pathErr)
}
