// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
				FilePath: configoptional.Some(apiKeyFile),
			},
			bearer:         nil,
			expectedScheme: "ApiKey",
			// No log expected
		},
		{
			name:           "Only Bearer file",
			apiKey:         nil,
			bearer:         &BearerTokenAuthentication{FilePath: configoptional.Some(bearerFile)},
			expectedScheme: "Bearer",
			// No log expected
		},
		{
			name: "APIKey context and file",
			apiKey: &APIKeyAuthentication{
				FilePath:         configoptional.Some(apiKeyFile),
				AllowFromContext: configoptional.Some(true),
			},
			bearer:         nil,
			expectedScheme: "ApiKey",
			expectedLog:    "Both API key file and context propagation are enabled",
		},
		{
			name:   "Bearer context and file",
			apiKey: nil,
			bearer: &BearerTokenAuthentication{
				FilePath:         configoptional.Some(bearerFile),
				AllowFromContext: configoptional.Some(true),
			},
			expectedScheme: "Bearer",
			expectedLog:    "Both Bearer Token file and context propagation are enabled",
		},
		{
			name: "Both APIKey and Bearer set",
			apiKey: &APIKeyAuthentication{
				FilePath:         configoptional.Some(apiKeyFile),
				AllowFromContext: configoptional.Some(true),
			},
			bearer: &BearerTokenAuthentication{
				FilePath:         configoptional.Some(bearerFile),
				AllowFromContext: configoptional.Some(true),
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
			if tc.expectedScheme == "ApiKey" && tc.apiKey != nil && tc.apiKey.FilePath.Get() != nil {
				assert.Equal(t, "api-file-key", authVars.TokenFn())
			}
			if tc.expectedScheme == "Bearer" && tc.bearer != nil && tc.bearer.FilePath.Get() != nil {
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
