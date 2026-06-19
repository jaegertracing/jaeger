// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package skills

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBuiltinSkills_LoadsRequiredSkills(t *testing.T) {
	skills := BuiltinSkills(zap.NewNop())
	require.NotEmpty(t, skills, "expected at least one built-in skill")

	names := make(map[string]bool, len(skills))
	for _, s := range skills {
		names[s.Name] = true
	}
	assert.True(t, names["skills-index"], "skills-index should be loaded")
	assert.True(t, names["greet-user"], "greet-user should be loaded")
	assert.True(t, names["echo-message"], "echo-message should be loaded")
}

func TestBuiltinSkills_ParsesFrontmatter(t *testing.T) {
	skills := BuiltinSkills(zap.NewNop())
	byName := make(map[string]Skill, len(skills))
	for _, s := range skills {
		byName[s.Name] = s
	}

	tests := []struct {
		name            string
		wantDescContain string
	}{
		{
			name:            "skills-index",
			wantDescContain: "discover",
		},
		{
			name:            "greet-user",
			wantDescContain: "greeting",
		},
		{
			name:            "echo-message",
			wantDescContain: "uppercase",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, ok := byName[tc.name]
			require.True(t, ok, "skill %q not found", tc.name)
			assert.Equal(t, tc.name, s.Name)
			assert.NotEmpty(t, s.Description)
			assert.Contains(t, strings.ToLower(s.Description), tc.wantDescContain)
		})
	}
}

func TestBuiltinSkills_BodyIncludesFrontmatter(t *testing.T) {
	skills := BuiltinSkills(zap.NewNop())
	for _, s := range skills {
		t.Run(s.Name, func(t *testing.T) {
			assert.True(t, strings.HasPrefix(s.Body, "---\n"),
				"body for %q should start with frontmatter delimiter", s.Name)
			assert.Contains(t, s.Body, "name: "+s.Name,
				"body for %q should contain its own name in frontmatter", s.Name)
		})
	}
}

func TestBuiltinSkills_LenientValidation(t *testing.T) {
	// A skill whose name field exceeds 64 characters should still load (warn only).
	longName := strings.Repeat("a", 65)
	skillMD := "---\nname: " + longName + "\ndescription: Test description for lenient validation.\n---\n\n# Body\n"
	skill, ok := parseSkill("skills/"+longName+"/SKILL.md", []byte(skillMD), zap.NewNop())
	require.True(t, ok, "skill with long name should load despite warning")
	assert.Equal(t, longName, skill.Name)
	assert.Equal(t, "Test description for lenient validation.", skill.Description)
}

func TestParseSkill_MissingOpeningDelimiter(t *testing.T) {
	_, ok := parseSkill("skills/x/SKILL.md", []byte("name: foo\ndescription: bar\n"), zap.NewNop())
	assert.False(t, ok)
}

func TestParseSkill_UnclosedFrontmatter(t *testing.T) {
	_, ok := parseSkill("skills/x/SKILL.md", []byte("---\nname: foo\ndescription: bar\n"), zap.NewNop())
	assert.False(t, ok)
}

func TestParseSkill_MissingDescription(t *testing.T) {
	_, ok := parseSkill("skills/x/SKILL.md", []byte("---\nname: foo\n---\n\n# Body\n"), zap.NewNop())
	assert.False(t, ok)
}

func TestParseSkill_MissingName_UsesDir(t *testing.T) {
	skill, ok := parseSkill("skills/my-skill/SKILL.md", []byte("---\ndescription: A skill without a name field.\n---\n\n# Body\n"), zap.NewNop())
	require.True(t, ok)
	assert.Equal(t, "my-skill", skill.Name)
}

func TestParseSkill_NameDirMismatch_StillLoads(t *testing.T) {
	skill, ok := parseSkill("skills/actual-dir/SKILL.md", []byte("---\nname: different-name\ndescription: Mismatch test.\n---\n\n# Body\n"), zap.NewNop())
	require.True(t, ok)
	assert.Equal(t, "different-name", skill.Name)
}

func TestParseSkill_ClosingDelimiterAtEOF(t *testing.T) {
	// Frontmatter closed by "---" at end-of-file with no trailing newline.
	skill, ok := parseSkill("skills/eof/SKILL.md", []byte("---\nname: eof\ndescription: Closing delimiter at EOF.\n---"), zap.NewNop())
	require.True(t, ok)
	assert.Equal(t, "eof", skill.Name)
	assert.Equal(t, "Closing delimiter at EOF.", skill.Description)
}

func TestParseSkill_InvalidYAML(t *testing.T) {
	_, ok := parseSkill("skills/x/SKILL.md", []byte("---\nname: [unterminated\n---\n\n# Body\n"), zap.NewNop())
	assert.False(t, ok)
}

func TestParseSkill_NoDirInPath(t *testing.T) {
	// A path without a parent directory yields an empty dir name; the skill still
	// loads because it carries an explicit name.
	skill, ok := parseSkill("SKILL.md", []byte("---\nname: rootless\ndescription: No parent dir.\n---\n\n# Body\n"), zap.NewNop())
	require.True(t, ok)
	assert.Equal(t, "rootless", skill.Name)
}

func TestParseSkill_NoNameNoDirSkipped(t *testing.T) {
	_, ok := parseSkill("SKILL.md", []byte("---\ndescription: No name and no parent dir.\n---\n\n# Body\n"), zap.NewNop())
	assert.False(t, ok)
}

func TestBuiltinSkills_ReturnsIndependentCopy(t *testing.T) {
	// Mutating the returned slice must not corrupt the cache shared across callers.
	first := BuiltinSkills(zap.NewNop())
	require.NotEmpty(t, first)
	first[0] = Skill{Name: "mutated"}

	second := BuiltinSkills(zap.NewNop())
	assert.NotEqual(t, "mutated", second[0].Name, "cache must be immune to caller mutation")
}
