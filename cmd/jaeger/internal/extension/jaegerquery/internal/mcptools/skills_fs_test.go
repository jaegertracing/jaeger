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
	"testing/fstest"

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

func testBuiltins() fstest.MapFS {
	return fstest.MapFS{
		"SKILL.md":               &fstest.MapFile{Data: []byte("builtin root catalog")},
		"builtin-skill/SKILL.md": &fstest.MapFile{Data: []byte("builtin skill")},
	}
}

func TestParseSkillFrontmatter(t *testing.T) {
	t.Run("valid with all spec fields", func(t *testing.T) {
		fm, err := parseSkillFrontmatter([]byte(`---
name: slow-db-call
description: Finds slow database calls.
license: Apache-2.0
metadata:
  author: jaeger
compatibility: requires Jaeger v2 with MCP enabled
allowed-tools: search_traces read_skill
---

# Body
`))
		require.NoError(t, err)
		assert.Equal(t, "slow-db-call", fm.Name)
		assert.Equal(t, "Finds slow database calls.", fm.Description)
		assert.Equal(t, "Apache-2.0", fm.License)
		assert.Equal(t, map[string]string{"author": "jaeger"}, fm.Metadata)
		assert.Equal(t, "requires Jaeger v2 with MCP enabled", fm.Compatibility)
		assert.Equal(t, "search_traces read_skill", fm.AllowedTools)
	})

	t.Run("missing frontmatter block", func(t *testing.T) {
		_, err := parseSkillFrontmatter([]byte("# Just markdown\n"))
		require.ErrorContains(t, err, "missing")
	})

	t.Run("unterminated frontmatter", func(t *testing.T) {
		_, err := parseSkillFrontmatter([]byte("---\nname: x\n"))
		require.ErrorContains(t, err, "not terminated")
	})

	t.Run("unknown key rejected by strict decoding", func(t *testing.T) {
		_, err := parseSkillFrontmatter([]byte("---\nname: x\nnot_a_real_field: oops\n---\nbody\n"))
		require.ErrorContains(t, err, "not_a_real_field")
	})

	t.Run("CRLF line endings are normalized", func(t *testing.T) {
		fm, err := parseSkillFrontmatter([]byte("---\r\nname: crlf-skill\r\ndescription: Authored on Windows.\r\n---\r\n\r\nBody\r\n"))
		require.NoError(t, err)
		assert.Equal(t, "crlf-skill", fm.Name)
		assert.Equal(t, "Authored on Windows.", fm.Description)
	})

	t.Run("embedded bare dashes mid-value do not truncate the block early", func(t *testing.T) {
		// A folded double-quoted YAML string can legitimately span raw lines;
		// one of those lines starts with "---" but isn't a full delimiter
		// line (it has trailing content), so it must not be mistaken for the
		// real terminator that follows.
		fm, err := parseSkillFrontmatter([]byte("---\nname: x\ndescription: \"line1\n---trailing\nline2\"\n---\n\nbody\n"))
		require.NoError(t, err)
		assert.Equal(t, "x", fm.Name)
		assert.Equal(t, "line1 ---trailing line2", fm.Description)
	})
}

