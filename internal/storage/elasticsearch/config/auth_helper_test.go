// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/internal/auth"
)

// TestInitBearerAuth tests bearer token authentication initialization
func TestInitBearerAuth(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()
	bearerFile := filepath.Join(tempDir, "bearer-token")
	require.NoError(t, os.WriteFile(bearerFile, []byte("test-bearer"), 0o600))

	tests := []struct {
		name        string
		bearerAuth  *BearerTokenAuthentication
		expectError bool
		expectNil   bool
		validate    func(t *testing.T, method *auth.Method)
	}{
		{
			name: "Valid file-based bearer auth",
			bearerAuth: &BearerTokenAuthentication{
				TokenAuthBase: TokenAuthBase{
					FilePath: bearerFile,
				},
			},
			expectError: false,
			expectNil:   false,
			validate: func(t *testing.T, method *auth.Method) {
				require.NotNil(t, method)
				assert.Equal(t, "Bearer", method.Scheme)
				assert.NotNil(t, method.TokenFn)
				assert.Equal(t, "test-bearer", method.TokenFn())
				assert.Nil(t, method.FromCtx)
			},
		},
		{
			name: "Valid context-based bearer auth",
			bearerAuth: &BearerTokenAuthentication{
				TokenAuthBase: TokenAuthBase{
					AllowFromContext: true,
				},
			},
			expectError: false,
			expectNil:   false,
			validate: func(t *testing.T, method *auth.Method) {
				require.NotNil(t, method)
				assert.Equal(t, "Bearer", method.Scheme)
				assert.Nil(t, method.TokenFn)
				assert.NotNil(t, method.FromCtx)
			},
		},
		{
			name:       "Nil bearer auth returns nil",
			bearerAuth: nil,
			expectNil:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			method, err := initBearerAuth(tc.bearerAuth, logger)
			switch {
			case tc.expectError:
				require.Error(t, err)
				assert.Nil(t, method)
			case tc.expectNil:
				require.NoError(t, err)
				assert.Nil(t, method)
			default:
				require.NoError(t, err)
				tc.validate(t, method)
			}
		})
	}
}

// TestInitAPIKeyAuth tests API key authentication initialization
func TestInitAPIKeyAuth(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()
	apiKeyFile := filepath.Join(tempDir, "api-key")
	require.NoError(t, os.WriteFile(apiKeyFile, []byte("test-apikey"), 0o600))

	tests := []struct {
		name        string
		apiKeyAuth  *APIKeyAuthentication
		expectError bool
		expectNil   bool
		validate    func(t *testing.T, method *auth.Method)
	}{
		{
			name: "Valid file-based API key auth",
			apiKeyAuth: &APIKeyAuthentication{
				TokenAuthBase: TokenAuthBase{
					FilePath: apiKeyFile,
				},
			},
			validate: func(t *testing.T, method *auth.Method) {
				require.NotNil(t, method)
				assert.Equal(t, "APIKey", method.Scheme)
				assert.NotNil(t, method.TokenFn)
				assert.Equal(t, "test-apikey", method.TokenFn())
				assert.Nil(t, method.FromCtx)
			},
		},
		{
			name: "Valid context-based API key auth",
			apiKeyAuth: &APIKeyAuthentication{
				TokenAuthBase: TokenAuthBase{
					AllowFromContext: true,
				},
			},
			validate: func(t *testing.T, method *auth.Method) {
				require.NotNil(t, method)
				assert.Equal(t, "APIKey", method.Scheme)
				assert.Nil(t, method.TokenFn)
				assert.NotNil(t, method.FromCtx)
			},
		},
		{
			name:       "Nil API key auth returns nil",
			apiKeyAuth: nil,
			expectNil:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			method, err := initAPIKeyAuth(tc.apiKeyAuth, logger)
			switch {
			case tc.expectError:
				require.Error(t, err)
				assert.Nil(t, method)
			case tc.expectNil:
				require.NoError(t, err)
				assert.Nil(t, method)
			default:
				require.NoError(t, err)
				tc.validate(t, method)
			}
		})
	}
}

// Test multiple auth types working together
func TestMultipleTokenAuth(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()

	bearerFile := filepath.Join(tempDir, "bearer")
	apiKeyFile := filepath.Join(tempDir, "apikey")
	require.NoError(t, os.WriteFile(bearerFile, []byte("bearer-token"), 0o600))
	require.NoError(t, os.WriteFile(apiKeyFile, []byte("api-key-token"), 0o600))

	bearerAuth := &BearerTokenAuthentication{
		TokenAuthBase: TokenAuthBase{FilePath: bearerFile},
	}
	apiKeyAuth := &APIKeyAuthentication{
		TokenAuthBase: TokenAuthBase{FilePath: apiKeyFile},
	}

	bearerMethod, err := initBearerAuth(bearerAuth, logger)
	require.NoError(t, err)
	require.NotNil(t, bearerMethod)

	apiKeyMethod, err := initAPIKeyAuth(apiKeyAuth, logger)
	require.NoError(t, err)
	require.NotNil(t, apiKeyMethod)

	assert.Equal(t, "Bearer", bearerMethod.Scheme)
	assert.Equal(t, "APIKey", apiKeyMethod.Scheme)
	assert.Equal(t, "bearer-token", bearerMethod.TokenFn())
	assert.Equal(t, "api-key-token", apiKeyMethod.TokenFn())
}

