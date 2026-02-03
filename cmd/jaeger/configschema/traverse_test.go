// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/configschema/testdata"
)

func TestTraverseSimpleConfig(t *testing.T) {
	cfg := &testdata.SimpleConfig{
		Port:  8080,
		Host:  "localhost",
		Debug: true,
	}

	info, err := traverseConfig(cfg)
	require.NoError(t, err)

	assert.Equal(t, "SimpleConfig", info.Name)
	assert.Contains(t, info.PackagePath, "testdata")
	assert.Len(t, info.Fields, 3)

	// Check Port field
	portField := findField(info.Fields, "Port")
	require.NotNil(t, portField)
	assert.Equal(t, "port", portField.JSONName)
	assert.Equal(t, reflect.Int, portField.Kind)
	assert.Equal(t, 8080, portField.Default)
	assert.False(t, portField.Required)

	// Check Host field (required)
	hostField := findField(info.Fields, "Host")
	require.NotNil(t, hostField)
	assert.Equal(t, "host", hostField.JSONName)
	assert.True(t, hostField.Required)

	// Check Debug field (omitempty)
	debugField := findField(info.Fields, "Debug")
	require.NotNil(t, debugField)
	assert.True(t, debugField.Omitempty)
}

func TestParseJSONTag(t *testing.T) {
	tests := []struct {
		name         string
		tag          string
		expectedName string
		expectedOmit bool
	}{
		{"simple", "fieldname", "fieldname", false},
		{"with omitempty", "fieldname,omitempty", "fieldname", true},
		{"skip field", "-", "-", false},
		{"multiple options", "name,omitempty,string", "name", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, omit := parseJSONTag(tt.tag)
			assert.Equal(t, tt.expectedName, name)
			assert.Equal(t, tt.expectedOmit, omit)
		})
	}
}

func TestIsFieldRequired(t *testing.T) {
	tests := []struct {
		name     string
		tag      reflect.StructTag
		expected bool
	}{
		{
			"mapstructure required",
			`mapstructure:"field,required"`,
			true,
		},
		{
			"valid required",
			`valid:"required"`,
			true,
		},
		{
			"not required",
			`mapstructure:"field"`,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isFieldRequired(tt.tag)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper function
func findField(fields []FieldInfo, name string) *FieldInfo {
	for _, f := range fields {
		if f.Name == name {
			return &f
		}
	}
	return nil
}
