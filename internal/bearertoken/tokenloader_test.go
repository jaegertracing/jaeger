// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import (
	"bytes"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestCachedFileTokenLoader_Basic covers basic cache and reload logic for the token loader.
func TestCachedFileTokenLoader_Basic(t *testing.T) {
	tmpfile, err := os.CreateTemp(t.TempDir(), "tokenfile-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	token := "my-secret-token"
	if _, err := tmpfile.WriteString(token + "\n"); err != nil {
		t.Fatalf("failed to write token: %v", err)
	}
	tmpfile.Close()

	loader := CachedFileTokenLoader(tmpfile.Name(), 100*time.Millisecond)

	// First load - should read from file
	t1, err := loader()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if t1 != token {
		t.Errorf("expected %q, got %q", token, t1)
	}

	// Change the file, but within cache interval
	os.WriteFile(tmpfile.Name(), []byte("new-token\n"), 0o644)
	t2, err := loader()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if t2 != token {
		t.Errorf("expected cached %q, got %q", token, t2)
	}

	// Wait for cache to expire
	time.Sleep(120 * time.Millisecond)
	t3, err := loader()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if t3 != "new-token" {
		t.Errorf("expected refreshed token 'new-token', got %q", t3)
	}
}

// TestNewTokenProvider_InitialLoad covers initial load, fail-fast on missing/empty file, and correct token return.
func TestNewTokenProvider_InitialLoad(t *testing.T) {
	// Create a temp token file
	tokenFile := createTempTokenFile(t, "initial-token\n")
	defer os.Remove(tokenFile)

	// Test successful initial load
	tokenFn, err := TokenProvider(tokenFile, 100*time.Millisecond, nil)
	require.NoError(t, err, "TokenProvider should not fail with valid token file")
	assert.Equal(t, "initial-token", tokenFn(), "Token should match file contents")

	// Test fail-fast on invalid file
	_, err = TokenProvider("/nonexistent/file", 100*time.Millisecond, nil)
	require.Error(t, err, "TokenProvider should fail fast on missing file")
	assert.Contains(t, err.Error(), "failed to get token from file", "Error message should indicate token loading failure")

	// Test fail-fast on empty token
	emptyFile := createTempTokenFile(t, "\n")
	defer os.Remove(emptyFile)
	_, err = TokenProvider(emptyFile, 100*time.Millisecond, nil)
	require.Error(t, err, "TokenProvider should fail fast on empty token")
}

// TestNewTokenProvider_ReloadErrors ensures reload errors log and return cached token.
func TestNewTokenProvider_ReloadErrors(t *testing.T) {
	// Create a temp token file
	tokenFile := createTempTokenFile(t, "initial-token\n")
	defer os.Remove(tokenFile)

	// Capture log output
	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	defer log.SetOutput(os.Stderr)

	// Initialize token provider
	tokenFn, err := TokenProvider(tokenFile, 10*time.Millisecond, nil)
	require.NoError(t, err)

	// Initial call should succeed
	token := tokenFn()
	assert.Equal(t, "initial-token", token)

	// Remove the file to force reload error
	os.Remove(tokenFile)

	// Wait for cache to expire
	time.Sleep(20 * time.Millisecond)

	// Call should return last cached token and log error
	token = tokenFn()
	assert.Equal(t, "initial-token", token, "Should return cached token even after file deletion")
	assert.Contains(t, logBuf.String(), "Token reload failed", "Error should be logged")
}

// TestNewTokenProvider_WithZapLogger ensures zap logger is used for reload errors.
func TestNewTokenProvider_WithZapLogger(t *testing.T) {
	// Create a temp token file
	tokenFile := createTempTokenFile(t, "initial-token\n")
	defer os.Remove(tokenFile)

	// Create an observed zap logger
	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)

	// Initialize token provider with structured logger
	tokenFn, err := TokenProvider(tokenFile, 10*time.Millisecond, logger)
	require.NoError(t, err)

	// Initial call should succeed
	token := tokenFn()
	assert.Equal(t, "initial-token", token)

	// No logs yet
	assert.Equal(t, 0, logs.Len(), "No logs should be emitted for successful loads")

	// Remove the file to force reload error
	os.Remove(tokenFile)

	// Wait for cache to expire
	time.Sleep(20 * time.Millisecond)

	// Call should log using zap logger
	tokenFn()
	assert.Equal(t, 1, logs.Len(), "Error should be logged using zap")
	logEntry := logs.All()[0]
	assert.Equal(t, "Token reload failed", logEntry.Message, "Log message should match")
	assert.NotNil(t, logEntry.Context[0].Interface, "Error should be attached to log")
}

// createTempTokenFile is a helper to create a temp file with the given content and returns its name.
func createTempTokenFile(t *testing.T, content string) string {
	tmpfile, err := os.CreateTemp(t.TempDir(), "tokenfile-*.txt")
	require.NoError(t, err, "Failed to create temp file")

	_, err = tmpfile.WriteString(content)
	require.NoError(t, err, "Failed to write to temp file")

	tmpfile.Close()
	return tmpfile.Name()
}
