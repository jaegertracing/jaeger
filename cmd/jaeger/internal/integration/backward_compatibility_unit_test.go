// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

func createMockJaeger(t *testing.T, versionJSON string) string {
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		batPath := filepath.Join(dir, "mock-jaeger.bat")
		content := `@echo off
if "%1"=="featuregate" (
    if "%2"=="known.gate" (
        exit /b 0
    )
    exit /b 1
)
if "%1"=="version" (
    echo ` + versionJSON + `
    exit /b 0
)
exit /b 2
`
		require.NoError(t, os.WriteFile(batPath, []byte(content), 0o755))
		return batPath
	}

	shPath := filepath.Join(dir, "mock-jaeger.sh")
	content := `#!/bin/sh
if [ "$1" = "featuregate" ]; then
    if [ "$2" = "known.gate" ]; then
        exit 0
    fi
    exit 1
elif [ "$1" = "version" ]; then
    echo '` + versionJSON + `'
    exit 0
fi
exit 2
`
	require.NoError(t, os.WriteFile(shPath, []byte(content), 0o755))
	return shPath
}

func TestProbeGate(t *testing.T) {
	mock := createMockJaeger(t, `{"gitVersion":"v2.24.0"}`)

	t.Run("known gate returns true", func(t *testing.T) {
		ok, err := probeGate(mock, "known.gate")
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("unknown gate returns false without error", func(t *testing.T) {
		ok, err := probeGate(mock, "unknown.gate")
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("nonexistent binary returns error", func(t *testing.T) {
		ok, err := probeGate(filepath.Join(t.TempDir(), "nonexistent-binary"), "known.gate")
		require.Error(t, err)
		assert.False(t, ok)
	})
}

func TestBinaryVersion(t *testing.T) {
	t.Run("gitVersion takes precedence", func(t *testing.T) {
		mock := createMockJaeger(t, `{"gitVersion":"v2.24.0","gitCommit":"abc1234"}`)
		assert.Equal(t, "v2.24.0", binaryVersion(mock))
	})

	t.Run("gitCommit fallback when gitVersion empty", func(t *testing.T) {
		mock := createMockJaeger(t, `{"gitVersion":"","gitCommit":"abc1234"}`)
		assert.Equal(t, "abc1234", binaryVersion(mock))
	})

	t.Run("basename fallback when version output is invalid", func(t *testing.T) {
		mock := createMockJaeger(t, `invalid-json`)
		assert.Equal(t, filepath.Base(mock), binaryVersion(mock))
	})
}

func TestResolveBinaryPath(t *testing.T) {
	t.Run("absolute path preserved", func(t *testing.T) {
		absPath := filepath.Join(string(filepath.Separator), "tmp", "jaeger")
		assert.Equal(t, absPath, resolveBinaryPath(absPath))
	})

	t.Run("relative path resolved to absolute", func(t *testing.T) {
		relPath := filepath.Join(".", "cmd", "jaeger", "jaeger")
		resolved := resolveBinaryPath(relPath)
		assert.True(t, filepath.IsAbs(resolved))
	})
}

func TestRunBackwardCompatibilityTests_SkipsWithoutEnv(t *testing.T) {
	t.Setenv(oldBinaryEnvVar, "")
	t.Setenv(oldConfigDirEnvVar, "")

	// Verify that runBackwardCompatibilityTests skips without panicking when env vars are missing
	runBackwardCompatibilityTests(t, "memory", E2EStorageIntegration{}, compatScenario{
		Name:         "default",
		Capabilities: capabilities.Memory(),
	})
}

func TestRunBackwardCompatibilityTests_SkipsUnknownOldGate(t *testing.T) {
	mock := createMockJaeger(t, `{"gitVersion":"v2.20.0"}`)
	t.Setenv(oldBinaryEnvVar, mock)
	t.Setenv(oldConfigDirEnvVar, t.TempDir())

	t.Run("wrapper", func(subT *testing.T) {
		runBackwardCompatibilityTests(subT, "memory", E2EStorageIntegration{}, compatScenario{
			Name:     "unknown-gate-scenario",
			OldGates: []string{"unknown.gate"},
		})
	})
}
