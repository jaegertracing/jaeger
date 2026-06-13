// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestCachedFileTokenLoader_Deterministic covers basic cache and reload logic with mock time
func TestCachedFileTokenLoader_Deterministic(t *testing.T) {
	currentTime := time.Unix(0, 0)
	timeFn := func() time.Time { return currentTime }

	tokenFile := createTempTokenFile(t, "my-secret-token\n")

	loader := cachedFileTokenLoader(tokenFile, 100*time.Millisecond, timeFn)

	// T=0: First load - should read from file
	token1, err := loader()
	require.NoError(t, err)
	assert.Equal(t, "my-secret-token", token1)

	// Change the file content
	updateTokenFile(t, tokenFile, "new-token\n")

	// T=50ms: Still within cache interval (< 100ms)
	currentTime = currentTime.Add(50 * time.Millisecond)
	token2, err := loader()
	require.NoError(t, err)
	assert.Equal(t, "my-secret-token", token2, "Should return cached token within interval")

	// T=150ms: Beyond cache interval (> 100ms)
	currentTime = currentTime.Add(100 * time.Millisecond) // Total: 150ms
	token3, err := loader()
	require.NoError(t, err)
	assert.Equal(t, "new-token", token3, "Should return refreshed token after cache expires")
}

// TestCachedFileTokenLoader_ExactBoundaries tests exact cache boundary conditions
func TestCachedFileTokenLoader_ExactBoundaries(t *testing.T) {
	currentTime := time.Unix(0, 0)
	timeFn := func() time.Time { return currentTime }
	tokenFile := createTempTokenFile(t, "boundary-token\n")
	cacheInterval := 200 * time.Millisecond

	loader := cachedFileTokenLoader(tokenFile, cacheInterval, timeFn)

	// Load initial token
	token, err := loader()
	require.NoError(t, err)
	assert.Equal(t, "boundary-token", token)

	// Update file
	updateTokenFile(t, tokenFile, "boundary-updated\n")

	// Test exact boundary conditions
	testCases := []struct {
		timeAdvance   time.Duration
		expectedToken string
		description   string
	}{
		{199 * time.Millisecond, "boundary-token", "1ms before cache expires"},
		{1 * time.Millisecond, "boundary-updated", "exactly at cache expiry"},
		{50 * time.Millisecond, "boundary-updated", "well past cache expiry"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			currentTime = currentTime.Add(tc.timeAdvance)
			token, err := loader()
			require.NoError(t, err)
			assert.Equal(t, tc.expectedToken, token, tc.description)
		})
	}
}

// TestCachedFileTokenLoader_ZeroInterval tests disabled reloading
func TestCachedFileTokenLoader_ZeroInterval(t *testing.T) {
	currentTime := time.Unix(0, 0)
	timeFn := func() time.Time { return currentTime }
	tokenFile := createTempTokenFile(t, "initial-token\n")
	loader := cachedFileTokenLoader(tokenFile, 0, timeFn) // Zero interval = no reloading

	// T=0: Initial load
	token, err := loader()
	require.NoError(t, err)
	assert.Equal(t, "initial-token", token)

	// Update file content
	updateTokenFile(t, tokenFile, "updated-token\n")

	// Should still return cached token (no reloading)
	currentTime = currentTime.Add(1 * time.Hour) // Advance time significantly
	token, err = loader()
	require.NoError(t, err)
	assert.Equal(t, "initial-token", token, "Zero interval should disable reloading completely")

	// Multiple calls should continue returning cached token
	for i := range 5 {
		// Generate different content for each iteration
		newContent := fmt.Sprintf("different-token-%d-%d\n", i, currentTime.Unix())
		updateTokenFile(t, tokenFile, newContent)

		currentTime = currentTime.Add(1 * time.Hour)
		token, err = loader()
		require.NoError(t, err)
		assert.Equal(t, "initial-token", token,
			"Should always return initially cached token despite file change to %s at time %v",
			strings.TrimSpace(newContent), currentTime)
	}
}

// TestNewTokenProvider_InitialLoad covers initial load and fail-fast scenarios
func TestNewTokenProvider_InitialLoad(t *testing.T) {
	// Test successful initial load
	tokenFile := createTempTokenFile(t, "initial-token\n")

	tokenFn, err := TokenProvider(tokenFile, 100*time.Millisecond, nil)
	require.NoError(t, err, "TokenProvider should not fail with valid token file")
	assert.Equal(t, "initial-token", tokenFn(), "Token should match file contents")

	// Test fail-fast on invalid file
	_, err = TokenProvider("/nonexistent/file", 100*time.Millisecond, nil)
	require.Error(t, err, "TokenProvider should fail fast on missing file")
	assert.Contains(t, err.Error(), "failed to get token from file", "Error message should indicate token loading failure")

	// Test empty file
	emptyFile := createTempTokenFile(t, "")
	tokenFn, err = TokenProvider(emptyFile, 100*time.Millisecond, nil)
	require.NoError(t, err)
	assert.Empty(t, tokenFn(), "Empty file should return empty token")
	// Test file with trailing whitespace - properly trimmed
	whitespaceFile := createTempTokenFile(t, "my-secret-token\r\n")
	tokenFn, err = TokenProvider(whitespaceFile, 100*time.Millisecond, nil)
	require.NoError(t, err)
	assert.Equal(t, "my-secret-token", tokenFn(), "\\r\\n should be properly trimmed from end")
}

