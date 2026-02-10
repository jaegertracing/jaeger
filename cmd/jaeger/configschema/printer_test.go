// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrinterJSON(t *testing.T) {
	collection := &ConfigCollection{
		Configs: []ConfigInfo{
			{
				Name:        "TestConfig",
				PackagePath: "test/pkg",
				Fields: []FieldInfo{
					{
						Name:     "Field1",
						JSONName: "field1",
						Type:     "string",
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	printer := NewPrinter(FormatJSON, &buf)
	err := printer.Print(collection)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "TestConfig")
	assert.Contains(t, output, "field1")
}

func TestPrinterJSONPretty(t *testing.T) {
	collection := &ConfigCollection{
		Configs: []ConfigInfo{
			{
				Name:        "TestConfig",
				PackagePath: "test/pkg",
				Fields:      []FieldInfo{},
			},
		},
	}

	var buf bytes.Buffer
	printer := NewPrinter(FormatJSONPretty, &buf)
	err := printer.Print(collection)
	require.NoError(t, err)

	output := buf.String()
	// Pretty format should have newlines and indentation
	assert.Contains(t, output, "\n")
	assert.Contains(t, output, "  ")
}

func TestPrinterNilWriter(t *testing.T) {
	// Should default to os.Stdout
	printer := NewPrinter(FormatJSON, nil)
	assert.NotNil(t, printer.writer)
}
