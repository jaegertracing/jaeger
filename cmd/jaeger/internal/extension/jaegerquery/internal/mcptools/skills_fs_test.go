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

func TestOpenOperatorSkillsDir_EmptyPathOpensNothing(t *testing.T) {
	operator, err := OpenOperatorSkillsDir("")
	require.NoError(t, err)
	assert.Nil(t, operator, "no skills_dir configured means no operator FS")
}

func TestOpenOperatorSkillsDir_ServesDirectoryContents(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("operator catalog"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "slow-db-call"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "slow-db-call", "SKILL.md"), []byte("skill body"), 0o600))

	operator, err := OpenOperatorSkillsDir(dir)
	require.NoError(t, err)

	catalog, err := fs.ReadFile(operator, "SKILL.md")
	require.NoError(t, err)
	assert.Equal(t, "operator catalog", string(catalog))

	skill, err := fs.ReadFile(operator, "slow-db-call/SKILL.md")
	require.NoError(t, err)
	assert.Equal(t, "skill body", string(skill))
}

func TestOpenOperatorSkillsDir_HardFailsOnUnusablePath(t *testing.T) {
	t.Run("nonexistent directory", func(t *testing.T) {
		_, err := OpenOperatorSkillsDir(filepath.Join(t.TempDir(), "no-such-dir"))
		require.ErrorContains(t, err, "cannot open skills_dir")
	})

	t.Run("path is a file", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "not-a-dir")
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
		_, err := OpenOperatorSkillsDir(f)
		require.ErrorContains(t, err, "cannot open skills_dir")
	})

	t.Run("directory is not listable", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("permission bits are not enforced the same way on Windows")
		}
		if os.Geteuid() == 0 {
			t.Skip("running as root ignores directory permission bits")
		}
		// Read without execute: os.OpenRoot succeeds (it can stat the
		// directory) but its entries cannot be listed.
		dir := t.TempDir()
		require.NoError(t, os.Chmod(dir, 0o400))
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

		_, err := OpenOperatorSkillsDir(dir)
		require.ErrorContains(t, err, "cannot list skills_dir")
	})
}

func TestOpenOperatorSkillsDir_BlocksSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600))

	dir := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "evil")))

	operator, err := OpenOperatorSkillsDir(dir)
	require.NoError(t, err)

	// os.OpenRoot refuses to follow a symlink out of skills_dir.
	_, err = operator.Open("evil/secret.txt")
	require.Error(t, err, "a symlink pointing outside skills_dir must not be followed")

	var pathErr *fs.PathError
	require.ErrorAs(t, err, &pathErr)
}
