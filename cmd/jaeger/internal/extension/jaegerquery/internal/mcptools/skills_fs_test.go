// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// writeSkillFile writes content at relPath under dir, creating parents.
func writeSkillFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(relPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
}

func validSkillMD(name string) string {
	return "---\nname: " + name + "\ndescription: A valid test skill.\n---\n\n# " + name + "\n"
}

var testRegisteredTools = map[string]bool{"read_skill": true}

func TestOpenOperatorSkillsDir_EmptyPathServesNothing(t *testing.T) {
	operator, excluded, err := openOperatorSkillsDir("", nil, zap.NewNop())
	require.NoError(t, err)
	assert.Nil(t, operator, "no skills_dir configured means no operator FS")
	assert.Nil(t, excluded)
}

func TestOpenOperatorSkillsDir_HardFailsOnUnusablePath(t *testing.T) {
	t.Run("nonexistent directory", func(t *testing.T) {
		_, _, err := openOperatorSkillsDir(filepath.Join(t.TempDir(), "no-such-dir"), nil, zap.NewNop())
		require.ErrorContains(t, err, "cannot open skills_dir")
	})

	t.Run("path is a file", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "not-a-dir")
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
		_, _, err := openOperatorSkillsDir(f, nil, zap.NewNop())
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
		// directory) but ReadDir cannot list its entries.
		dir := t.TempDir()
		require.NoError(t, os.Chmod(dir, 0o400))
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

		_, _, err := openOperatorSkillsDir(dir, nil, zap.NewNop())
		require.ErrorContains(t, err, "cannot list skills_dir")
	})
}

func TestOpenOperatorSkillsDir_ServesValidSkills(t *testing.T) {
	dir := t.TempDir()
	// The top-level SKILL.md is the operator's hand-written catalog: served
	// as-is, never validated as a skill (no frontmatter here on purpose).
	writeSkillFile(t, dir, "SKILL.md", "operator catalog")
	writeSkillFile(t, dir, "slow-db-call/SKILL.md", validSkillMD("slow-db-call"))

	operator, excluded, err := openOperatorSkillsDir(dir, testRegisteredTools, zap.NewNop())
	require.NoError(t, err)
	assert.Empty(t, excluded)

	catalog, err := fs.ReadFile(operator, "SKILL.md")
	require.NoError(t, err)
	assert.Equal(t, "operator catalog", string(catalog))

	skill, err := fs.ReadFile(operator, "slow-db-call/SKILL.md")
	require.NoError(t, err)
	assert.Contains(t, string(skill), "name: slow-db-call")
}

func TestOpenOperatorSkillsDir_ExcludesInvalidSkill(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "good-skill/SKILL.md", validSkillMD("good-skill"))
	writeSkillFile(t, dir, "bad-skill/SKILL.md", "---\nname: MISMATCH\n---\nbody\n")

	core, logs := observer.New(zap.WarnLevel)
	operator, excluded, err := openOperatorSkillsDir(dir, testRegisteredTools, zap.New(core))
	require.NoError(t, err, "an invalid skill must not fail construction")

	warnings := logs.FilterMessage("skipping invalid operator skill").All()
	require.Len(t, warnings, 1, "exactly one warning for the one bad skill")
	assert.Equal(t, "bad-skill/SKILL.md", warnings[0].ContextMap()["file"])

	assert.True(t, excluded["bad-skill"])
	assert.False(t, excluded["good-skill"])
	_, err = fs.ReadFile(operator, "good-skill/SKILL.md")
	require.NoError(t, err, "the good skill must still be readable")
}

func TestOpenOperatorSkillsDir_IgnoresNestedSkillMD(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "good-skill/SKILL.md", validSkillMD("good-skill"))
	writeSkillFile(t, dir, "good-skill/docs/SKILL.md", "not frontmatter at all")

	core, logs := observer.New(zap.WarnLevel)
	_, excluded, err := openOperatorSkillsDir(dir, testRegisteredTools, zap.New(core))
	require.NoError(t, err)

	assert.Empty(t, logs.FilterMessage("skipping invalid operator skill").All(),
		"the nested SKILL.md must not be validated at all")
	assert.Empty(t, excluded, "a nested SKILL.md must not exclude its ancestor")
}

