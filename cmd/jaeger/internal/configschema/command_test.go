// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/otelcol"
)

func TestCommand_JSON(t *testing.T) {
	orig := generateFunc
	defer func() { generateFunc = orig }()
	generateFunc = func(_ otelcol.Factories) (*Schema, error) {
		return &Schema{
			Schema: "https://json-schema.org/draft/2020-12/schema",
			Type:   "object",
		}, nil
	}

	dir := t.TempDir()
	output := filepath.Join(dir, "out.json")
	cmd := Command(otelcol.Factories{})
	cmd.SetArgs([]string{"--format", "json", "--output", output})

	require.NoError(t, cmd.Execute())
	data, err := os.ReadFile(output)
	require.NoError(t, err)
	assert.Contains(t, string(data), "https://json-schema.org/draft/2020-12/schema")
}

func TestCommand_Markdown(t *testing.T) {
	orig := generateFunc
	defer func() { generateFunc = orig }()
	generateFunc = func(_ otelcol.Factories) (*Schema, error) {
		return &Schema{
			Title:       "Jaeger v2 component configuration schema",
			Type:        "object",
			Description: "Auto-generated reference.",
			Properties: map[string]*Schema{
				"test": {
					Ref:         "#/$defs/test_TestConfig",
					Description: "Test component.",
				},
			},
			Defs: map[string]*Schema{
				"test_TestConfig": {
					Type: "object",
					Properties: map[string]*Schema{
						"field": {Type: "string"},
					},
				},
			},
		}, nil
	}

	dir := t.TempDir()
	output := filepath.Join(dir, "out.md")
	cmd := Command(otelcol.Factories{})
	cmd.SetArgs([]string{"--format", "markdown", "--output", output})

	require.NoError(t, cmd.Execute())
	data, err := os.ReadFile(output)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "## test")
	assert.Contains(t, content, "| field | string | no | | |")
}

func TestCommand_MarkdownDefaultOutput(t *testing.T) {
	orig := generateFunc
	defer func() { generateFunc = orig }()
	generateFunc = func(_ otelcol.Factories) (*Schema, error) {
		return &Schema{
			Title: "Jaeger v2 component configuration schema",
			Type:  "object",
		}, nil
	}

	dir := t.TempDir()
	t.Chdir(dir)
	cmd := Command(otelcol.Factories{})
	cmd.SetArgs([]string{"--format", "markdown"})

	require.NoError(t, cmd.Execute())
	_, err := os.Stat(filepath.Join(dir, defaultMarkdownOutput))
	require.NoError(t, err)
}

func TestCommand_UnsupportedFormat(t *testing.T) {
	cmd := Command(otelcol.Factories{})
	cmd.SetArgs([]string{"--format", "yaml"})
	assert.ErrorContains(t, cmd.Execute(), "unsupported format")
}