// TestNewTokenProvider_ReloadErrors_Deterministic ensures reload errors log and return cached token
func TestNewTokenProvider_ReloadErrors_Deterministic(t *testing.T) {
	currentTime := time.Unix(0, 0)
	timeFn := func() time.Time { return currentTime }
	tokenFile := createTempTokenFile(t, "initial-token\n")

	// Create an observed zap logger
	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)

	// Initialize token provider with mock time
	tokenFn, err := TokenProviderWithTime(tokenFile, 10*time.Millisecond, logger, timeFn)
	require.NoError(t, err)

	// Initial call should succeed
	token := tokenFn()
	assert.Equal(t, "initial-token", token)

	// Remove the file to force reload error
	os.Remove(tokenFile)

	// Advance time beyond cache interval
	currentTime = currentTime.Add(15 * time.Millisecond)

	// Call should return last cached token and log error
	token = tokenFn()
	assert.Equal(t, "initial-token", token, "Should return cached token even after file deletion")

	// Verify the error was logged
	require.Equal(t, 1, logs.Len(), "Expected one log message")
	logEntry := logs.All()[0]
	assert.Equal(t, "Token reload failed", logEntry.Message, "Expected log message to match")
	assert.Equal(t, zapcore.WarnLevel, logEntry.Level, "Expected warning level log")
}

// TestTokenProviderWithTime_DirectCall tests the time-injectable function directly
func TestTokenProviderWithTime_DirectCall(t *testing.T) {
	currentTime := time.Unix(0, 0)
	timeFn := func() time.Time { return currentTime }
	tokenFile := createTempTokenFile(t, "time-test-token\n")
	logger := zap.NewNop()

	// Create token provider with mock time
	tokenFn, err := TokenProviderWithTime(tokenFile, 200*time.Millisecond, logger, timeFn)
	require.NoError(t, err)
	require.NotNil(t, tokenFn)

	// Test initial token
	token := tokenFn()
	assert.Equal(t, "time-test-token", token)

	// Update file
	updateTokenFile(t, tokenFile, "time-updated\n")

	// Should still return cached token
	currentTime = currentTime.Add(100 * time.Millisecond)
	token = tokenFn()
	assert.Equal(t, "time-test-token", token)

	// Should return updated token after cache expires
	currentTime = currentTime.Add(150 * time.Millisecond)
	token = tokenFn()
	assert.Equal(t, "time-updated", token)
}

// TestNewTokenProvider_WithZapLogger ensures zap logger is used properly
func TestNewTokenProvider_WithZapLogger(t *testing.T) {
	currentTime := time.Unix(0, 0)
	timeFn := func() time.Time { return currentTime }
	tokenFile := createTempTokenFile(t, "initial-token\n")

	// Create an observed zap logger
	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)

	// Initialize token provider with structured logger
	tokenFn, err := TokenProviderWithTime(tokenFile, 10*time.Millisecond, logger, timeFn)
	require.NoError(t, err)

	// Initial call should succeed
	token := tokenFn()
	assert.Equal(t, "initial-token", token)

	// No logs yet
	assert.Equal(t, 0, logs.Len(), "No logs should be emitted for successful loads")

	// Remove the file to force reload error
	os.Remove(tokenFile)

	// Advance time to trigger reload
	currentTime = currentTime.Add(15 * time.Millisecond)

	// Call should log using zap logger
	tokenFn()
	assert.Equal(t, 1, logs.Len(), "Error should be logged using zap")

	logEntry := logs.All()[0]
	assert.Equal(t, "Token reload failed", logEntry.Message, "Log message should match")
	assert.NotNil(t, logEntry.Context[0].Interface, "Error should be attached to log")
}

// TestCachedFileTokenLoader_FilePermissions tests file permission errors
func TestCachedFileTokenLoader_FilePermissions(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Running as root - file permission tests not meaningful")
	}

	currentTime := time.Unix(0, 0)
	timeFn := func() time.Time { return currentTime }

	// Create a file and make it unreadable
	tokenFile := createTempTokenFile(t, "permission-test\n")
	err := os.Chmod(tokenFile, 0o000) // No permissions
	require.NoError(t, err)

	// Cleanup with restored permissions
	t.Cleanup(func() {
		os.Chmod(tokenFile, 0o644)
		os.Remove(tokenFile)
	})

	loader := cachedFileTokenLoader(tokenFile, 100*time.Millisecond, timeFn)

	// Should return permission error
	_, err = loader()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestTokenProvider_Wrapper(t *testing.T) {
	tokenFile := createTempTokenFile(t, "production-test-token\n")
	logger := zap.NewNop()

	// Test that TokenProvider uses real time
	tokenFn, err := TokenProvider(tokenFile, 100*time.Millisecond, logger)
	require.NoError(t, err)

	token := tokenFn()
	assert.Equal(t, "production-test-token", token)
}

// Helper functions

// createTempTokenFile creates a temp file with the given content and returns its name
func createTempTokenFile(t *testing.T, content string) string {
	t.Helper()

	tmpFile, err := os.CreateTemp(t.TempDir(), "token-*.txt")
	require.NoError(t, err, "Failed to create temp file")

	_, err = tmpFile.WriteString(content)
	require.NoError(t, err, "Failed to write to temp file")

	err = tmpFile.Close()
	require.NoError(t, err, "Failed to close temp file")

	// Cleanup happens automatically with t.TempDir()
	return tmpFile.Name()
}

// updateTokenFile updates an existing token file with new content
func updateTokenFile(t *testing.T, filename, content string) {
	t.Helper()

	err := os.WriteFile(filename, []byte(content), 0o600)
	require.NoError(t, err, "Failed to update token file")
}
