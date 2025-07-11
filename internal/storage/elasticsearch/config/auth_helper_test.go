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

func TestInitAuthVars_MultiAuthAndLogging(t *testing.T) {
	// Setup zap observer
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	tempDir := t.TempDir()
	apiKeyFile := filepath.Join(tempDir, "api-key")
	bearerFile := filepath.Join(tempDir, "bearer-token")
	require.NoError(t, os.WriteFile(apiKeyFile, []byte("api-file-key"), 0o600))
	require.NoError(t, os.WriteFile(bearerFile, []byte("bearer-file-token"), 0o600))

	tests := []struct {
		name               string
		apiKey             *APIKeyAuthentication
		bearer             *BearerTokenAuthentication
		expectedNumConfigs int
		expectedSchemes    []string
		expectedLogs       []string
	}{
		{
			name: "Only APIKey file",
			apiKey: &APIKeyAuthentication{
				FilePath: apiKeyFile,
			},
			bearer:             nil,
			expectedNumConfigs: 1,
			expectedSchemes:    []string{"ApiKey"},
		},
		{
			name:   "Only Bearer file",
			apiKey: nil,
			bearer: &BearerTokenAuthentication{
				FilePath: bearerFile,
			},
			expectedNumConfigs: 1,
			expectedSchemes:    []string{"Bearer"},
		},
		{
			name: "APIKey context and file",
			apiKey: &APIKeyAuthentication{
				FilePath:         apiKeyFile,
				AllowFromContext: true,
			},
			bearer:             nil,
			expectedNumConfigs: 1,
			expectedSchemes:    []string{"ApiKey"},
			expectedLogs:       []string{"Both API key file and context propagation are enabled"},
		},
		{
			name:   "Bearer context and file",
			apiKey: nil,
			bearer: &BearerTokenAuthentication{
				FilePath:         bearerFile,
				AllowFromContext: true,
			},
			expectedNumConfigs: 1,
			expectedSchemes:    []string{"Bearer"},
			expectedLogs:       []string{"Both Bearer Token file and context propagation are enabled"},
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
			expectedNumConfigs: 2,
			expectedSchemes:    []string{"ApiKey", "Bearer"},
			expectedLogs: []string{
				"Both API Key and Bearer Token authentication are configured. The client will attempt to use both methods, prioritizing tokens from the context over files.",
				"Both API key file and context propagation are enabled",
				"Both Bearer Token file and context propagation are enabled",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logs.TakeAll() // Clear previous logs
			authVars, err := initAuthVars(tc.apiKey, tc.bearer, logger)
			require.NoError(t, err)

			require.Len(t, authVars.AuthConfigs, tc.expectedNumConfigs)

			var schemes []string
			for _, config := range authVars.AuthConfigs {
				schemes = append(schemes, config.Scheme)
				if config.Scheme == "ApiKey" {
					if tc.apiKey.FilePath != "" {
						assert.NotNil(t, config.TokenFn)
						assert.Equal(t, "api-file-key", config.TokenFn())
					}
					if tc.apiKey.AllowFromContext {
						assert.NotNil(t, config.FromCtx)
					}
				}
				if config.Scheme == "Bearer" {
					if tc.bearer.FilePath != "" {
						assert.NotNil(t, config.TokenFn)
						assert.Equal(t, "bearer-file-token", config.TokenFn())
					}
					if tc.bearer.AllowFromContext {
						assert.NotNil(t, config.FromCtx)
					}
				}
			}
			assert.ElementsMatch(t, tc.expectedSchemes, schemes)

			logEntries := logs.All()
			if len(tc.expectedLogs) > 0 {
				require.Len(t, logEntries, len(tc.expectedLogs), "Mismatched number of logs")
				actualLogs := make([]string, len(logEntries))
				for i, entry := range logEntries {
					actualLogs[i] = entry.Message
				}

				for _, expectedLog := range tc.expectedLogs {
					var found bool
					for _, actualLog := range actualLogs {
						if strings.Contains(actualLog, expectedLog) {
							found = true
							break
						}
					}
					assert.True(t, found, "expected log message not found: %s", expectedLog)
				}
			} else {
				assert.Empty(t, logEntries, "unexpected log messages")
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
	tokenFile := filepath.Join(tempDir, "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("some-token"), 0o600))

	tests := []struct {
		name           string
		auth           any // *APIKeyAuthentication or *BearerTokenAuthentication
		expectedScheme string
	}{
		{
			name: "APIKey with ReloadInterval",
			auth: &APIKeyAuthentication{
				FilePath:       tokenFile,
				ReloadInterval: configoptional.Some(5 * time.Second),
			},
			expectedScheme: "ApiKey",
		},
		{
			name: "Bearer with ReloadInterval",
			auth: &BearerTokenAuthentication{
				FilePath:       tokenFile,
				ReloadInterval: configoptional.Some(10 * time.Second),
			},
			expectedScheme: "Bearer",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var authVars AuthVars
			var err error

			switch auth := tc.auth.(type) {
			case *APIKeyAuthentication:
				authVars, err = initAuthVars(auth, nil, logger)
			case *BearerTokenAuthentication:
				authVars, err = initAuthVars(nil, auth, logger)
			default:
				t.Fatalf("Unknown auth type: %T", auth)
			}

			require.NoError(t, err)
			require.Len(t, authVars.AuthConfigs, 1)
			assert.Equal(t, tc.expectedScheme, authVars.AuthConfigs[0].Scheme)
			assert.NotNil(t, authVars.AuthConfigs[0].TokenFn)
		})
	}
}
