// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"context"
	"io/fs"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/handlers"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/types"
)

// The root index and each skill's frontmatter both carry that skill's
// description, and the two must be identical so they cannot drift. Two
// readers see them: an agent matches the index line before deciding whether
// to open a skill, while clients following the agentskills.io specification
// or the Open Knowledge Format read the frontmatter block.
func TestBuiltinSkillIndexMatchesFrontmatter(t *testing.T) {
	index, err := skillsEmbedFS.ReadFile("skills/SKILL.md")
	require.NoError(t, err, "the built-in skills always embed a root index")

	entries := parseSkillsIndex(string(index))
	require.NotEmpty(t, entries, "root index lists no skills")

	// A skill the index does not list is unreachable to an agent, and would
	// also slip past the description check below, so count both sides.
	// Index links are relative to the skills root, which is also where the
	// embedded filesystem stores them, one level above the package root.
	dirs, err := fs.ReadDir(skillsEmbedFS, "skills")
	require.NoError(t, err)
	builtins := 0
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		builtins++
		assert.Contains(t, entries, d.Name()+"/SKILL.md",
			"skill %s is not linked from the root index", d.Name())
	}
	assert.Len(t, entries, builtins,
		"root index and skills directory disagree on how many skills there are")

	for link, want := range entries {
		raw, err := skillsEmbedFS.ReadFile("skills/" + link)
		require.NoError(t, err, "read %s", link)

		skill, err := types.ParseSkill(string(raw))
		require.NoError(t, err, "parse frontmatter of %s", link)
		require.NotEmpty(t, skill.FrontMatter.Name,
			"skill file %s does not open with YAML frontmatter", link)

		assert.Equal(t, want, normalizeSpace(skill.FrontMatter.Description),
			"frontmatter description of %s does not match its index line in skills/SKILL.md", link)
	}
}

// read_skill serves the body of a skill, not the attributes its front matter
// declares, so none of the built-in skills may reach an agent with them.
func TestReadSkillServesBuiltinsWithoutFrontmatter(t *testing.T) {
	skills, err := fs.Sub(skillsEmbedFS, "skills")
	require.NoError(t, err)
	read := handlers.NewReadSkillHandler(skills, nil, DefaultServerParams().MaxReadFileSize)

	served := 0
	err = fs.WalkDir(skills, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != entryPointSkill {
			return err
		}
		served++
		raw, err := fs.ReadFile(skills, path)
		require.NoError(t, err)
		want, err := types.ParseSkill(string(raw))
		require.NoError(t, err, path)
		require.NotEqual(t, want.Content, string(raw), "%s has no front matter to strip", path)

		_, out, err := read(context.Background(), &mcp.CallToolRequest{}, types.ReadSkillInput{Path: path})
		require.NoError(t, err, path)
		assert.Equal(t, want.Content, out.Instructions, path)
		assert.False(t, strings.HasPrefix(out.Instructions, "---"), path)
		return nil
	})
	require.NoError(t, err)
	assert.Positive(t, served)
}

// parseSkillsIndex returns the description of each entry in the root catalog,
// keyed by the path the entry links to. An entry may wrap over several lines,
// and entries are separated by a blank line.
func parseSkillsIndex(index string) map[string]string {
	entries := map[string]string{}
	for _, block := range strings.Split(index, "\n\n") {
		block = strings.TrimSpace(block)
		if !strings.HasPrefix(block, "- [") {
			continue
		}
		link := strings.Index(block, "](")
		closed := strings.Index(block, ")")
		if link < 0 || closed < link {
			continue
		}
		desc := strings.TrimPrefix(strings.TrimSpace(block[closed+1:]), "—")
		entries[block[link+2:closed]] = normalizeSpace(desc)
	}
	return entries
}

// normalizeSpace collapses the line wrapping shared by the index entries and
// the folded YAML descriptions so the two can be compared.
func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