func TestValidateSkillFrontmatter(t *testing.T) {
	registered := map[string]bool{"search_traces": true, "read_skill": true}
	valid := skillFrontmatter{Name: "good-skill", Description: "d"}

	tests := []struct {
		name    string
		mutate  func(*skillFrontmatter)
		dirName string
		wantErr string
	}{
		{name: "valid", mutate: func(*skillFrontmatter) {}, dirName: "good-skill"},
		{
			name:    "missing name",
			mutate:  func(fm *skillFrontmatter) { fm.Name = "" },
			dirName: "good-skill", wantErr: "name is required",
		},
		{
			name:    "missing description",
			mutate:  func(fm *skillFrontmatter) { fm.Description = "" },
			dirName: "good-skill", wantErr: "description is required",
		},
		{
			name:    "oversize name",
			mutate:  func(fm *skillFrontmatter) { fm.Name = strings.Repeat("a", maxSkillNameLen+1) },
			dirName: "good-skill", wantErr: "name exceeds",
		},
		{
			name:    "uppercase in name",
			mutate:  func(fm *skillFrontmatter) { fm.Name = "Good-Skill" },
			dirName: "good-skill", wantErr: "lowercase",
		},
		{
			name:    "leading hyphen",
			mutate:  func(fm *skillFrontmatter) { fm.Name = "-good" },
			dirName: "good-skill", wantErr: "lowercase",
		},
		{
			name:    "trailing hyphen",
			mutate:  func(fm *skillFrontmatter) { fm.Name = "good-" },
			dirName: "good-skill", wantErr: "lowercase",
		},
		{
			name:    "consecutive hyphens",
			mutate:  func(fm *skillFrontmatter) { fm.Name = "good--skill" },
			dirName: "good-skill", wantErr: "lowercase",
		},
		{
			name:    "name differs from directory",
			mutate:  func(*skillFrontmatter) {},
			dirName: "other-dir", wantErr: `must match its directory name "other-dir"`,
		},
		{
			name:    "oversize description",
			mutate:  func(fm *skillFrontmatter) { fm.Description = strings.Repeat("d", maxSkillDescriptionLen+1) },
			dirName: "good-skill", wantErr: "description exceeds",
		},
		{
			// Multi-byte UTF-8: well within the character limit, but over it
			// in bytes, so this must not be rejected by a byte-length check.
			name:    "multi-byte description within character limit",
			mutate:  func(fm *skillFrontmatter) { fm.Description = strings.Repeat("日", maxSkillDescriptionLen/2) },
			dirName: "good-skill",
		},
		{
			name:    "compatibility at limit accepted",
			mutate:  func(fm *skillFrontmatter) { fm.Compatibility = strings.Repeat("c", maxSkillCompatibilityLen) },
			dirName: "good-skill",
		},
		{
			name:    "oversize compatibility",
			mutate:  func(fm *skillFrontmatter) { fm.Compatibility = strings.Repeat("c", maxSkillCompatibilityLen+1) },
			dirName: "good-skill", wantErr: "compatibility exceeds",
		},
		{
			name:    "allowed-tools with registered tools",
			mutate:  func(fm *skillFrontmatter) { fm.AllowedTools = "search_traces read_skill" },
			dirName: "good-skill",
		},
		{
			name:    "allowed-tools with unregistered tool",
			mutate:  func(fm *skillFrontmatter) { fm.AllowedTools = "search_traces no_such_tool" },
			dirName: "good-skill", wantErr: `unregistered tool "no_such_tool"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fm := valid
			tc.mutate(&fm)
			err := validateSkillFrontmatter(fm, tc.dirName, registered)
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
		})
	}

	t.Run("multiple problems aggregated", func(t *testing.T) {
		err := validateSkillFrontmatter(skillFrontmatter{AllowedTools: "ghost"}, "d", registered)
		require.ErrorContains(t, err, "name is required")
		require.ErrorContains(t, err, "description is required")
		require.ErrorContains(t, err, `unregistered tool "ghost"`)
	})
}

func TestBuildMergedSkillsFS_EmptySkillsDirIsPassthrough(t *testing.T) {
	builtins := testBuiltins()
	merged, err := buildMergedSkillsFS(builtins, "", nil, zap.NewNop())
	require.NoError(t, err)
	assert.Equal(t, fs.FS(builtins), merged, "empty skills_dir must return the builtins unchanged")
}

func TestBuildMergedSkillsFS_HardFailsOnUnusablePath(t *testing.T) {
	t.Run("nonexistent directory", func(t *testing.T) {
		_, err := buildMergedSkillsFS(testBuiltins(), filepath.Join(t.TempDir(), "no-such-dir"), nil, zap.NewNop())
		require.ErrorContains(t, err, "cannot open skills_dir")
	})

	t.Run("path is a file", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "not-a-dir")
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
		_, err := buildMergedSkillsFS(testBuiltins(), f, nil, zap.NewNop())
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

		_, err := buildMergedSkillsFS(testBuiltins(), dir, nil, zap.NewNop())
		require.ErrorContains(t, err, "cannot list skills_dir")
	})
}

func TestBuildMergedSkillsFS_ServesOperatorSkillsUnderCustom(t *testing.T) {
	dir := t.TempDir()
	// The top-level SKILL.md is the operator's hand-written catalog: served
	// as-is, never validated as a skill (no frontmatter here on purpose).
	writeSkillFile(t, dir, "SKILL.md", "operator catalog")
	writeSkillFile(t, dir, "slow-db-call/SKILL.md", validSkillMD("slow-db-call"))

	merged, err := buildMergedSkillsFS(testBuiltins(), dir, map[string]bool{"read_skill": true}, zap.NewNop())
	require.NoError(t, err)

	catalog, err := fs.ReadFile(merged, "custom/SKILL.md")
	require.NoError(t, err)
	assert.Equal(t, "operator catalog", string(catalog))

	skill, err := fs.ReadFile(merged, "custom/slow-db-call/SKILL.md")
	require.NoError(t, err)
	assert.Contains(t, string(skill), "name: slow-db-call")

	// Built-ins remain reachable at the root.
	root, err := fs.ReadFile(merged, "SKILL.md")
	require.NoError(t, err)
	assert.Equal(t, "builtin root catalog", string(root))
	_, err = fs.ReadFile(merged, "builtin-skill/SKILL.md")
	require.NoError(t, err)
}

func TestBuildMergedSkillsFS_ExcludesInvalidSkillAndServesTheRest(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "good-skill/SKILL.md", validSkillMD("good-skill"))
	writeSkillFile(t, dir, "bad-skill/SKILL.md", "---\nname: MISMATCH\n---\nbody\n")

	core, logs := observer.New(zap.WarnLevel)
	merged, err := buildMergedSkillsFS(testBuiltins(), dir, map[string]bool{"read_skill": true}, zap.New(core))
	require.NoError(t, err, "an invalid skill must not fail construction")

	warnings := logs.FilterMessage("skipping invalid operator skill").All()
	require.Len(t, warnings, 1, "exactly one warning for the one bad skill")
	assert.Equal(t, "bad-skill/SKILL.md", warnings[0].ContextMap()["file"])

	_, err = merged.Open("custom/bad-skill/SKILL.md")
	require.ErrorIs(t, err, fs.ErrNotExist, "excluded skill must be invisible")
	_, err = merged.Open("custom/bad-skill")
	require.ErrorIs(t, err, fs.ErrNotExist, "the excluded skill's directory must be invisible too")

	_, err = fs.ReadFile(merged, "custom/good-skill/SKILL.md")
	require.NoError(t, err, "the good skill must still be served")
	_, err = fs.ReadFile(merged, "SKILL.md")
	require.NoError(t, err, "built-ins are unaffected")
}

func TestBuildMergedSkillsFS_IgnoresNestedSkillMD(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "good-skill/SKILL.md", validSkillMD("good-skill"))
	writeSkillFile(t, dir, "good-skill/docs/SKILL.md", "not frontmatter at all")

	core, logs := observer.New(zap.WarnLevel)
	merged, err := buildMergedSkillsFS(testBuiltins(), dir, map[string]bool{"read_skill": true}, zap.New(core))
	require.NoError(t, err)

	warnings := logs.FilterMessage("skipping invalid operator skill").All()
	assert.Empty(t, warnings, "the nested SKILL.md must not be validated at all")

	_, err = fs.ReadFile(merged, "custom/good-skill/SKILL.md")
	require.NoError(t, err, "the top-level skill must not be excluded by its nested SKILL.md")

	nested, err := fs.ReadFile(merged, "custom/good-skill/docs/SKILL.md")
	require.NoError(t, err)
	assert.Equal(t, "not frontmatter at all", string(nested))
}

func TestBuildMergedSkillsFS_ExcludesOversizeSkillFile(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "good-skill/SKILL.md", validSkillMD("good-skill"))
	oversize := "---\nname: huge-skill\n" + strings.Repeat("x", maxSkillValidationReadSize+1) + "\n---\nbody\n"
	writeSkillFile(t, dir, "huge-skill/SKILL.md", oversize)

	core, logs := observer.New(zap.WarnLevel)
	merged, err := buildMergedSkillsFS(testBuiltins(), dir, map[string]bool{"read_skill": true}, zap.New(core))
	require.NoError(t, err, "an oversize skill must not fail construction")

	warnings := logs.FilterMessage("skipping invalid operator skill").All()
	require.Len(t, warnings, 1)
	assert.Equal(t, "huge-skill/SKILL.md", warnings[0].ContextMap()["file"])

	_, err = merged.Open("custom/huge-skill/SKILL.md")
	require.ErrorIs(t, err, fs.ErrNotExist, "the oversize skill must be excluded")
	_, err = fs.ReadFile(merged, "custom/good-skill/SKILL.md")
	require.NoError(t, err, "the good skill must still be served")
}

func TestBuildMergedSkillsFS_AllowsSmallFrontmatterWithLargeBody(t *testing.T) {
	dir := t.TempDir()
	large := validSkillMD("large-body-skill") + strings.Repeat("x", maxSkillValidationReadSize*2)
	writeSkillFile(t, dir, "large-body-skill/SKILL.md", large)

	core, logs := observer.New(zap.WarnLevel)
	merged, err := buildMergedSkillsFS(testBuiltins(), dir, map[string]bool{"read_skill": true}, zap.New(core))
	require.NoError(t, err)

	warnings := logs.FilterMessage("skipping invalid operator skill").All()
	assert.Empty(t, warnings, "the size cap must not reject valid frontmatter followed by a large body")

	_, err = fs.ReadFile(merged, "custom/large-body-skill/SKILL.md")
	require.NoError(t, err)
}

func TestMergedSkillsFS_BlocksPathTraversal(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "SKILL.md", "operator catalog")
	merged, err := buildMergedSkillsFS(testBuiltins(), dir, nil, zap.NewNop())
	require.NoError(t, err)

	for _, p := range []string{
		"custom/../../etc/passwd",
		"custom/../SKILL.md",
		"../outside",
		"/etc/passwd",
	} {
		t.Run(p, func(t *testing.T) {
			_, err := merged.Open(p)
			require.Error(t, err)
		})
	}
}

func TestBuildMergedSkillsFS_SkipsUnreadableSubdirectory(t *testing.T) {
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
	merged, err := buildMergedSkillsFS(testBuiltins(), dir, map[string]bool{"read_skill": true}, zap.New(core))
	require.NoError(t, err, "an unreadable subdirectory must not fail construction")

	warnings := logs.FilterMessage("skipping unreadable path in skills_dir").All()
	require.Len(t, warnings, 1)

	_, err = fs.ReadFile(merged, "custom/good-skill/SKILL.md")
	require.NoError(t, err, "sibling skills must still be served")
}

func TestBuildMergedSkillsFS_ExcludesSkillWithMalformedFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "good-skill/SKILL.md", validSkillMD("good-skill"))
	// No "---" frontmatter block at all: parseSkillFrontmatter fails before
	// validateSkillFrontmatter ever runs.
	writeSkillFile(t, dir, "unparseable-skill/SKILL.md", "# Just markdown, no frontmatter\n")

	core, logs := observer.New(zap.WarnLevel)
	merged, err := buildMergedSkillsFS(testBuiltins(), dir, map[string]bool{"read_skill": true}, zap.New(core))
	require.NoError(t, err, "a malformed skill must not fail construction")

	warnings := logs.FilterMessage("skipping invalid operator skill").All()
	require.Len(t, warnings, 1)
	assert.Equal(t, "unparseable-skill/SKILL.md", warnings[0].ContextMap()["file"])

	_, err = merged.Open("custom/unparseable-skill/SKILL.md")
	require.ErrorIs(t, err, fs.ErrNotExist)
	_, err = fs.ReadFile(merged, "custom/good-skill/SKILL.md")
	require.NoError(t, err)
}

func TestBuildMergedSkillsFS_ExcludesSkillWithUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}
	dir := t.TempDir()
	writeSkillFile(t, dir, "good-skill/SKILL.md", validSkillMD("good-skill"))
	brokenDir := filepath.Join(dir, "broken-skill")
	require.NoError(t, os.MkdirAll(brokenDir, 0o755))
	require.NoError(t, os.Symlink(filepath.Join(brokenDir, "missing-target"), filepath.Join(brokenDir, "SKILL.md")))

	core, logs := observer.New(zap.WarnLevel)
	merged, err := buildMergedSkillsFS(testBuiltins(), dir, map[string]bool{"read_skill": true}, zap.New(core))
	require.NoError(t, err, "a skill file that cannot be read must not fail construction")

	warnings := logs.FilterMessage("skipping invalid operator skill").All()
	require.Len(t, warnings, 1)
	assert.Equal(t, "broken-skill/SKILL.md", warnings[0].ContextMap()["file"])

	_, err = fs.ReadFile(merged, "custom/good-skill/SKILL.md")
	require.NoError(t, err)
}

func TestMergedSkillsFS_OpensCustomDirListing(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "slow-db-call/SKILL.md", validSkillMD("slow-db-call"))
	merged, err := buildMergedSkillsFS(testBuiltins(), dir, map[string]bool{"read_skill": true}, zap.NewNop())
	require.NoError(t, err)

	f, err := merged.Open(customSkillsDir)
	require.NoError(t, err)
	defer f.Close()
	info, err := f.Stat()
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestMergedSkillsFS_BlocksSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600))

	dir := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "evil")))

	merged, err := buildMergedSkillsFS(testBuiltins(), dir, nil, zap.NewNop())
	require.NoError(t, err)

	// os.OpenRoot refuses to follow the symlink out of skills_dir.
	_, err = merged.Open("custom/evil/secret.txt")
	require.Error(t, err, "a symlink pointing outside skills_dir must not be followed")

	var pathErr *fs.PathError
	require.ErrorAs(t, err, &pathErr)
}