// TestInitBasicAuth tests basic authentication initialization
func TestInitBasicAuth(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()
	passwordFile := filepath.Join(tempDir, "password")
	require.NoError(t, os.WriteFile(passwordFile, []byte("testpass"), 0o600))

	tests := []struct {
		name        string
		basicAuth   *BasicAuthentication
		expectError bool
		expectNil   bool
		validate    func(t *testing.T, method *auth.Method)
	}{
		{
			name: "Static password basic auth",
			basicAuth: &BasicAuthentication{
				Username: "user",
				Password: "pass",
			},
			expectNil: false,
			validate: func(t *testing.T, method *auth.Method) {
				assert.Equal(t, "Basic", method.Scheme)
				assert.NotNil(t, method.TokenFn)
				// Verify base64 encoded "user:pass"
				expected := base64.StdEncoding.EncodeToString([]byte("user:pass"))
				assert.Equal(t, expected, method.TokenFn())
			},
		},
		{
			name: "File-based password basic auth",
			basicAuth: &BasicAuthentication{
				Username:         "user",
				PasswordFilePath: passwordFile,
			},
			expectNil: false,
			validate: func(t *testing.T, method *auth.Method) {
				assert.Equal(t, "Basic", method.Scheme)
				assert.NotNil(t, method.TokenFn)
				// Verify base64 encoded "user:testpass"
				expected := base64.StdEncoding.EncodeToString([]byte("user:testpass"))
				assert.Equal(t, expected, method.TokenFn())
			},
		},
		{
			name: "No username returns nil",
			basicAuth: &BasicAuthentication{
				Password: "pass",
			},
			expectNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			method, err := initBasicAuth(tc.basicAuth, logger)
			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tc.expectNil {
					assert.Nil(t, method)
				} else {
					tc.validate(t, method)
				}
			}
		})
	}
}

// TestInitBasicAuthWithReload tests password file reloading
func TestInitBasicAuthWithReload(t *testing.T) {
	currentTime := time.Unix(0, 0)
	timeFn := func() time.Time { return currentTime }

	tempDir := t.TempDir()
	passwordFile := filepath.Join(tempDir, "password")
	require.NoError(t, os.WriteFile(passwordFile, []byte("initial"), 0o600))

	logger := zap.NewNop()
	basicAuth := &BasicAuthentication{
		Username:         "user",
		PasswordFilePath: passwordFile,
		ReloadInterval:   50 * time.Millisecond,
	}

	method, err := initBasicAuthWithTime(basicAuth, logger, timeFn)
	require.NoError(t, err)
	require.NotNil(t, method)

	// Initial token
	initialExpected := base64.StdEncoding.EncodeToString([]byte("user:initial"))
	assert.Equal(t, initialExpected, method.TokenFn())

	// Update password file
	require.NoError(t, os.WriteFile(passwordFile, []byte("updated"), 0o600))

	// Before reload interval - should return cached
	currentTime = currentTime.Add(25 * time.Millisecond)
	assert.Equal(t, initialExpected, method.TokenFn())

	// After reload interval - should return updated
	currentTime = currentTime.Add(50 * time.Millisecond)
	updatedExpected := base64.StdEncoding.EncodeToString([]byte("user:updated"))
	assert.Equal(t, updatedExpected, method.TokenFn())
}

func TestInitBasicAuth_EdgeCases(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name        string
		basicAuth   *BasicAuthentication
		expectError bool
		expectNil   bool
		errorMsg    string
	}{
		{
			name:      "nil basicAuth returns nil",
			basicAuth: nil,
			expectNil: true,
		},
		{
			name: "both password and file path set - validation error",
			basicAuth: &BasicAuthentication{
				Username:         "user",
				Password:         "pass",
				PasswordFilePath: "/some/path",
			},
			expectError: true,
			errorMsg:    "both Password and PasswordFilePath are set",
		},
		{
			name: "empty username returns nil",
			basicAuth: &BasicAuthentication{
				Username: "",
				Password: "pass",
			},
			expectNil: true,
		},
		{
			name: "file path error",
			basicAuth: &BasicAuthentication{
				Username:         "user",
				PasswordFilePath: "/nonexistent/path",
			},
			expectError: true,
			errorMsg:    "failed to load password from file",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			method, err := initBasicAuth(tc.basicAuth, logger)
			switch {
			case tc.expectError:
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMsg)
				assert.Nil(t, method)
			case tc.expectNil:
				require.NoError(t, err)
				assert.Nil(t, method)
			default:
				require.NoError(t, err)
				require.NotNil(t, method)
			}
		})
	}
}

// Test warning logs for conflicting configuration
func TestTokenAuthBase_WarningLogs(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)
	tempDir := t.TempDir()

	tokenFile := filepath.Join(tempDir, "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("test-token"), 0o600))

	base := &TokenAuthBase{
		FilePath:         tokenFile,
		AllowFromContext: true,
	}

	method, err := initTokenAuthBaseWithTime(base, "Bearer", logger, time.Now)
	require.NoError(t, err)
	require.NotNil(t, method)

	// Check that warning was logged
	require.Equal(t, 1, logs.Len())
	logEntry := logs.All()[0]
	assert.Equal(t, zap.WarnLevel, logEntry.Level)
	assert.Contains(t, logEntry.Message, "Both token file and context propagation are enabled")
	assert.Equal(t, "Bearer", logEntry.Context[0].String)
}
