// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
)

func TestViperize(t *testing.T) {
	intFlag := "intFlag"
	stringFlag := "stringFlag"
	durationFlag := "durationFlag"

	expectedInt := 5
	expectedString := "string"
	expectedDuration := 13 * time.Second

	addFlags := func(flagSet *flag.FlagSet) {
		flagSet.Int(intFlag, 0, "")
		flagSet.String(stringFlag, "", "")
		flagSet.Duration(durationFlag, 0, "")
	}

	v, command := Viperize(addFlags)
	command.ParseFlags([]string{
		fmt.Sprintf("--%s=%d", intFlag, expectedInt),
		fmt.Sprintf("--%s=%s", stringFlag, expectedString),
		fmt.Sprintf("--%s=%s", durationFlag, expectedDuration.String()),
	})

	assert.Equal(t, expectedInt, v.GetInt(intFlag))
	assert.Equal(t, expectedString, v.GetString(stringFlag))
	assert.Equal(t, expectedDuration, v.GetDuration(durationFlag))
}

func TestEnv(t *testing.T) {
	envFlag := "jaeger.test-flag"
	actualEnvFlag := "JAEGER_TEST_FLAG"

	addFlags := func(flagSet *flag.FlagSet) {
		flagSet.String(envFlag, "", "")
	}
	expectedString := "string"
	t.Setenv(actualEnvFlag, expectedString)

	v, _ := Viperize(addFlags)
	assert.Equal(t, expectedString, v.GetString(envFlag))
}

// Tests for optional fields functionality

func TestProcessOptionalPointers(t *testing.T) {
	type SubConfig struct {
		Name string `mapstructure:"name"`
		Port int    `mapstructure:"port"`
	}

	type NestedConfig struct {
		OptionalSub *SubConfig `mapstructure:"optional_sub"`
		RequiredStr string     `mapstructure:"required_str"`
	}

	type Config struct {
		Required  string       `mapstructure:"required"`
		Optional1 *SubConfig   `mapstructure:"optional1"`
		Optional2 *SubConfig   `mapstructure:"optional2"`
		Nested    NestedConfig `mapstructure:"nested"`
	}

	tests := []struct {
		name               string
		input              map[string]interface{}
		expectOpt1Nil      bool
		expectOpt2Nil      bool
		expectNestedSubNil bool
	}{
		{
			name: "all fields set",
			input: map[string]interface{}{
				"required": "test",
				"optional1": map[string]interface{}{
					"name": "service1",
					"port": 8080,
				},
				"optional2": map[string]interface{}{
					"name": "service2",
					"port": 9090,
				},
				"nested": map[string]interface{}{
					"optional_sub": map[string]interface{}{
						"name": "nested-service",
						"port": 3000,
					},
					"required_str": "nested-test",
				},
			},
			expectOpt1Nil:      false,
			expectOpt2Nil:      false,
			expectNestedSubNil: false,
		},
		{
			name: "only required fields set",
			input: map[string]interface{}{
				"required": "test",
				"nested": map[string]interface{}{
					"required_str": "nested-test",
				},
			},
			expectOpt1Nil:      true,
			expectOpt2Nil:      true,
			expectNestedSubNil: true,
		},
		{
			name: "partial optional fields set",
			input: map[string]interface{}{
				"required": "test",
				"optional1": map[string]interface{}{
					"name": "service1",
					"port": 8080,
				},
				"nested": map[string]interface{}{
					"optional_sub": map[string]interface{}{
						"name": "nested-service",
						"port": 3000,
					},
					"required_str": "nested-test",
				},
			},
			expectOpt1Nil:      false,
			expectOpt2Nil:      true,
			expectNestedSubNil: false,
		},
		{
			name: "explicit null values",
			input: map[string]interface{}{
				"required":  "test",
				"optional1": nil,
				"optional2": nil,
				"nested": map[string]interface{}{
					"optional_sub": nil,
					"required_str": "nested-test",
				},
			},
			expectOpt1Nil:      true,
			expectOpt2Nil:      true,
			expectNestedSubNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := confmap.NewFromStringMap(tt.input)

			// Create config with default values (simulating factory default)
			config := &Config{
				Optional1: &SubConfig{Name: "default1", Port: 1111},
				Optional2: &SubConfig{Name: "default2", Port: 2222},
				Nested: NestedConfig{
					OptionalSub: &SubConfig{Name: "default-nested", Port: 3333},
				},
			}

			// First unmarshal normally
			err := conf.Unmarshal(config)
			require.NoError(t, err)

			// Then process optional pointers
			err = ProcessOptionalPointers(config, conf, "")
			require.NoError(t, err)

			// Verify results
			if tt.expectOpt1Nil {
				assert.Nil(t, config.Optional1, "Optional1 should be nil")
			} else {
				assert.NotNil(t, config.Optional1, "Optional1 should not be nil")
			}

			if tt.expectOpt2Nil {
				assert.Nil(t, config.Optional2, "Optional2 should be nil")
			} else {
				assert.NotNil(t, config.Optional2, "Optional2 should not be nil")
			}

			if tt.expectNestedSubNil {
				assert.Nil(t, config.Nested.OptionalSub, "Nested.OptionalSub should be nil")
			} else {
				assert.NotNil(t, config.Nested.OptionalSub, "Nested.OptionalSub should not be nil")
			}
		})
	}
}

