// Copyright (c) The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// staticToolLister is a test double for ToolLister.
type staticToolLister struct {
	tools []string
	err   error
}

func (s *staticToolLister) ListTools(_ context.Context) ([]string, error) {
	return s.tools, s.err
}

var testTools = []string{
	"search_traces",
	"get_trace_topology",
	"get_critical_path",
	"get_span_details",
	"get_trace_errors",
	"get_services",
}

func writeSkillYAML(t *testing.T, dir, filename, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644))
}

const validSkillYAML = `
name: analyze-critical-path
description: Identifies the critical path and slowest spans in a trace.
system_prompt: |
  You are a distributed systems expert. Use the provided tools to identify
  performance bottlenecks. Do not speculate beyond the trace data.
allowed_tools:
  - get_critical_path
  - get_span_details
output_format: markdown
`

func TestLoader_Load_ValidSkill(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "critical-path.yaml", validSkillYAML)

	loader := NewLoader(dir, zaptest.NewLogger(t))
	err := loader.Load(context.Background(), &staticToolLister{tools: testTools})
	require.NoError(t, err)

	skill, ok := loader.Get("analyze-critical-path")
	require.True(t, ok)
	assert.Equal(t, "analyze-critical-path", skill.Name)
	assert.Equal(t, []string{"get_critical_path", "get_span_details"}, skill.AllowedTools)
}

func TestLoader_Load_NonExistentDir(t *testing.T) {
	loader := NewLoader("/does/not/exist", zaptest.NewLogger(t))
	err := loader.Load(context.Background(), &staticToolLister{tools: testTools})
	// Should not error when directory simply doesn't exist yet.
	require.NoError(t, err)
	assert.Empty(t, loader.All())
}

func TestLoader_Load_UnknownToolRef(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "bad.yaml", `
name: bad-skill
description: references a tool that does not exist
system_prompt: prompt
allowed_tools:
  - nonexistent_tool
`)

	loader := NewLoader(dir, zap.NewNop())
	err := loader.Load(context.Background(), &staticToolLister{tools: testTools})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent_tool")
	// The bad skill must not be registered.
	_, ok := loader.Get("bad-skill")
	assert.False(t, ok)
}

func TestLoader_Load_MixedValidAndInvalid(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "good.yaml", validSkillYAML)
	writeSkillYAML(t, dir, "bad.yaml", `
name: bad-skill
description: bad tool ref
system_prompt: prompt
allowed_tools:
  - nonexistent_tool
`)

	loader := NewLoader(dir, zap.NewNop())
	err := loader.Load(context.Background(), &staticToolLister{tools: testTools})
	require.Error(t, err) // error for bad-skill

	// good skill still loaded
	_, ok := loader.Get("analyze-critical-path")
	assert.True(t, ok)
	// bad skill NOT loaded
	_, ok = loader.Get("bad-skill")
	assert.False(t, ok)
}

func TestLoader_Load_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "broken.yaml", `name: [unclosed bracket`)

	loader := NewLoader(dir, zap.NewNop())
	err := loader.Load(context.Background(), &staticToolLister{tools: testTools})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken.yaml")
}

func TestLoader_Load_DuplicateSkillName(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "a.yaml", validSkillYAML)
	writeSkillYAML(t, dir, "b.yaml", validSkillYAML) // same name, different file

	loader := NewLoader(dir, zap.NewNop())
	err := loader.Load(context.Background(), &staticToolLister{tools: testTools})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "defined more than once")
}

func TestLoader_All(t *testing.T) {
	dir := t.TempDir()
	writeSkillYAML(t, dir, "a.yaml", validSkillYAML)

	loader := NewLoader(dir, zap.NewNop())
	require.NoError(t, loader.Load(context.Background(), &staticToolLister{tools: testTools}))

	all := loader.All()
	assert.Len(t, all, 1)
	assert.Contains(t, all, "analyze-critical-path")
}

func TestLoader_IgnoresNonYAMLFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{}`), 0o644))

	loader := NewLoader(dir, zap.NewNop())
	err := loader.Load(context.Background(), &staticToolLister{tools: testTools})
	require.NoError(t, err)
	assert.Empty(t, loader.All())
}
