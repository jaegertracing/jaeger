// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestInitAuthVars_PriorityAndLogging(t *testing.T) {
	// Setup zap observer
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	tempDir := t.TempDir()
	apiKeyFile := filepath.Join(tempDir, "api-key")
	bearerFile := filepath.Join(tempDir, "bearer-token")
	require.NoError(t, os.WriteFile(apiKeyFile, []byte("api-file-key"), 0o600))
	require.NoError(t, os.WriteFile(bearerFile, []byte("bearer-file-token"), 0o600))

	tests := []struct {
		name           string
		apiKey         *APIKeyAuthentication
		bearer         *BearerTokenAuthentication
		expectedScheme string
		expectedLog    string
	}{
		{
			name: "Only APIKey file",
			apiKey: &APIKeyAuthentication{
				FilePath: apiKeyFile,
			},
			bearer:         nil,
			expectedScheme: "ApiKey",
			// No log expected
		},
		{
			name:   "Only Bearer file",
			apiKey: nil,
			bearer: &BearerTokenAuthentication{
				FilePath: bearerFile,
			},
			expectedScheme: "Bearer",
			// No log expected
		},
		{
			name: "APIKey context and file",
			apiKey: &APIKeyAuthentication{
				FilePath:         apiKeyFile,
				AllowFromContext: true,
			},
			bearer:         nil,
			expectedScheme: "ApiKey",
			expectedLog:    "Both API key file and context propagation are enabled",
		},
		{
			name:   "Bearer context and file",
			apiKey: nil,
			bearer: &BearerTokenAuthentication{
				FilePath:         bearerFile,
				AllowFromContext: true,
			},
			expectedScheme: "Bearer",
			expectedLog:    "Both Bearer Token file and context propagation are enabled",
		},
		{
			name: "Both APIKey and Bearer set",
			apiKey: &APIKeyAuthentication{
				FilePath:         apiKeyFile,
				AllowFromContext: true,
			},
			bearer: &BearerTokenAuthentication{
				FilePath:         bearerFile,
				AllowFromContext: true,
			},
			expectedScheme: "ApiKey",
			expectedLog:    "Both API Key and Bearer Token authentication are configured. Priority order: (1) API Key will be used if available",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logs.TakeAll() // Clear previous logs
			authVars, err := initAuthVars(tc.apiKey, tc.bearer, logger)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedScheme, authVars.Scheme)
			if tc.expectedScheme == "ApiKey" && tc.apiKey != nil && tc.apiKey.FilePath != "" {
				assert.Equal(t, "api-file-key", authVars.TokenFn())
			}
			if tc.expectedScheme == "Bearer" && tc.bearer != nil && tc.bearer.FilePath != "" {
				assert.Equal(t, "bearer-file-token", authVars.TokenFn())
			}
			if tc.expectedLog != "" {
				found := false
				for _, entry := range logs.All() {
					if strings.Contains(entry.Message, tc.expectedLog) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected log message not found: %s", tc.expectedLog)
			}
		})
	}
}

// TestInitAuthVars_ReloadInterval verifies that the ReloadInterval is properly handled
// when initializing authentication variables.
func TestInitAuthVars_ReloadInterval(t *testing.T) {
	// Setup zap observer
	core, _ := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	tempDir := t.TempDir()
	apiKeyFile := filepath.Join(tempDir, "api-key")
	require.NoError(t, os.WriteFile(apiKeyFile, []byte("api-file-key"), 0o600))

	tests := []struct {
		name           string
		auth           any // *APIKeyAuthentication or *BearerTokenAuthentication
		expectedScheme string
	}{
		{
			name: "APIKey with ReloadInterval",
			auth: &APIKeyAuthentication{
				FilePath:         apiKeyFile,
				AllowFromContext: false,
				ReloadInterval:   configoptional.Some(5 * time.Second),
			},
			expectedScheme: "ApiKey",
		},
		{
			name: "Bearer with ReloadInterval",
			auth: &BearerTokenAuthentication{
				FilePath:         apiKeyFile, // Reusing apiKeyFile since we just need a valid file
				AllowFromContext: false,
				ReloadInterval:   configoptional.Some(10 * time.Second),
			},
			expectedScheme: "Bearer",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			switch auth := tc.auth.(type) {
			case *APIKeyAuthentication:
				authVars, err := initAuthVars(auth, nil, logger)
				require.NoError(t, err)
				assert.Equal(t, tc.expectedScheme, authVars.Scheme)
			case *BearerTokenAuthentication:
				authVars, err := initAuthVars(nil, auth, logger)
				require.NoError(t, err)
				assert.Equal(t, tc.expectedScheme, authVars.Scheme)
			}
		})
	}
}
