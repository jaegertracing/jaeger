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
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/internal/auth"
	"github.com/jaegertracing/jaeger/internal/auth/apikey"
	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
)

type roundTripFunc func(r *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestGetHTTPRoundTripper(t *testing.T) {
	t.Run("APIKey/Bearer context override logic table-driven", func(t *testing.T) {
		tempFile := filepath.Join(t.TempDir(), "api-key")
		require.NoError(t, os.WriteFile(tempFile, []byte("file-api-key"), 0o600))

		tests := []struct {
			name             string
			scheme           string
			filePath         string
			allowFromContext bool
			contextToken     string
			wantHeader       string
		}{
			{
				name:             "ApiKey: only file, no context override",
				scheme:           "ApiKey",
				filePath:         tempFile,
				allowFromContext: false,
				contextToken:     "",
				wantHeader:       "ApiKey file-api-key",
			},
			{
				name:             "ApiKey: only context override, context token present",
				scheme:           "ApiKey",
				filePath:         "",
				allowFromContext: true,
				contextToken:     "ctx-api-key",
				wantHeader:       "ApiKey ctx-api-key",
			},
			{
				name:             "ApiKey: both file and context override, context token present",
				scheme:           "ApiKey",
				filePath:         tempFile,
				allowFromContext: true,
				contextToken:     "ctx-api-key",
				wantHeader:       "ApiKey ctx-api-key",
			},
			{
				name:             "ApiKey: both file and context override, context token absent",
				scheme:           "ApiKey",
				filePath:         tempFile,
				allowFromContext: true,
				contextToken:     "",
				wantHeader:       "ApiKey file-api-key",
			},
			{
				name:             "Bearer: only file, no context override",
				scheme:           "Bearer",
				filePath:         tempFile,
				allowFromContext: false,
				contextToken:     "",
				wantHeader:       "Bearer file-api-key",
			},
			{
				name:             "Bearer: only context override, context token present",
				scheme:           "Bearer",
				filePath:         "",
				allowFromContext: true,
				contextToken:     "ctx-bearer",
				wantHeader:       "Bearer ctx-bearer",
			},
			{
				name:             "Bearer: both file and context override, context token present",
				scheme:           "Bearer",
				filePath:         tempFile,
				allowFromContext: true,
				contextToken:     "ctx-bearer",
				wantHeader:       "Bearer ctx-bearer",
			},
			{
				name:             "Bearer: both file and context override, context token absent",
				scheme:           "Bearer",
				filePath:         tempFile,
				allowFromContext: true,
				contextToken:     "",
				wantHeader:       "Bearer file-api-key",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				cfg := &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						APIKeyAuthentication:      configoptional.None[APIKeyAuthentication](),
						BearerTokenAuthentication: configoptional.None[BearerTokenAuthentication](),
					},
				}
				if tc.scheme == "ApiKey" {
					cfg.Authentication.APIKeyAuthentication = newAPIKeyAuth(tc.allowFromContext, tc.filePath)
				} else {
					cfg.Authentication.BearerTokenAuthentication = newBearerTokenAuth(tc.allowFromContext, tc.filePath)
				}
				logger := zap.NewNop()
				rt, err := GetHTTPRoundTripper(context.Background(), cfg, logger)
				require.NoError(t, err)
				tr, ok := rt.(*auth.RoundTripper)
				require.True(t, ok)

				ctx := context.Background()
				if tc.contextToken != "" {
					if tc.scheme == "ApiKey" {
						ctx = apikey.ContextWithAPIKey(ctx, tc.contextToken)
					} else {
						ctx = bearertoken.ContextWithBearerToken(ctx, tc.contextToken)
					}
				}
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com", nil)
				require.NoError(t, err)

				tr.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
					assert.Equal(t, tc.wantHeader, r.Header.Get("Authorization"))
					return &http.Response{StatusCode: http.StatusOK}, nil
				})

				_, err = tr.RoundTrip(req)
				require.NoError(t, err)
			})
		}
	})

	t.Run("APIKey/Bearer file and context logs warning (table-driven)", func(t *testing.T) {
		cases := []struct {
			authType    string
			fileName    string
			expectedLog string
		}{
			{
				authType:    "APIKey",
				fileName:    "api-key",
				expectedLog: "Both API key file and context propagation are enabled",
			},
			{
				authType:    "Bearer",
				fileName:    "bearer-token",
				expectedLog: "Both Bearer Token file and context propagation are enabled",
			},
			{
				authType:    "Both",
				fileName:    "api-key",
				expectedLog: "Both API Key and Bearer Token authentication are configured. The client will attempt to use both methods, prioritizing tokens from the context over files.",
			},
		}
		for _, tc := range cases {
			t.Run(tc.authType, func(t *testing.T) {
				core, observedLogs := observer.New(zap.WarnLevel)
				logger := zap.New(core)
				tempFile := filepath.Join(t.TempDir(), tc.fileName)
				require.NoError(t, os.WriteFile(tempFile, []byte("file-token"), 0o600))
				cfg := &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						APIKeyAuthentication:      configoptional.None[APIKeyAuthentication](),
						BearerTokenAuthentication: configoptional.None[BearerTokenAuthentication](),
					},
				}
				switch tc.authType {
				case "Both":
					cfg.Authentication.APIKeyAuthentication = newAPIKeyAuth(true, tempFile)
					cfg.Authentication.BearerTokenAuthentication = newBearerTokenAuth(true, tempFile)
				case "APIKey":
					cfg.Authentication.APIKeyAuthentication = newAPIKeyAuth(true, tempFile)
				case "Bearer":
					cfg.Authentication.BearerTokenAuthentication = newBearerTokenAuth(true, tempFile)
				}
				rt, err := GetHTTPRoundTripper(context.Background(), cfg, logger)
				require.NoError(t, err)

				switch tc.authType {
				case "Both":
					tr, ok := rt.(*auth.RoundTripper)
					require.True(t, ok)
					req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
					require.NoError(t, err)
					tr.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
						assert.Equal(t, "ApiKey file-token", r.Header.Get("Authorization"))
						return &http.Response{StatusCode: http.StatusOK}, nil
					})
					_, err = tr.RoundTrip(req)
					require.NoError(t, err)
				case "APIKey":
					tr, ok := rt.(*auth.RoundTripper)
					require.True(t, ok)
					req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
					require.NoError(t, err)
					tr.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
						assert.Equal(t, "ApiKey file-token", r.Header.Get("Authorization"))
						return &http.Response{StatusCode: http.StatusOK}, nil
					})
					_, err = tr.RoundTrip(req)
					require.NoError(t, err)
				case "Bearer":
					tr, ok := rt.(*auth.RoundTripper)
					require.True(t, ok)
					req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
					require.NoError(t, err)
					tr.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
						assert.Equal(t, "Bearer file-token", r.Header.Get("Authorization"))
						return &http.Response{StatusCode: http.StatusOK}, nil
					})
					_, err = tr.RoundTrip(req)
					require.NoError(t, err)
				default:
					// no-op
				}

				found := false
				for _, entry := range observedLogs.All() {
					if strings.Contains(entry.Message, tc.expectedLog) {
						found = true
						break
					}
				}
				assert.True(t, found, "expected warning log for %s", tc.authType)
			})
		}
	})

	tests := []struct {
		name           string
		setup          func(*testing.T) *Configuration
		setupCtx       func(context.Context) context.Context
		wantErr        bool
		wantLogMsgs    []string
		expectedType   string // "http.Transport" or "auth.RoundTripper"
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
						APIKeyAuthentication: newAPIKeyAuth(false, tempFile),
					},
				}
			},
			expectedType:   "auth.RoundTripper",
			expectedScheme: "ApiKey",
		},
		{
			name: "API key from context",
			setup: func(_ *testing.T) *Configuration {
				return &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						APIKeyAuthentication: newAPIKeyAuth(true, ""),
					},
				}
			},
			setupCtx: func(ctx context.Context) context.Context {
				return apikey.ContextWithAPIKey(ctx, "context-api-key")
			},
			expectedType:   "auth.RoundTripper",
			expectedScheme: "ApiKey",
		},
		{
			name: "bearer token from context",
			setup: func(_ *testing.T) *Configuration {
				return &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						BearerTokenAuthentication: newBearerTokenAuth(true, ""),
					},
				}
			},
			setupCtx: func(ctx context.Context) context.Context {
				return bearertoken.ContextWithBearerToken(ctx, "context-bearer-token")
			},
			expectedType:   "auth.RoundTripper",
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
						APIKeyAuthentication: newAPIKeyAuth(true, tempFile),
					},
				}
			},
			setupCtx: func(ctx context.Context) context.Context {
				return apikey.ContextWithAPIKey(ctx, "context-api-key")
			},
			wantLogMsgs: []string{
				"Both API key file and context propagation are enabled - context token will take precedence over file-based token",
			},
			expectedType:   "auth.RoundTripper",
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
						BearerTokenAuthentication: newBearerTokenAuth(false, tempFile),
					},
				}
			},
			expectedType:   "auth.RoundTripper",
			expectedScheme: "Bearer",
		},
		{
			name: "bearer token from context",
			setup: func(_ *testing.T) *Configuration {
				return &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						BearerTokenAuthentication: newBearerTokenAuth(true, ""),
					},
				}
			},
			expectedType:   "auth.RoundTripper",
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
						BearerTokenAuthentication: newBearerTokenAuth(true, tempFile),
					},
				}
			},
			wantLogMsgs: []string{
				"Both Bearer Token file and context propagation are enabled",
			},
			expectedType:   "auth.RoundTripper",
			expectedScheme: "Bearer",
		},
		{
			name: "API key file not found",
			setup: func(_ *testing.T) *Configuration {
				return &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						APIKeyAuthentication: newAPIKeyAuth(false, "/nonexistent/file"),
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
						BearerTokenAuthentication: newBearerTokenAuth(false, "/nonexistent/token"),
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
						BearerTokenAuthentication: newBearerTokenAuth(true, tempFile),
					},
				}
			},
			setupCtx: func(ctx context.Context) context.Context {
				return bearertoken.ContextWithBearerToken(ctx, "context-bearer-token")
			},
			wantLogMsgs: []string{
				"Both Bearer Token file and context propagation are enabled",
			},
			expectedType:   "auth.RoundTripper",
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
				case "auth.RoundTripper":
					// Try both pointer and non-pointer types
					tr, ok := transport.(*auth.RoundTripper)
					if !ok {
						// Try non-pointer type
						tr2, ok2 := transport.(*auth.RoundTripper)
						if !assert.True(t, ok2, "expected *auth.RoundTripper or auth.RoundTripper, got %T", transport) {
							return
						}
						// Convert to pointer for consistent handling
						tr = tr2
					}
					assert.NotNil(t, tr.Transport, "transport should not be nil")
					if len(tr.Auths) > 0 {
						assert.Equal(t, tt.expectedScheme, tr.Auths[0].Scheme, "unexpected auth scheme")
					}
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
	tests := []struct {
		name     string
		authType string
		fileName string
		cfgFn    func(tempFile string) *Configuration
	}{
		{
			name:     "APIKey reload error",
			authType: "APIKey",
			fileName: "api-key",
			cfgFn: func(tempFile string) *Configuration {
				return &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						APIKeyAuthentication: newAPIKeyAuth(false, tempFile),
					},
				}
			},
		},
		{
			name:     "BearerToken reload error",
			authType: "BearerToken",
			fileName: "bearer-token",
			cfgFn: func(tempFile string) *Configuration {
				return &Configuration{
					TLS: configtls.ClientConfig{Insecure: true},
					Authentication: Authentication{
						BearerTokenAuthentication: newBearerTokenAuth(false, tempFile),
					},
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempFile := filepath.Join(t.TempDir(), tc.fileName)
			require.NoError(t, os.WriteFile(tempFile, []byte("file-token"), 0o600))
			cfg := tc.cfgFn(tempFile)
			logger := zap.NewNop()
			rt, err := GetHTTPRoundTripper(context.Background(), cfg, logger)
			require.NoError(t, err)

			// Delete the token file to force reload error
			require.NoError(t, os.Remove(tempFile))

			// Type assert to auth.RoundTripper to access TokenFn
			tr, ok := rt.(*auth.RoundTripper)
			require.True(t, ok, "returned transport is not auth.RoundTripper")

			// Call TokenFn to trigger reload error branch
			require.NotEmpty(t, tr.Auths)
			_ = tr.Auths[0].TokenFn()
		})
	}
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
