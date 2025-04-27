// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
)

func TestGenerateDocs(t *testing.T) {
	tempDir := t.TempDir()
	err := GenerateSchema(filepath.Join(tempDir, "test-output.json"))
	require.NoError(t, err)

	err = writeSchema(&JSONSchema{}, filepath.Join(filepath.Join(tempDir, "badsubdir"), "test-output.json"))
	require.ErrorContains(t, err, "failed to create output file")
}

func TestCollectPackages(t *testing.T) {
	configs := []any{
		&otlpreceiver.Config{},
	}
	packages := collectPackages(configs)
	expected := []string{
		"go.opentelemetry.io/collector/component",
		"go.opentelemetry.io/collector/config/configauth",
		"go.opentelemetry.io/collector/config/configgrpc",
		"go.opentelemetry.io/collector/config/confighttp",
		"go.opentelemetry.io/collector/config/confignet",
		"go.opentelemetry.io/collector/config/configtls",
		"go.opentelemetry.io/collector/receiver/otlpreceiver",
	}
	assert.Equal(t, expected, packages)
}

func TestConstructSchema(t *testing.T) {
	configs := []any{
		&otlpreceiver.Config{},
	}
	packages := collectPackages(configs)
	pkgs, err := loadPackages(packages)
	require.NoError(t, err)
	schema, err := constructSchema(pkgs, configs)
	require.NoError(t, err)
	assert.NotNil(t, schema)
	str, err := json.MarshalIndent(schema, "", "  ")
	require.NoError(t, err)
	t.Logf("Generated schema:\n%s", string(str))
}
