// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

// TestCommand tests the Command function
func TestCommand(t *testing.T) {
	// Create a viper instance
	v := viper.New()

	// Test that the command is created with the correct properties
	cmd := Command(v)
	assert.Equal(t, "config-schema", cmd.Use)
	assert.Equal(t, "Generates JSON schema for configuration documentation", cmd.Short)

	// Test that the output flag is set with the correct default value
	outputFlag := cmd.Flag("output")
	require.NotNil(t, outputFlag)
	assert.Equal(t, defaultOutput, outputFlag.DefValue)
	assert.Equal(t, "Output file", outputFlag.Usage)
}

// TestCommandExecution tests the execution of the command
func TestCommandExecution(t *testing.T) {
	// Create a temporary directory for test output
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "test-output.json")

	// Save the original GenerateDocs function and restore it after the test
	originalGenerateDocs := genDocs
	defer func() { genDocs = originalGenerateDocs }()

	// Create a mock GenerateDocs function
	var capturedOutputPath string
	genDocs = func(path string) error {
		capturedOutputPath = path
		// Create an empty file to simulate successful generation
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		return nil
	}

	// Create a viper instance and command
	v := viper.New()
	cmd := Command(v)

	// Set the output flag
	cmd.SetArgs([]string{"--output", outputPath})

	// Execute the command
	err := cmd.Execute()
	require.NoError(t, err)

	// Verify that GenerateDocs was called with the correct output path
	assert.Equal(t, outputPath, capturedOutputPath)

	// Verify that the output file was created
	_, err = os.Stat(outputPath)
	assert.NoError(t, err)
}

// TestCommandExecutionError tests the error handling of the command
func TestCommandExecutionError(t *testing.T) {
	originalGenerateDocs := genDocs
	defer func() { genDocs = originalGenerateDocs }()

	genDocs = func(path string) error {
		return assert.AnError
	}

	// Create a viper instance and command
	v := viper.New()
	cmd := Command(v)

	// Execute the command
	err := cmd.Execute()
	require.Error(t, err)
	assert.Equal(t, assert.AnError, err)
}

// TestMain is used to set up and tear down the tests
func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
