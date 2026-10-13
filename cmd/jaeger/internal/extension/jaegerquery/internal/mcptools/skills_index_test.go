// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// The root index and each skill's frontmatter both carry that skill's
// description, and AUTHORING.md's checklist requires the two to be identical
// so they cannot drift. Two readers see them: an agent matches the index line
// before deciding whether to open a skill, while clients following the
// agentskills.io specification or the Open Knowledge Format read the
// frontmatter block. See #9838.

type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

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

		var fm skillFrontmatter
		require.NoError(t, yaml.Unmarshal(skillFrontmatterOf(t, raw), &fm),
			"parse frontmatter of %s", link)

		assert.Equal(t, want, normalizeSpace(fm.Description),
			"frontmatter description of %s does not match its index line "+
				"(AUTHORING.md checklist; see #9838)", link)
	}
}

// skillFrontmatterOf returns the YAML frontmatter block of a skill file, or
// fails the test if the file does not open with one.
func skillFrontmatterOf(t *testing.T, content []byte) []byte {
	t.Helper()
	text := string(content)
	require.True(t, strings.HasPrefix(text, "---\n"),
		"skill file does not open with YAML frontmatter")
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---")
	require.GreaterOrEqual(t, end, 0, "skill file frontmatter is not terminated")
	return []byte(rest[:end])
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
