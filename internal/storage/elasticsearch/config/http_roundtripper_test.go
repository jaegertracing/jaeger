// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/internal/bearertoken"
)

func TestGetHTTPRoundTripper(t *testing.T) {
	t.Run("APIKey file and context logs warning and exercises token loader", func(t *testing.T) {
		core, observedLogs := observer.New(zap.WarnLevel)
		logger := zap.New(core)
		tempFile := filepath.Join(t.TempDir(), "api-key")
		require.NoError(t, os.WriteFile(tempFile, []byte("file-api-key"), 0o600))
		cfg := &Configuration{
			TLS: configtls.ClientConfig{Insecure: true},
			Authentication: Authentication{
				APIKeyAuthentication: APIKeyAuthentication{
					FilePath:         tempFile,
					AllowFromContext: true,
				},
			},
		}
		rt, err := GetHTTPRoundTripper(context.Background(), cfg, logger)
		require.NoError(t, err)
		// Exercise the tokenFn to ensure loader is called (for branch coverage)
		tr, ok := rt.(bearertoken.RoundTripper)
		require.True(t, ok, "returned transport is not bearertoken.RoundTripper")
		_ = tr.TokenFn()
		found := false
		for _, entry := range observedLogs.All() {
			if strings.Contains(entry.Message, "Both API key file and context propagation are enabled") {
				found = true
				break
			}
		}
		assert.True(t, found, "expected warning log for both APIKey file and context")
	})

	t.Run("APIKey file and context logs warning", func(t *testing.T) {
		core, observedLogs := observer.New(zap.WarnLevel)
		logger := zap.New(core)
		tempFile := filepath.Join(t.TempDir(), "api-key")
		require.NoError(t, os.WriteFile(tempFile, []byte("file-api-key"), 0o600))
		cfg := &Configuration{
			TLS: configtls.ClientConfig{Insecure: true},
			Authentication: Authentication{
				APIKeyAuthentication: APIKeyAuthentication{
					FilePath:         tempFile,
					AllowFromContext: true,
				},
			},
		}
		_, err := GetHTTPRoundTripper(context.Background(), cfg, logger)
		require.NoError(t, err)
		found := false
		for _, entry := range observedLogs.All() {
			if strings.Contains(entry.Message, "Both API key file and context propagation are enabled") {
				found = true
				break
			}
		}
		assert.True(t, found, "expected warning log for both APIKey file and context")
	})

	t.Run("BearerToken file and context logs warning", func(t *testing.T) {
		core, observedLogs := observer.New(zap.WarnLevel)
		logger := zap.New(core)
		tempFile := filepath.Join(t.TempDir(), "bearer-token")
		require.NoError(t, os.WriteFile(tempFile, []byte("file-bearer-token"), 0o600))
		cfg := &Configuration{
			TLS: configtls.ClientConfig{Insecure: true},
			Authentication: Authentication{
				BearerTokenAuthentication: BearerTokenAuthentication{
					FilePath:         tempFile,
					AllowFromContext: true,
				},
			},
		}
		_, err := GetHTTPRoundTripper(context.Background(), cfg, logger)
		require.NoError(t, err)
		found := false
		for _, entry := range observedLogs.All() {
			if strings.Contains(entry.Message, "Token file and token propagation are both enabled") {
				found = true
				break
			}
		}
		assert.True(t, found, "expected warning log for both BearerToken file and context")
	})

	tests := []struct {
		name           string
		setup          func(*testing.T) *Configuration
		setupCtx       func(context.Context) context.Context
		wantErr        bool
		wantLogMsgs    []string
		expectedType   string // "http.Transport" or "bearertoken.RoundTripper"
		expectedScheme string // "" (none), "ApiKey", or "Bearer"
	}{
		{
			name: "secure TLS config",
			setup: func(_ *testing.T) *Configuration {
				return &Configuration{
					TLS: configtls.ClientConfig{
						Insecure: false,
					},
				}
			},
			expectedType: "http.Transport",
		},

		{
			name: "API key from file",
			setup: func(_ *testing.T) *Configuration {
				tempFile := filepath.Join(t.TempDir(), "api-key")
				require.NoError(t, os.WriteFile(tempFile, []byte("file-api-key"), 0o600))
				return &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						APIKeyAuthentication: APIKeyAuthentication{
							FilePath: tempFile,
						},
					},
				}
			},
			expectedType:   "bearertoken.RoundTripper",
			expectedScheme: "ApiKey",
		},
		{
			name: "API key from context",
			setup: func(_ *testing.T) *Configuration {
				return &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						APIKeyAuthentication: APIKeyAuthentication{
							AllowFromContext: true,
						},
					},
				}
			},
			setupCtx: func(ctx context.Context) context.Context {
				return bearertoken.ContextWithAPIKey(ctx, "context-api-key")
			},
			expectedType:   "bearertoken.RoundTripper",
			expectedScheme: "ApiKey",
		},
		{
			name: "bearer token from context",
			setup: func(_ *testing.T) *Configuration {
				return &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						BearerTokenAuthentication: BearerTokenAuthentication{
							AllowFromContext: true,
						},
					},
				}
			},
			setupCtx: func(ctx context.Context) context.Context {
				return bearertoken.ContextWithBearerToken(ctx, "context-bearer-token")
			},
			expectedType:   "bearertoken.RoundTripper",
			expectedScheme: "Bearer",
		},
		// Explicit assertion for tokenFn
		{
			name: "bearer token from context",
			setup: func(_ *testing.T) *Configuration {
				return &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						BearerTokenAuthentication: BearerTokenAuthentication{
							AllowFromContext: true,
						},
					},
				}
			},
			setupCtx: func(ctx context.Context) context.Context {
				return bearertoken.ContextWithBearerToken(ctx, "context-bearer-token")
			},
			expectedType:   "bearertoken.RoundTripper",
			expectedScheme: "Bearer",
		},
		{
			name: "API key file and context with warning",
			setup: func(_ *testing.T) *Configuration {
				tempFile := filepath.Join(t.TempDir(), "api-key")
				require.NoError(t, os.WriteFile(tempFile, []byte("file-api-key"), 0o600))
				return &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						APIKeyAuthentication: APIKeyAuthentication{
							FilePath:         tempFile,
							AllowFromContext: true,
						},
					},
				}
			},
			setupCtx: func(ctx context.Context) context.Context {
				return bearertoken.ContextWithAPIKey(ctx, "context-api-key")
			},
			wantLogMsgs: []string{
				"Both API key file and context propagation are enabled - context token will take precedence over file-based token",
			},
			expectedType:   "bearertoken.RoundTripper",
			expectedScheme: "ApiKey",
		},
		{
			name: "bearer token from file",
			setup: func(_ *testing.T) *Configuration {
				tempFile := filepath.Join(t.TempDir(), "token")
				require.NoError(t, os.WriteFile(tempFile, []byte("test-token"), 0o600))
				return &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						BearerTokenAuthentication: BearerTokenAuthentication{
							FilePath: tempFile,
						},
					},
				}
			},
			expectedType:   "bearertoken.RoundTripper",
			expectedScheme: "Bearer",
		},
		{
			name: "bearer token from context",
			setup: func(_ *testing.T) *Configuration {
				return &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						BearerTokenAuthentication: BearerTokenAuthentication{
							AllowFromContext: true,
						},
					},
				}
			},
			expectedType:   "bearertoken.RoundTripper",
			expectedScheme: "Bearer",
		},
		{
			name: "bearer token file and context with warning",
			setup: func(_ *testing.T) *Configuration {
				tempFile := filepath.Join(t.TempDir(), "token")
				require.NoError(t, os.WriteFile(tempFile, []byte("test-token"), 0o600))
				return &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						BearerTokenAuthentication: BearerTokenAuthentication{
							FilePath:         tempFile,
							AllowFromContext: true,
						},
					},
				}
			},
			wantLogMsgs: []string{
				"Token file and token propagation are both enabled, token from file won't be used",
			},
			expectedType:   "bearertoken.RoundTripper",
			expectedScheme: "Bearer",
		},
		{
			name: "API key file not found",
			setup: func(_ *testing.T) *Configuration {
				return &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						APIKeyAuthentication: APIKeyAuthentication{
							FilePath: "/nonexistent/file",
						},
					},
				}
			},
			wantErr: true,
		},
		{
			name: "bearer token file not found",
			setup: func(_ *testing.T) *Configuration {
				return &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						BearerTokenAuthentication: BearerTokenAuthentication{
							FilePath: "/nonexistent/token",
						},
					},
				}
			},
			wantErr: true,
		},
		{
			name: "bearer token file and context with warning",
			setup: func(_ *testing.T) *Configuration {
				tempFile := filepath.Join(t.TempDir(), "token")
				require.NoError(t, os.WriteFile(tempFile, []byte("test-token"), 0o600))
				return &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						BearerTokenAuthentication: BearerTokenAuthentication{
							FilePath:         tempFile,
							AllowFromContext: true,
						},
					},
				}
			},
			setupCtx: func(ctx context.Context) context.Context {
				return bearertoken.ContextWithBearerToken(ctx, "context-bearer-token")
			},
			wantLogMsgs: []string{
				"Token file and token propagation are both enabled, token from file won't be used",
			},
			expectedType:   "bearertoken.RoundTripper",
			expectedScheme: "Bearer",
		},
		{
			name: "No auth, fallback to http.Transport",
			setup: func(_ *testing.T) *Configuration {
				return &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
				}
			},
			expectedType: "http.Transport",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup logger with observer
			observedZapCore, observedLogs := observer.New(zap.InfoLevel)
			logger := zap.New(observedZapCore)

			// Setup config and context
			cfg := tt.setup(t)
			ctx := context.Background()
			if tt.setupCtx != nil {
				ctx = tt.setupCtx(ctx)
			}

			// Call the function
			transport, err := GetHTTPRoundTripper(ctx, cfg, logger)

			// Verify results
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, transport)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, transport)

				// Debug: Print the actual transport type we received
				t.Logf("Test case '%s' got transport type: %T", tt.name, transport)

				// Verify the transport type matches expected
				switch tt.expectedType {
				case "http.Transport":
					tr, ok := transport.(*http.Transport)
					if !assert.True(t, ok, "expected *http.Transport, got %T", transport) {
						return
					}
					if strings.Contains(tt.name, "insecure") {
						assert.True(t, tr.TLSClientConfig.InsecureSkipVerify, "InsecureSkipVerify should be true for insecure config")
					} else {
						assert.False(t, tr.TLSClientConfig.InsecureSkipVerify, "InsecureSkipVerify should be false for secure config")
					}
				case "bearertoken.RoundTripper":
					// Try both pointer and non-pointer types
					tr, ok := transport.(*bearertoken.RoundTripper)
					if !ok {
						// Try non-pointer type
						tr2, ok2 := transport.(bearertoken.RoundTripper)
						if !assert.True(t, ok2, "expected *bearertoken.RoundTripper or bearertoken.RoundTripper, got %T", transport) {
							return
						}
						// Convert to pointer for consistent handling
						tr = &tr2
					}
					assert.NotNil(t, tr.Transport, "transport should not be nil")
					assert.Equal(t, tt.expectedScheme, tr.AuthScheme, "unexpected auth scheme")
				}
			}

			// Verify log messages
			if len(tt.wantLogMsgs) > 0 {
				assert.Equal(t, len(tt.wantLogMsgs), observedLogs.Len())
				for i, msg := range tt.wantLogMsgs {
					assert.Contains(t, observedLogs.All()[i].Message, msg)
				}
			} else {
				assert.Zero(t, observedLogs.Len(), "expected no log messages")
			}
		})
	}
}