func TestOpenOperatorSkillsDir_ExcludesOversizeSkillFile(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "good-skill/SKILL.md", validSkillMD("good-skill"))
	oversize := "---\nname: huge-skill\n" + strings.Repeat("x", maxSkillValidationReadSize+1) + "\n---\nbody\n"
	writeSkillFile(t, dir, "huge-skill/SKILL.md", oversize)

	core, logs := observer.New(zap.WarnLevel)
	_, excluded, err := openOperatorSkillsDir(dir, testRegisteredTools, zap.New(core))
	require.NoError(t, err, "an oversize skill must not fail construction")

	warnings := logs.FilterMessage("skipping invalid operator skill").All()
	require.Len(t, warnings, 1)
	assert.Equal(t, "huge-skill/SKILL.md", warnings[0].ContextMap()["file"])
	assert.True(t, excluded["huge-skill"])
}

func TestOpenOperatorSkillsDir_AllowsSmallFrontmatterWithLargeBody(t *testing.T) {
	dir := t.TempDir()
	large := validSkillMD("large-body-skill") + strings.Repeat("x", maxSkillValidationReadSize*2)
	writeSkillFile(t, dir, "large-body-skill/SKILL.md", large)

	core, logs := observer.New(zap.WarnLevel)
	_, excluded, err := openOperatorSkillsDir(dir, testRegisteredTools, zap.New(core))
	require.NoError(t, err)

	assert.Empty(t, logs.FilterMessage("skipping invalid operator skill").All(),
		"the size cap must not reject valid frontmatter followed by a large body")
	assert.Empty(t, excluded)
}

func TestOpenOperatorSkillsDir_SkipsUnreadableSubdirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not enforced the same way on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root ignores directory permission bits")
	}
	dir := t.TempDir()
	writeSkillFile(t, dir, "good-skill/SKILL.md", validSkillMD("good-skill"))
	unreadable := filepath.Join(dir, "locked-skill")
	require.NoError(t, os.Mkdir(unreadable, 0o755))
	writeSkillFile(t, dir, "locked-skill/SKILL.md", validSkillMD("locked-skill"))
	require.NoError(t, os.Chmod(unreadable, 0o000))
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })

	core, logs := observer.New(zap.WarnLevel)
	operator, _, err := openOperatorSkillsDir(dir, testRegisteredTools, zap.New(core))
	require.NoError(t, err, "an unreadable subdirectory must not fail construction")

	require.Len(t, logs.FilterMessage("skipping unreadable path in skills_dir").All(), 1)
	_, err = fs.ReadFile(operator, "good-skill/SKILL.md")
	require.NoError(t, err, "sibling skills must still be served")
}

func TestOpenOperatorSkillsDir_ExcludesSkillWithMalformedFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "good-skill/SKILL.md", validSkillMD("good-skill"))
	// No "---" frontmatter block at all: decoding fails before validation.
	writeSkillFile(t, dir, "unparseable-skill/SKILL.md", "# Just markdown, no frontmatter\n")

	core, logs := observer.New(zap.WarnLevel)
	_, excluded, err := openOperatorSkillsDir(dir, testRegisteredTools, zap.New(core))
	require.NoError(t, err, "a malformed skill must not fail construction")

	warnings := logs.FilterMessage("skipping invalid operator skill").All()
	require.Len(t, warnings, 1)
	assert.Equal(t, "unparseable-skill/SKILL.md", warnings[0].ContextMap()["file"])
	assert.True(t, excluded["unparseable-skill"])
}

func TestOpenOperatorSkillsDir_ExcludesSkillWithUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}
	dir := t.TempDir()
	writeSkillFile(t, dir, "good-skill/SKILL.md", validSkillMD("good-skill"))
	brokenDir := filepath.Join(dir, "broken-skill")
	require.NoError(t, os.MkdirAll(brokenDir, 0o755))
	require.NoError(t, os.Symlink(filepath.Join(brokenDir, "missing-target"), filepath.Join(brokenDir, "SKILL.md")))

	core, logs := observer.New(zap.WarnLevel)
	_, excluded, err := openOperatorSkillsDir(dir, testRegisteredTools, zap.New(core))
	require.NoError(t, err, "a skill file that cannot be read must not fail construction")

	warnings := logs.FilterMessage("skipping invalid operator skill").All()
	require.Len(t, warnings, 1)
	assert.Equal(t, "broken-skill/SKILL.md", warnings[0].ContextMap()["file"])
	assert.True(t, excluded["broken-skill"])
}

func TestOpenOperatorSkillsDir_BlocksSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600))

	dir := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "evil")))

	operator, _, err := openOperatorSkillsDir(dir, testRegisteredTools, zap.NewNop())
	require.NoError(t, err)

	// os.OpenRoot refuses to follow the symlink out of skills_dir.
	_, err = operator.Open("evil/secret.txt")
	require.Error(t, err, "a symlink pointing outside skills_dir must not be followed")

	var pathErr *fs.PathError
	require.ErrorAs(t, err, &pathErr)
}
