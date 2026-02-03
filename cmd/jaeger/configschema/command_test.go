// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectConfigs(t *testing.T) {
	configs := collectConfigs()

	// Should have 6 Jaeger extensions
	assert.Equal(t, 6, len(configs))

	// Check none are nil
	for i, cfg := range configs {
		assert.NotNil(t, cfg, "config at index %d is nil", i)
	}
}

func TestGenerateSchemaE2E(t *testing.T) {
	configs := collectConfigs()
	require.NotEmpty(t, configs)

	collection, err := extractConfigInfoWithComments(configs)
	require.NoError(t, err)
	require.NotNil(t, collection)

	// Should have 6 configs
	assert.Len(t, collection.Configs, 6)

	// Each should have fields
	for _, cfg := range collection.Configs {
		assert.NotEmpty(t, cfg.Name)
		assert.NotEmpty(t, cfg.PackagePath)
		assert.NotEmpty(t, cfg.Fields)
	}
}

func TestPrintJSON(t *testing.T) {
	collection := &ConfigCollection{
		Configs: []ConfigInfo{
			{
				Name:        "TestConfig",
				PackagePath: "test/pkg",
				Fields: []FieldInfo{
					{
						Name:     "Port",
						JSONName: "port",
						Type:     "int",
						Comment:  "Server port",
						Default:  8080,
						Required: true,
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	printer := NewPrinter(FormatJSONPretty, &buf)
	err := printer.Print(collection)
	require.NoError(t, err)

	// Parse back
	var result ConfigCollection
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Len(t, result.Configs, 1)
	assert.Equal(t, "TestConfig", result.Configs[0].Name)
}

func TestCommandExecution(t *testing.T) {
	tmpFile := t.TempDir() + "/schema.json"

	err := runGenerateSchema(tmpFile)
	require.NoError(t, err)

	data, err := os.ReadFile(tmpFile)
	require.NoError(t, err)

	var collection ConfigCollection
	err = json.Unmarshal(data, &collection)
	require.NoError(t, err)

	assert.NotEmpty(t, collection.Configs)
}