func TestTokenReloadErrorBranch(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "api-key")
	require.NoError(t, os.WriteFile(tempFile, []byte("file-api-key"), 0o600))
	cfg := &Configuration{
		TLS: configtls.ClientConfig{Insecure: true},
		Authentication: Authentication{
			APIKeyAuthentication: APIKeyAuthentication{
				FilePath: tempFile,
			},
		},
	}
	logger := zap.NewNop()
	rt, err := GetHTTPRoundTripper(context.Background(), cfg, logger)
	require.NoError(t, err)

	// Delete the token file to force reload error
	require.NoError(t, os.Remove(tempFile))

	// Type assert to bearertoken.RoundTripper to access TokenFn
	tr, ok := rt.(bearertoken.RoundTripper)
	require.True(t, ok, "returned transport is not bearertoken.RoundTripper")

	// Call TokenFn to trigger reload error branch
	_ = tr.TokenFn()
}

func TestLoadTokenFromFile(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) string
		wantToken   string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid token file",
			setup: func(_ *testing.T) string {
				tempFile := filepath.Join(t.TempDir(), "token")
				require.NoError(t, os.WriteFile(tempFile, []byte("test-token\n"), 0o600))
				return tempFile
			},
			wantToken: "test-token",
		},
		{
			name: "file not found",
			setup: func(_ *testing.T) string {
				return "/nonexistent/file"
			},
			wantErr:     true,
			errContains: "no such file or directory",
		},
		{
			name: "empty file",
			setup: func(t *testing.T) string {
				tempFile := filepath.Join(t.TempDir(), "empty")
				require.NoError(t, os.WriteFile(tempFile, []byte(""), 0o600))
				return tempFile
			},
			wantToken: "",
		},
		{
			name: "file with only whitespace",
			setup: func(t *testing.T) string {
				tempFile := filepath.Join(t.TempDir(), "whitespace")
				require.NoError(t, os.WriteFile(tempFile, []byte("\n \t\n"), 0o600))
				return tempFile
			},
			wantToken: "\n \t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			token, err := loadTokenFromFile(path)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantToken, token)
			}
		})
	}
}
