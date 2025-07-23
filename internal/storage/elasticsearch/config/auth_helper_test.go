// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
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
