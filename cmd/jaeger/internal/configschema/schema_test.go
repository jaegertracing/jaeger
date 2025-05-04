// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/configschema/testdata"
)

// normalizeRequiredFields sorts the required fields in a schema to ensure consistent ordering
func normalizeRequiredFields(schema map[string]any) {
	// Sort required fields in the components
	components, ok := schema["components"].(map[string]any)
	if !ok {
		return
	}
	schemas, ok2 := components["schemas"].(map[string]any)
	if !ok2 {
		return
	}
	for _, def := range schemas {
		if defMap, ok := def.(map[string]any); ok {
			if required, ok := defMap["required"].([]any); ok {
				// Convert to strings for sorting
				requiredStrs := make([]string, len(required))
				for i, r := range required {
					requiredStrs[i] = r.(string)
				}
				// Sort the strings
				sort.Strings(requiredStrs)
				// Convert back to any
				sortedRequired := make([]any, len(requiredStrs))
				for i, r := range requiredStrs {
					sortedRequired[i] = r
				}
				defMap["required"] = sortedRequired
			}
		}
	}
}

func normalizeJSONSchema(t *testing.T, schemaBytes []byte, fileName string) []byte {
	var jsonMap map[string]any
	require.NoError(t, json.Unmarshal(schemaBytes, &jsonMap))

	// Sort the required fields
	normalizeRequiredFields(jsonMap)

	// Convert back to JSON for comparison
	outBytes, err := json.MarshalIndent(jsonMap, "", "  ")
	require.NoError(t, err)

	// Write the normalized schema to a file for debugging
	err = os.WriteFile("testdata/normalized_"+fileName, outBytes, 0o644)
	require.NoError(t, err)

	return outBytes
}

func TestConstructSchemaWithSimpleConfig(t *testing.T) {
	expectedSchemaBytes, err := os.ReadFile(filepath.Join("testdata", "testconfig_schema.json"))
	require.NoError(t, err)

	configs := []any{&testdata.TestConfig{
		Name:    "test-service",
		Port:    8080,
		Enabled: true,
		Tags: map[string]string{
			"env": "test",
		},
		Addresses: []string{"localhost:8080"},
		NestedConfig: testdata.NestedConfig{
			Timeout:    30,
			RetryCount: 3,
		},
		DeprecatedField: "deprecated",
	}}
	pkgNames := collectPackages(configs)
	// TODO use more complex structs referring to different packages
	require.Equal(t, []string{
		"github.com/jaegertracing/jaeger/cmd/jaeger/internal/configschema/testdata",
	}, pkgNames)

	pkgs, err := loadPackages(pkgNames)
	require.NoError(t, err)

	// Generate schema for our test config using the package's constructSchema function
	schema, err := constructSchema(pkgs, configs)
	require.NoError(t, err)

	// Convert actual schema to JSON for comparison
	actualSchemaBytes, err := json.MarshalIndent(schema, "", "  ")
	require.NoError(t, err)

	// Compare the schemas, but first normalize the required fields order
	// TODO exporting schema as stable JSON should be in the prod functions
	expectedSchemaBytes = normalizeJSONSchema(t, expectedSchemaBytes, "testconfig_schema.json")
	actualSchemaBytes = normalizeJSONSchema(t, actualSchemaBytes, "actual_testconfig_schema.json")
	if !assert.JSONEq(t, string(expectedSchemaBytes), string(actualSchemaBytes)) {
		// write the actual schema to a file for debugging
		err = os.WriteFile("testdata/actual_testconfig_schema.json", actualSchemaBytes, 0o644)
		require.NoError(t, err)
	}
}
