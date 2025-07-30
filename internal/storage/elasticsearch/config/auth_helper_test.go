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

// Test the  initBearerAuth function that uses real time
func TestInitBearerAuth(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()
	bearerFile := filepath.Join(tempDir, "bearer-token")
	require.NoError(t, os.WriteFile(bearerFile, []byte("test-token"), 0o600))

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
				FilePath: bearerFile,
			},
			expectError: false,
			expectNil:   false,
			validate: func(t *testing.T, method *auth.Method) {
				require.NotNil(t, method)
				assert.Equal(t, "Bearer", method.Scheme)
				assert.NotNil(t, method.TokenFn)
				assert.Equal(t, "test-token", method.TokenFn())
				assert.Nil(t, method.FromCtx)
			},
		},
		{
			name: "Valid context-based bearer auth",
			bearerAuth: &BearerTokenAuthentication{
				AllowFromContext: true,
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
			name: "Invalid bearer auth config returns nil",
			bearerAuth: &BearerTokenAuthentication{
				FilePath:         "",
				AllowFromContext: false,
			},
			expectError: false,
			expectNil:   true,
			validate: func(t *testing.T, method *auth.Method) {
				assert.Nil(t, method)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			method, err := initBearerAuth(tc.bearerAuth, logger)
			if tc.expectError {
				require.Error(t, err)
				assert.Nil(t, method)
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

// Test the time-injectable function for advanced scenarios
func TestInitBearerAuthWithTime(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()

	tests := []struct {
		name               string
		reloadInterval     time.Duration
		timeAdvance        time.Duration
		expectedFinalToken string
		description        string
	}{
		{
			name:               "Normal reloading behavior",
			reloadInterval:     50 * time.Millisecond,
			timeAdvance:        60 * time.Millisecond,
			expectedFinalToken: "updated-token",
			description:        "Should return updated token after cache expires",
		},
		{
			name:               "Zero interval disables reloading",
			reloadInterval:     0,
			timeAdvance:        24 * time.Minute,
			expectedFinalToken: "initial-token",
			description:        "Zero interval should completely disable token reloading",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tokenFile := filepath.Join(tempDir, "reloadable-token-"+tc.name)
			require.NoError(t, os.WriteFile(tokenFile, []byte("initial-token"), 0o600))

			currentTime := time.Unix(0, 0)
			timeFn := func() time.Time { return currentTime }

			bearer := &BearerTokenAuthentication{
				FilePath:       tokenFile,
				ReloadInterval: tc.reloadInterval,
			}

			// Initialize with mock time
			method, err := initBearerAuthWithTime(bearer, logger, timeFn)
			require.NoError(t, err)
			require.NotNil(t, method)
			require.NotNil(t, method.TokenFn)

			// Initial token
			token := method.TokenFn()
			assert.Equal(t, "initial-token", token)

			// Update the file
			err = os.WriteFile(tokenFile, []byte("updated-token"), 0o600)
			require.NoError(t, err)

			// Advance time and test
			currentTime = currentTime.Add(tc.timeAdvance)
			token = method.TokenFn()
			assert.Equal(t, tc.expectedFinalToken, token, tc.description)
		})
	}
}

// TestInitBearerAuth_FileErrors tests error handling for file operations in initBearerAuth
func TestInitBearerAuth_FileErrors(t *testing.T) {
	core, _ := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	tests := []struct {
		name          string
		bearer        *BearerTokenAuthentication
		expectedError bool
		expectNil     bool
		errorContains string
	}{
		{
			name: "Non-existent file path - returns error",
			bearer: &BearerTokenAuthentication{
				FilePath:         "/non/existent/path/token.txt",
				AllowFromContext: false,
			},
			expectedError: true,
			expectNil:     true,
			errorContains: "failed to get token from file",
		},
		{
			name: "Non-existent file with context enabled - still returns error",
			bearer: &BearerTokenAuthentication{
				FilePath:         "/non/existent/path/token.txt",
				AllowFromContext: true,
				ReloadInterval:   5 * time.Second,
			},
			expectedError: true,
			expectNil:     true,
			errorContains: "failed to get token from file",
		},
		{
			name: "Empty file path with context disabled - returns nil",
			bearer: &BearerTokenAuthentication{
				FilePath:         "",
				AllowFromContext: false,
			},
			expectedError: false,
			expectNil:     true,
		},
		{
			name: "Directory instead of file - returns error",
			bearer: &BearerTokenAuthentication{
				FilePath:         "/tmp",
				AllowFromContext: false,
			},
			expectedError: true,
			expectNil:     true,
			errorContains: "failed to get token from file",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			method, err := initBearerAuth(tc.bearer, logger)
			if tc.expectedError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
				assert.Nil(t, method)
			} else {
				require.NoError(t, err)
				if tc.expectNil {
					assert.Nil(t, method)
				} else {
					assert.NotNil(t, method)
				}
			}
		})
	}
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

func TestInitBearerAuth_EdgeCases(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name        string
		bearerAuth  *BearerTokenAuthentication
		expectError bool
		expectNil   bool
	}{
		{
			name:       "nil bearerAuth returns nil",
			bearerAuth: nil,
			expectNil:  true,
		},
		{
			name: "empty config returns nil",
			bearerAuth: &BearerTokenAuthentication{
				FilePath:         "",
				AllowFromContext: false,
			},
			expectNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			method, err := initBearerAuth(tc.bearerAuth, logger)
			switch {
			case tc.expectError:
				require.Error(t, err)
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

// TestInitAPIKeyAuth tests API key initialization
func TestInitAPIKeyAuth(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()
	apiKeyFile := filepath.Join(tempDir, "api-key")
	require.NoError(t, os.WriteFile(apiKeyFile, []byte("test-api-key"), 0o600))

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
				FilePath: apiKeyFile,
			},
			expectError: false,
			expectNil:   false,
			validate: func(t *testing.T, method *auth.Method) {
				require.NotNil(t, method)
				assert.Equal(t, "APIKey", method.Scheme)
				assert.NotNil(t, method.TokenFn)
				assert.Equal(t, "test-api-key", method.TokenFn())
				assert.Nil(t, method.FromCtx)
			},
		},
		{
			name: "Valid context-based API key auth",
			apiKeyAuth: &APIKeyAuthentication{
				AllowFromContext: true,
			},
			expectError: false,
			expectNil:   false,
			validate: func(t *testing.T, method *auth.Method) {
				require.NotNil(t, method)
				assert.Equal(t, "APIKey", method.Scheme)
				assert.Nil(t, method.TokenFn)
				assert.NotNil(t, method.FromCtx)
			},
		},
		{
			name: "Invalid API key config returns nil",
			apiKeyAuth: &APIKeyAuthentication{
				FilePath:         "",
				AllowFromContext: false,
			},
			expectError: false,
			expectNil:   true,
			validate: func(t *testing.T, method *auth.Method) {
				assert.Nil(t, method)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			method, err := initAPIKeyAuth(tc.apiKeyAuth, logger)
			if tc.expectError {
				require.Error(t, err)
				assert.Nil(t, method)
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

// Test the time-injectable function for advanced scenarios
func TestInitAPIKeyAuthWithTime(t *testing.T) {
	logger := zap.NewNop()
	tempDir := t.TempDir()

	tests := []struct {
		name               string
		reloadInterval     time.Duration
		timeAdvance        time.Duration
		expectedFinalToken string
		description        string
	}{
		{
			name:               "Normal reloading behavior",
			reloadInterval:     50 * time.Millisecond,
			timeAdvance:        60 * time.Millisecond,
			expectedFinalToken: "updated-api-key",
			description:        "Should return updated API key after cache expires",
		},
		{
			name:               "Zero interval disables reloading",
			reloadInterval:     0,
			timeAdvance:        24 * time.Minute,
			expectedFinalToken: "initial-api-key",
			description:        "Zero interval should completely disable API key reloading",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			apiKeyFile := filepath.Join(tempDir, "reloadable-api-key-"+tc.name)
			require.NoError(t, os.WriteFile(apiKeyFile, []byte("initial-api-key"), 0o600))

			currentTime := time.Unix(0, 0)
			timeFn := func() time.Time { return currentTime }

			apiKeyAuth := &APIKeyAuthentication{
				FilePath:       apiKeyFile,
				ReloadInterval: tc.reloadInterval,
			}

			// Initialize with mock time
			method, err := initAPIKeyAuthWithTime(apiKeyAuth, logger, timeFn)
			require.NoError(t, err)
			require.NotNil(t, method)
			require.NotNil(t, method.TokenFn)

			// Initial API key
			token := method.TokenFn()
			assert.Equal(t, "initial-api-key", token)

			// Update the file
			err = os.WriteFile(apiKeyFile, []byte("updated-api-key"), 0o600)
			require.NoError(t, err)

			// Advance time and test
			currentTime = currentTime.Add(tc.timeAdvance)
			token = method.TokenFn()
			assert.Equal(t, tc.expectedFinalToken, token, tc.description)
		})
	}
}

func TestInitAPIKeyAuth_EdgeCases(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name        string
		apiKeyAuth  *APIKeyAuthentication
		expectError bool
		expectNil   bool
	}{
		{
			name:       "nil apiKeyAuth returns nil",
			apiKeyAuth: nil,
			expectNil:  true,
		},
		{
			name: "file path error",
			apiKeyAuth: &APIKeyAuthentication{
				FilePath: "/nonexistent/path",
			},
			expectError: true,
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
				require.NotNil(t, method)
			}
		})
	}
}
