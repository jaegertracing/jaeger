// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
    "context"
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "go.uber.org/zap/zaptest"

    "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/skills"
)

type staticToolLister struct{ tools []string }

func (s *staticToolLister) ListTools(_ context.Context) ([]string, error) { return s.tools, nil }

func newTestLoader(t *testing.T) *skills.Loader {
    t.Helper()
    dir := t.TempDir()
    require.NoError(t, os.WriteFile(filepath.Join(dir, "test.yaml"), []byte(`
name: analyze-critical-path
description: Identifies the critical path and slowest spans in a trace.
system_prompt: "You are a distributed tracing expert."
allowed_tools:
  - get_critical_path
  - get_span_details
`), 0o644))
    loader := skills.NewLoader(dir, zaptest.NewLogger(t))
    lister := &staticToolLister{tools: []string{"get_critical_path", "get_span_details"}}
    require.NoError(t, loader.Load(context.Background(), lister))
    return loader
}

func TestListSkillsHandler_ReturnsSummaries(t *testing.T) {
    loader := newTestLoader(t)
    h := &listSkillsHandler{loader: loader}
    _, output, err := h.handle(context.Background(), nil, ListSkillsInput{})
    require.NoError(t, err)
    require.Len(t, output.Skills, 1)
    assert.Equal(t, "analyze-critical-path", output.Skills[0].Name)
}

func TestGetSkillHandler_Found(t *testing.T) {
    loader := newTestLoader(t)
    h := &getSkillHandler{loader: loader}
    _, output, err := h.handle(context.Background(), nil, GetSkillInput{Name: "analyze-critical-path"})
    require.NoError(t, err)
    require.NotNil(t, output.Skill)
    assert.Equal(t, "analyze-critical-path", output.Skill.Name)
}

func TestGetSkillHandler_NotFound(t *testing.T) {
    loader := newTestLoader(t)
    h := &getSkillHandler{loader: loader}
    _, _, err := h.handle(context.Background(), nil, GetSkillInput{Name: "nonexistent"})
    require.Error(t, err)
    assert.Contains(t, err.Error(), "not found")
}

func TestGetSkillHandler_EmptyName(t *testing.T) {
    loader := newTestLoader(t)
    h := &getSkillHandler{loader: loader}
    _, _, err := h.handle(context.Background(), nil, GetSkillInput{Name: ""})
    require.Error(t, err)
    assert.Contains(t, err.Error(), "name is required")
}