func TestProcessOptionalPointers_Squash(t *testing.T) {
	type EmbeddedConfig struct {
		OptionalField *string `mapstructure:"optional_field"`
		RequiredField string  `mapstructure:"required_field"`
	}

	type Config struct {
		EmbeddedConfig `mapstructure:",squash"`
		TopLevel       *string `mapstructure:"top_level"`
	}

	input := map[string]interface{}{
		"required_field": "test",
		// optional_field and top_level are not set
	}

	conf := confmap.NewFromStringMap(input)

	// Create config with defaults
	config := &Config{
		EmbeddedConfig: EmbeddedConfig{
			OptionalField: stringPtr("default"),
		},
		TopLevel: stringPtr("default-top"),
	}

	// Unmarshal normally
	err := conf.Unmarshal(config)
	require.NoError(t, err)

	// Process optional pointers
	err = ProcessOptionalPointers(config, conf, "")
	require.NoError(t, err)

	// Verify that optional fields were set to nil
	assert.Nil(t, config.OptionalField, "EmbeddedConfig.OptionalField should be nil")
	assert.Nil(t, config.TopLevel, "TopLevel should be nil")
	assert.Equal(t, "test", config.RequiredField, "RequiredField should keep its value")
}

func TestProcessOptionalPointers_ErrorCases(t *testing.T) {
	conf := confmap.NewFromStringMap(map[string]interface{}{})

	tests := []struct {
		name        string
		config      interface{}
		expectError string
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: "config cannot be nil",
		},
		{
			name:        "non-pointer config",
			config:      struct{}{},
			expectError: "config must be a pointer to struct",
		},
		{
			name:        "pointer to non-struct",
			config:      stringPtr("test"),
			expectError: "config must be a pointer to struct",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ProcessOptionalPointers(tt.config, conf, "")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectError)
		})
	}
}

func TestIsOptionalFieldSet(t *testing.T) {
	input := map[string]interface{}{
		"set_field": "value",
		"nested": map[string]interface{}{
			"set_nested": "nested_value",
		},
	}

	conf := confmap.NewFromStringMap(input)

	tests := []struct {
		fieldPath string
		expected  bool
	}{
		{"set_field", true},
		{"unset_field", false},
		{"nested", true},             // nested object exists
		{"nested.set_nested", false}, // confmap.IsSet doesn't support dot notation for nested fields
		{"nonexistent.nested.field", false},
	}

	for _, tt := range tests {
		t.Run(tt.fieldPath, func(t *testing.T) {
			result := IsOptionalFieldSet(conf, tt.fieldPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
