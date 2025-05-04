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
	"golang.org/x/tools/go/packages"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/configschema/testdata"
)

// normalizeRequiredFields sorts the required fields in a schema to ensure consistent ordering
func normalizeRequiredFields(schema map[string]interface{}) {
	// Sort required fields in the definitions
	if definitions, ok := schema["definitions"].(map[string]interface{}); ok {
		for _, def := range definitions {
			if defMap, ok := def.(map[string]interface{}); ok {
				if required, ok := defMap["required"].([]interface{}); ok {
					// Convert to strings for sorting
					requiredStrs := make([]string, len(required))
					for i, r := range required {
						requiredStrs[i] = r.(string)
					}
					// Sort the strings
					sort.Strings(requiredStrs)
					// Convert back to interface{}
					sortedRequired := make([]interface{}, len(requiredStrs))
					for i, r := range requiredStrs {
						sortedRequired[i] = r
					}
					defMap["required"] = sortedRequired
				}
			}
		}
	}
}

func normalizeJSONSchema(t *testing.T, schemaBytes []byte) []byte {
	var jsonMap map[string]any
	require.NoError(t, json.Unmarshal(schemaBytes, &jsonMap))

	// Sort the required fields
	normalizeRequiredFields(jsonMap)

	// Convert back to JSON for comparison
	outBytes, err := json.MarshalIndent(jsonMap, "", "  ")
	require.NoError(t, err)
	return outBytes
}

func TestConstructSchemaWithSimpleConfig(t *testing.T) {
	expectedSchemaBytes, err := os.ReadFile(filepath.Join("testdata", "testconfig_schema.json"))
	require.NoError(t, err)

	// Create a test config
	testConfig := &testdata.TestConfig{
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
	}

	// Load the package containing our test structs
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes | packages.NeedName,
	}
	pkgs, err := packages.Load(cfg, "github.com/jaegertracing/jaeger/cmd/jaeger/internal/configschema/testdata")
	require.NoError(t, err)

	// Generate schema for our test config using the package's constructSchema function
	schema, err := constructSchema(pkgs, []any{testConfig})
	require.NoError(t, err)

	// Convert actual schema to JSON for comparison
	actualSchemaBytes, err := json.MarshalIndent(schema, "", "  ")
	require.NoError(t, err)

	// Compare the schemas, but first normalize the required fields order
	// TODO exporting schema as stable JSON should be in the prod functions
	expectedSchemaBytes = normalizeJSONSchema(t, expectedSchemaBytes)
	actualSchemaBytes = normalizeJSONSchema(t, actualSchemaBytes)
	if !assert.JSONEq(t, string(expectedSchemaBytes), string(actualSchemaBytes)) {
		// write the actual schema to a file for debugging
		err = os.WriteFile("testdata/actual_testconfig_schema.json", actualSchemaBytes, 0o644)
		require.NoError(t, err)
	}
}
