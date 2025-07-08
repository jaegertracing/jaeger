// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/auth/apikey"
	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

var mockEsServerResponseWithVersion0 = []byte(`
{
	"Version": {
		"Number": "0"
	}
}
`)

var mockEsServerResponseWithVersion1 = []byte(`
{
	"tagline": "OpenSearch",
	"Version": {
		"Number": "1"
	}
}
`)

var mockEsServerResponseWithVersion2 = []byte(`
{
	"tagline": "OpenSearch",
	"Version": {
		"Number": "2"
	}
}
`)

var mockEsServerResponseWithVersion8 = []byte(`
{
	"tagline": "OpenSearch",
	"Version": {
		"Number": "9"
	}
}
`)

func copyToTempFile(t *testing.T, pattern string, filename string) (file *os.File) {
	tempDir := t.TempDir()
	tempFilePath := tempDir + "/" + pattern
	tempFile, err := os.Create(tempFilePath)
	require.NoError(t, err)
	data, err := os.ReadFile(filename)

	require.NoError(t, err)

	_, err = tempFile.Write(data)
	require.NoError(t, err)
	require.NoError(t, tempFile.Close())

	return tempFile
}

func int64Ptr(v int64) *int64 {
	return &v
}

// Helper function to create a BasicAuthentication with optional fields
func newBasicAuth(username, password, passwordFilePath string) configoptional.Optional[BasicAuthentication] {
	return configoptional.Some(BasicAuthentication{
		Username:         username,
		Password:         password,
		PasswordFilePath: passwordFilePath,
	})
}

// Helper function to create a BearerTokenAuthentication with optional fields
func newBearerTokenAuth(allowFromContext bool, filePath string) configoptional.Optional[BearerTokenAuthentication] {
	auth := BearerTokenAuthentication{
		AllowFromContext: allowFromContext,
		FilePath:         filePath,
	}
	if !allowFromContext && filePath != "" {
		auth.ReloadInterval = configoptional.Some(0 * time.Second)
	}
	return configoptional.Some(auth)
}

// Helper function to create an APIKeyAuthentication with optional fields
func newAPIKeyAuth(allowFromContext bool, filePath string) configoptional.Optional[APIKeyAuthentication] {
	auth := APIKeyAuthentication{
		AllowFromContext: allowFromContext,
		FilePath:         filePath,
	}
	if !allowFromContext && filePath != "" {
		auth.ReloadInterval = configoptional.Some(0 * time.Second)
	}
	return configoptional.Some(auth)
}

func TestNewClient(t *testing.T) {
	const (
		pwd1       = "password"
		token      = "token"
		apiKey     = "api-key"
		serverCert = "../../../../internal/config/tlscfg/testdata/example-server-cert.pem"
	)
	pwdFile := filepath.Join(t.TempDir(), "pwd")
	require.NoError(t, os.WriteFile(pwdFile, []byte(pwd1), 0o600))
	pwdtokenFile := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(pwdtokenFile, []byte(token), 0o600))
	apiKeyFile := filepath.Join(t.TempDir(), "apikey")
	require.NoError(t, os.WriteFile(apiKeyFile, []byte(apiKey), 0o600))
	// copy certs to temp so we can modify them
	certFilePath := copyToTempFile(t, "cert.crt", serverCert)
	defer certFilePath.Close()

	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		// Accept both GET (for version check) and HEAD (for health check)
		assert.Contains(t, []string{http.MethodGet, http.MethodHead}, req.Method)
		res.WriteHeader(http.StatusOK)
		res.Write(mockEsServerResponseWithVersion0)
	}))
	defer testServer.Close()
	testServer1 := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		// Accept both GET (for version check) and HEAD (for health check)
		assert.Contains(t, []string{http.MethodGet, http.MethodHead}, req.Method)
		res.WriteHeader(http.StatusOK)
		res.Write(mockEsServerResponseWithVersion1)
	}))
	defer testServer1.Close()
	testServer2 := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		// Accept both GET (for version check) and HEAD (for health check)
		assert.Contains(t, []string{http.MethodGet, http.MethodHead}, req.Method)
		res.WriteHeader(http.StatusOK)
		res.Write(mockEsServerResponseWithVersion2)
	}))
	defer testServer2.Close()
	testServer8 := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		// Accept both GET (for version check) and HEAD (for health check)
		assert.Contains(t, []string{http.MethodGet, http.MethodHead}, req.Method)
		res.WriteHeader(http.StatusOK)
		res.Write(mockEsServerResponseWithVersion8)
	}))
	defer testServer8.Close()
	tests := []struct {
		name          string
		config        *Configuration
		expectedError bool
	}{
		{
			name: "success with valid configuration",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					BasicAuthentication: configoptional.Some(BasicAuthentication{
						Username:         "user",
						Password:         "secret",
						PasswordFilePath: "",
					}),
					BearerTokenAuthentication: configoptional.Some(BearerTokenAuthentication{
						AllowFromContext: true,
					}),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
				Version: 8,
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration and tls enabled",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					BasicAuthentication: configoptional.Some(BasicAuthentication{
						Username:         "user",
						Password:         "secret",
						PasswordFilePath: "",
					}),
					BearerTokenAuthentication: configoptional.Some(BearerTokenAuthentication{
						AllowFromContext: true,
					}),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
				Version: 0,
				TLS:     configtls.ClientConfig{Insecure: false},
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration and reading token and certicate from file",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					BasicAuthentication:       newBasicAuth("user", "secret", ""),
					BearerTokenAuthentication: newBearerTokenAuth(true, pwdtokenFile),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
				Version: 0,
				TLS: configtls.ClientConfig{
					Insecure: true,
					Config: configtls.Config{
						CAFile: certFilePath.Name(),
					},
				},
			},
			expectedError: false,
		},
		{
			name: "succes with invalid configuration of version higher than 8",
			config: &Configuration{
				Servers: []string{testServer8.URL},
				Authentication: Authentication{
					BasicAuthentication:       newBasicAuth("user", "secret", ""),
					BearerTokenAuthentication: newBearerTokenAuth(true, ""),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
				Version: 9,
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration with version 1",
			config: &Configuration{
				Servers: []string{testServer1.URL},
				Authentication: Authentication{
					BasicAuthentication:       newBasicAuth("user", "secret", ""),
					BearerTokenAuthentication: newBearerTokenAuth(true, ""),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration with version 2",
			config: &Configuration{
				Servers: []string{testServer2.URL},
				Authentication: Authentication{
					BasicAuthentication:       newBasicAuth("user", "secret", ""),
					BearerTokenAuthentication: newBearerTokenAuth(true, ""),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration password from file",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					BasicAuthentication:       newBasicAuth("user", "", pwdFile),
					BearerTokenAuthentication: newBearerTokenAuth(true, ""),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "fali with configuration password and password from file are set",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					BasicAuthentication:       newBasicAuth("user", "secret", pwdFile),
					BearerTokenAuthentication: newBearerTokenAuth(true, ""),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: true,
		},
		{
			name: "fail with missing server",
			config: &Configuration{
				Servers: []string{},
				Authentication: Authentication{
					BasicAuthentication:       newBasicAuth("user", "secret", ""),
					BearerTokenAuthentication: newBearerTokenAuth(true, ""),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: true,
		},
		{
			name: "fail with invalid configuration invalid loglevel",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					BasicAuthentication:       newBasicAuth("user", "secret", ""),
					BearerTokenAuthentication: newBearerTokenAuth(true, ""),
				},
				LogLevel: "invalid",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: true,
		},
		{
			name: "fail with invalid configuration invalid bulkworkers number",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					BasicAuthentication:       newBasicAuth("user", "secret", ""),
					BearerTokenAuthentication: newBearerTokenAuth(true, ""),
				},
				LogLevel: "invalid",
				BulkProcessing: BulkProcessing{
					Workers:  0,
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: true,
		},
		{
			name: "success with API key authentication",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					APIKeyAuthentication: newAPIKeyAuth(false, apiKeyFile),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "success with API key authentication from file",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					APIKeyAuthentication: newAPIKeyAuth(false, apiKeyFile),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "success with API key authentication from context",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					APIKeyAuthentication: newAPIKeyAuth(true, ""),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "fail with API key authentication from non-existent file",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					APIKeyAuthentication: newAPIKeyAuth(false, "non-existent-file"),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: true, // No error at config time, error will occur at request time
		},
		{
			name: "success with API key authentication from file with context override",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					APIKeyAuthentication: newAPIKeyAuth(true, apiKeyFile),
				},
				LogLevel: "debug",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration and info log level",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					BasicAuthentication:       newBasicAuth("user", "secret", ""),
					BearerTokenAuthentication: newBearerTokenAuth(true, ""),
				},
				LogLevel: "info",
				BulkProcessing: BulkProcessing{
					Workers:  0,
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
		{
			name: "success with valid configuration and error log level",
			config: &Configuration{
				Servers: []string{testServer.URL},
				Authentication: Authentication{
					BasicAuthentication:       newBasicAuth("user", "secret", ""),
					BearerTokenAuthentication: newBearerTokenAuth(true, ""),
				},
				LogLevel: "error",
				BulkProcessing: BulkProcessing{
					MaxBytes: -1, // disable bulk; we want immediate flush
				},
			},
			expectedError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger := zap.NewNop()
			metricsFactory := metrics.NullFactory
			config := test.config
			ctx := context.Background()
			if strings.Contains(test.name, "from context") {
				ctx = apikey.ContextWithAPIKey(ctx, "context-api-key")
			}
			client, err := NewClient(ctx, config, logger, metricsFactory)
			if test.expectedError {
				require.Error(t, err)
				require.Nil(t, client)
			} else {
				require.NoError(t, err)
				require.NotNil(t, client)
				err = client.Close()
				require.NoError(t, err)
			}
		})
	}
}

// compareIndexOptions is a helper function to compare two IndexOptions
func compareIndexOptions(t *testing.T, got, want *IndexOptions) {
	t.Helper()
	assert.Equal(t, want.Shards, got.Shards)
	assert.Equal(t, want.Replicas, got.Replicas)
	assert.Equal(t, want.Priority, got.Priority)
	assert.Equal(t, want.DateLayout, got.DateLayout)
	assert.Equal(t, want.RolloverFrequency, got.RolloverFrequency)
}

// compareConfigs is a helper function to compare two Configuration objects
func compareConfigs(t *testing.T, got, want *Configuration) {
	t.Helper()

	// Compare basic fields
	assert.Equal(t, want.RemoteReadClusters, got.RemoteReadClusters)
	assert.Equal(t, want.Sniffing, got.Sniffing)
	assert.Equal(t, want.MaxSpanAge, got.MaxSpanAge)
	assert.Equal(t, want.AdaptiveSamplingLookback, got.AdaptiveSamplingLookback)
	assert.Equal(t, want.Indices.IndexPrefix, got.Indices.IndexPrefix)
	assert.Equal(t, want.BulkProcessing, got.BulkProcessing)
	assert.Equal(t, want.Tags, got.Tags)
	assert.Equal(t, want.MaxDocCount, got.MaxDocCount)
	assert.Equal(t, want.LogLevel, got.LogLevel)
	assert.Equal(t, want.SendGetBodyAs, got.SendGetBodyAs)

	// Compare BasicAuthentication
	gotHasBasic := got.Authentication.BasicAuthentication.HasValue()
	wantHasBasic := want.Authentication.BasicAuthentication.HasValue()

	// If both don't have values, we're done
	if !wantHasBasic && !gotHasBasic {
		return
	}

	// If one has a value but not the other, fail
	if wantHasBasic != gotHasBasic {
		t.Errorf("BasicAuthentication presence mismatch: want %v, got %v", wantHasBasic, gotHasBasic)
		return
	}

	// Both have values, compare them
	gotBasic := got.Authentication.BasicAuthentication.Get()
	wantBasic := want.Authentication.BasicAuthentication.Get()

	// Compare username if present
	gotUsername := gotBasic.Username
	wantUsername := wantBasic.Username
	if wantUsername != "" {
		assert.Equal(t, wantUsername, gotUsername)
	} else {
		assert.Nil(t, gotUsername)
	}

	// Compare password if present
	gotPassword := gotBasic.Password
	wantPassword := wantBasic.Password
	if wantPassword != "" {
		assert.Equal(t, wantPassword, gotPassword)
	} else {
		assert.Nil(t, gotPassword)
	}

	// Compare IndexOptions
	compareIndexOptions(t, &got.Indices.Spans, &want.Indices.Spans)
	compareIndexOptions(t, &got.Indices.Services, &want.Indices.Services)
	compareIndexOptions(t, &got.Indices.Dependencies, &want.Indices.Dependencies)
}

func TestApplyDefaults(t *testing.T) {
	source := &Configuration{
		RemoteReadClusters: []string{"cluster1", "cluster2"},
		Authentication: Authentication{
			BasicAuthentication: newBasicAuth("sourceUser", "sourcePass", ""),
		},
		Sniffing: Sniffing{
			Enabled:  true,
			UseHTTPS: true,
		},
		MaxSpanAge:               100,
		AdaptiveSamplingLookback: 50,
		Indices: Indices{
			IndexPrefix: "hello",
			Spans: IndexOptions{
				Shards:   5,
				Replicas: int64Ptr(1),
				Priority: 10,
			},
			Services: IndexOptions{
				Shards:   5,
				Replicas: int64Ptr(1),
				Priority: 20,
			},
			Dependencies: IndexOptions{
				Shards:   5,
				Replicas: int64Ptr(1),
				Priority: 30,
			},
			Sampling: IndexOptions{},
		},
		BulkProcessing: BulkProcessing{
			MaxBytes:      1000,
			Workers:       10,
			MaxActions:    100,
			FlushInterval: 30,
		},
		Tags:          TagsAsFields{AllAsFields: true, DotReplacement: "dot", Include: "include", File: "file"},
		MaxDocCount:   10000,
		LogLevel:      "info",
		SendGetBodyAs: "json",
	}

	tests := []struct {
		name     string
		target   *Configuration
		expected *Configuration
	}{
		{
			name: "All Defaults Applied except PriorityDependenciesTemplate",
			target: &Configuration{
				Indices: Indices{
					Dependencies: IndexOptions{
						Priority: 30,
					},
				},
			}, // All fields are empty
			expected: source,
		},
		{
			name:     "All fields empty, all default",
			target:   &Configuration{},
			expected: source,
		},
		{
			name: "Fields already set, not defaulted",
			target: &Configuration{
				RemoteReadClusters: []string{"already"},
				LogLevel:           "already",
				HTTPCompression:    true,
			},
			expected: func() *Configuration {
				cfg := *source
				cfg.RemoteReadClusters = []string{"already"}
				cfg.LogLevel = "already"
				cfg.HTTPCompression = true
				return &cfg
			}(),
		},
		{
			name: "Partial fields, some defaulted",
			target: &Configuration{
				RemoteReadClusters: nil,
				LogLevel:           "",
				HTTPCompression:    false,
			},
			expected: source,
		},
		{
			name: "Some Defaults Applied",
			target: &Configuration{
				RemoteReadClusters: []string{"customCluster"},
				Authentication: Authentication{
					BasicAuthentication: newBasicAuth("customUser", "", ""),
				},
				Indices: Indices{
					Spans: IndexOptions{
						Priority: 10,
					},
					Services: IndexOptions{
						Priority: 20,
					},
					Dependencies: IndexOptions{
						Priority: 30,
					},
				},
				// Other fields left default
			},
			expected: &Configuration{
				RemoteReadClusters: []string{"customCluster"},
				Authentication: Authentication{
					BasicAuthentication: newBasicAuth("customUser", "sourcePass", ""),
				},
				Sniffing: Sniffing{
					Enabled:  true,
					UseHTTPS: true,
				},
				MaxSpanAge:               100,
				AdaptiveSamplingLookback: 50,
				Indices: Indices{
					IndexPrefix: "hello",
					Spans: IndexOptions{
						Shards:   5,
						Replicas: int64Ptr(1),
						Priority: 10,
					},
					Services: IndexOptions{
						Shards:   5,
						Replicas: int64Ptr(1),
						Priority: 20,
					},
					Dependencies: IndexOptions{
						Shards:   5,
						Replicas: int64Ptr(1),
						Priority: 30,
					},
				},
				BulkProcessing: BulkProcessing{
					MaxBytes:      1000,
					Workers:       10,
					MaxActions:    100,
					FlushInterval: 30,
				},
				Tags:          TagsAsFields{AllAsFields: true, DotReplacement: "dot", Include: "include", File: "file"},
				MaxDocCount:   10000,
				LogLevel:      "info",
				SendGetBodyAs: "json",
			},
		},
		{
			name: "No Defaults Applied",
			target: &Configuration{
				RemoteReadClusters: []string{"cluster1", "cluster2"},
				Authentication: Authentication{
					BasicAuthentication: newBasicAuth("sourceUser", "sourcePass", ""),
				},
				Sniffing: Sniffing{
					Enabled:  true,
					UseHTTPS: true,
				},
				MaxSpanAge:               100,
				AdaptiveSamplingLookback: 50,
				Indices: Indices{
					IndexPrefix: "hello",
					Spans: IndexOptions{
						Shards:   5,
						Replicas: int64Ptr(1),
						Priority: 10,
					},
					Services: IndexOptions{
						Shards:   5,
						Replicas: int64Ptr(1),
						Priority: 20,
					},
					Dependencies: IndexOptions{
						Shards:   5,
						Replicas: int64Ptr(1),
						Priority: 30,
					},
				},
				BulkProcessing: BulkProcessing{
					MaxBytes:      1000,
					Workers:       10,
					MaxActions:    100,
					FlushInterval: 30,
				},
				Tags:          TagsAsFields{AllAsFields: true, DotReplacement: "dot", Include: "include", File: "file"},
				MaxDocCount:   10000,
				LogLevel:      "info",
				SendGetBodyAs: "json",
			},
			expected: &Configuration{
				RemoteReadClusters: []string{"cluster1", "cluster2"},
				Authentication: Authentication{
					BasicAuthentication: newBasicAuth("sourceUser", "sourcePass", ""),
				},
				Sniffing: Sniffing{
					Enabled:  true,
					UseHTTPS: true,
				},
				MaxSpanAge:               100,
				AdaptiveSamplingLookback: 50,
				Indices: Indices{
					IndexPrefix: "hello",
					Spans: IndexOptions{
						Shards:   5,
						Replicas: int64Ptr(1),
						Priority: 10,
					},
					Services: IndexOptions{
						Shards:   5,
						Replicas: int64Ptr(1),
						Priority: 20,
					},
					Dependencies: IndexOptions{
						Shards:   5,
						Replicas: int64Ptr(1),
						Priority: 30,
					},
				},
				BulkProcessing: BulkProcessing{
					MaxBytes:      1000,
					Workers:       10,
					MaxActions:    100,
					FlushInterval: 30,
				},
				Tags:          TagsAsFields{AllAsFields: true, DotReplacement: "dot", Include: "include", File: "file"},
				MaxDocCount:   10000,
				LogLevel:      "info",
				SendGetBodyAs: "json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.target.ApplyDefaults(source)
			compareConfigs(t, tt.target, tt.expected)
		})
	}
}

func TestTagKeysAsFields(t *testing.T) {
	tempFile := filepath.Join(t.TempDir(), "test_tags.txt")
	require.NoError(t, os.WriteFile(tempFile, []byte("tag1\ntag2\n\n"), 0o600))

	tests := []struct {
		name         string
		config       *Configuration
		expectedTags []string
		expectError  bool
	}{
		{
			name:         "Empty config",
			config:       &Configuration{Tags: TagsAsFields{}},
			expectedTags: nil,
			expectError:  false,
		},
		{
			name:         "Include only",
			config:       &Configuration{Tags: TagsAsFields{Include: "foo,bar"}},
			expectedTags: []string{"foo", "bar"},
			expectError:  false,
		},
		{
			name:         "File not found",
			config:       &Configuration{Tags: TagsAsFields{File: "/nonexistent/file"}},
			expectedTags: nil,
			expectError:  true,
		},
		{
			name:         "File with tags",
			config:       &Configuration{Tags: TagsAsFields{File: tempFile}},
			expectedTags: []string{"tag1", "tag2"},
			expectError:  false,
		},
		{
			name: "include with tags",
			config: &Configuration{
				Tags: TagsAsFields{
					File:    "",
					Include: "cmdtag1,cmdtag2",
				},
			},
			expectedTags: []string{"cmdtag1", "cmdtag2"},
			expectError:  false,
		},
		{
			name: "File and include with tags",
			config: &Configuration{
				Tags: TagsAsFields{
					File:    tempFile,
					Include: "cmdtag1,cmdtag2",
				},
			},
			expectedTags: []string{"tag1", "tag2", "cmdtag1", "cmdtag2"},
			expectError:  false,
		},
		{
			name: "File read error",
			config: &Configuration{
				Tags: TagsAsFields{
					File:    "/invalid/path/to/file.txt",
					Include: "",
				},
			},
			expectedTags: nil,
			expectError:  true,
		},
		{
			name: "Empty file and params",
			config: &Configuration{
				Tags: TagsAsFields{
					File:    "",
					Include: "",
				},
			},
			expectedTags: nil,
			expectError:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tags, err := test.config.TagKeysAsFields()

			if test.expectError {
				require.Error(t, err)
				require.Nil(t, tags)
			} else {
				require.NoError(t, err)
				require.ElementsMatch(t, test.expectedTags, tags)
			}
		})
	}
}

func TestRolloverFrequencyAsNegativeDuration(t *testing.T) {
	tests := []struct {
		name           string
		indexFrequency string
		expected       time.Duration
	}{
		{
			name:           "hourly jaeger-span",
			indexFrequency: "hour",
			expected:       -1 * time.Hour,
		},
		{
			name:           "daily jaeger-span",
			indexFrequency: "daily",
			expected:       -24 * time.Hour,
		},
		{
			name:           "empty jaeger-span",
			indexFrequency: "",
			expected:       -24 * time.Hour,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := RolloverFrequencyAsNegativeDuration(test.indexFrequency)
			require.Equal(t, test.expected, got)
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name          string
		config        *Configuration
		expectedError string
	}{
		{
			name: "All valid input are set",
			config: &Configuration{
				Servers: []string{"localhost:8000/dummyserver"},
			},
		},
		{
			name:          "no valid input are set",
			config:        &Configuration{},
			expectedError: "Servers: non zero value required",
		},
		{
			name:          "ilm disabled and read-write aliases enabled error",
			config:        &Configuration{Servers: []string{"localhost:8000/dummyserver"}, UseILM: true},
			expectedError: "UseILM must always be used in conjunction with UseReadWriteAliases to ensure ES writers and readers refer to the single index mapping",
		},
		{
			name:          "ilm and create templates enabled",
			config:        &Configuration{Servers: []string{"localhost:8000/dummyserver"}, UseILM: true, CreateIndexTemplates: true, UseReadWriteAliases: true},
			expectedError: "when UseILM is set true, CreateIndexTemplates must be set to false and index templates must be created by init process of es-rollover app",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.config.Validate()
			if test.expectedError != "" {
				require.ErrorContains(t, got, test.expectedError)
			} else {
				require.NoError(t, got)
			}
		})
	}
}

func TestApplyForIndexPrefix(t *testing.T) {
	tests := []struct {
		testName     string
		prefix       IndexPrefix
		name         string
		expectedName string
	}{
		{
			testName:     "no prefix",
			prefix:       "",
			name:         "hello",
			expectedName: "hello",
		},
		{
			testName:     "empty name",
			prefix:       "bye",
			name:         "",
			expectedName: "bye-",
		},
		{
			testName:     "separator suffix",
			prefix:       "bye-",
			name:         "hello",
			expectedName: "bye-hello",
		},
		{
			testName:     "no separator suffix",
			prefix:       "bye",
			name:         "hello",
			expectedName: "bye-hello",
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			got := test.prefix.Apply(test.name)
			require.Equal(t, test.expectedName, got)
		})
	}
}

func TestHandleBulkAfterCallback_ErrorMetricsEmitted(t *testing.T) {
	mf := metricstest.NewFactory(time.Minute)
	sm := spanstoremetrics.NewWriter(mf, "bulk_index")
	logger := zap.NewNop()
	defer mf.Stop()

	var m sync.Map
	batchID := int64(1)
	start := time.Now().Add(-100 * time.Millisecond)
	m.Store(batchID, start)

	fakeRequests := []elastic.BulkableRequest{nil, nil}
	response := &elastic.BulkResponse{
		Errors: true,
		Items: []map[string]*elastic.BulkResponseItem{
			{
				"index": {
					Status: 500,
					Error:  &elastic.ErrorDetails{Type: "server_error"},
				},
			},
			{
				"index": {
					Status: 200,
					Error:  nil,
				},
			},
		},
	}

	bcb := bulkCallback{
		sm:     sm,
		logger: logger,
	}

	bcb.invoke(batchID, fakeRequests, response, assert.AnError)

	mf.AssertCounterMetrics(t,
		metricstest.ExpectedMetric{
			Name:  "bulk_index.errors",
			Value: 1,
		},
		metricstest.ExpectedMetric{
			Name:  "bulk_index.inserts",
			Value: 1,
		},
		metricstest.ExpectedMetric{
			Name:  "bulk_index.attempts",
			Value: 2,
		},
	)
}

func TestHandleBulkAfterCallback_MissingStartTime(t *testing.T) {
	mf := metricstest.NewFactory(time.Minute)
	sm := spanstoremetrics.NewWriter(mf, "bulk_index")
	logger := zap.NewNop()
	defer mf.Stop()

	batchID := int64(42) // assign any value which is not stored in the map

	fakeRequests := []elastic.BulkableRequest{nil}
	response := &elastic.BulkResponse{
		Errors: true,
		Items: []map[string]*elastic.BulkResponseItem{
			{
				"index": {
					Status: 500,
					Error:  &elastic.ErrorDetails{Type: "mock_error"},
				},
			},
		},
	}

	bcb := bulkCallback{
		sm:     sm,
		logger: logger,
	}

	bcb.invoke(batchID, fakeRequests, response, assert.AnError)

	mf.AssertCounterMetrics(t,
		metricstest.ExpectedMetric{
			Name:  "bulk_index.errors",
			Value: 1,
		},
		metricstest.ExpectedMetric{
			Name:  "bulk_index.inserts",
			Value: 0,
		},
		metricstest.ExpectedMetric{
			Name:  "bulk_index.attempts",
			Value: 1,
		},
	)
}

func TestTagKeysAsFields_WhitespaceLines(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "tags.txt")
	require.NoError(t, os.WriteFile(tmpFile, []byte("tag1\n   \ntag2\n\n\t\n"), 0o600))
	tagsConfig := &Configuration{Tags: TagsAsFields{File: tmpFile}}
	tags, err := tagsConfig.TagKeysAsFields()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"tag1", "tag2"}, tags)
}

func TestGetESOptions(t *testing.T) {
	tests := []struct {
		name               string
		cfg                *Configuration
		disableHealthCheck bool
		wantErr            bool
		validateOptions    func(t *testing.T, options []elastic.ClientOptionFunc)
	}{
		{
			name: "Basic configuration",
			cfg: &Configuration{
				Servers: []string{"http://localhost:9200"},
				Sniffing: Sniffing{
					Enabled:  true,
					UseHTTPS: false,
				},
				HTTPCompression: true,
				SendGetBodyAs:   "POST",
			},
			disableHealthCheck: false,
			wantErr:            false,
			validateOptions: func(t *testing.T, options []elastic.ClientOptionFunc) {
				require.NotNil(t, options)
				require.NotEmpty(t, options, "Expected non-empty options slice")
			},
		},
		{
			name: "HTTPS configuration",
			cfg: &Configuration{
				Servers: []string{"https://localhost:9200"},
				Sniffing: Sniffing{
					Enabled:  false,
					UseHTTPS: true,
				},
				HTTPCompression: false,
				SendGetBodyAs:   "",
			},
			disableHealthCheck: true,
			wantErr:            false,
			validateOptions: func(t *testing.T, options []elastic.ClientOptionFunc) {
				require.NotNil(t, options)
				require.NotEmpty(t, options, "Expected non-empty options slice")
			},
		},
		{
			name: "Minimal configuration",
			cfg: &Configuration{
				Servers: []string{"http://localhost:9200"},
				Sniffing: Sniffing{
					Enabled:  false,
					UseHTTPS: false,
				},
				HTTPCompression: false,
				SendGetBodyAs:   "",
			},
			disableHealthCheck: false,
			wantErr:            false,
			validateOptions: func(t *testing.T, options []elastic.ClientOptionFunc) {
				require.NotNil(t, options)
				require.NotEmpty(t, options, "Expected non-empty options slice")
			},
		},
		{
			name: "Multiple servers",
			cfg: &Configuration{
				Servers: []string{
					"http://localhost:9200",
					"http://localhost:9201",
					"http://localhost:9202",
				},
				Sniffing: Sniffing{
					Enabled:  true,
					UseHTTPS: false,
				},
				HTTPCompression: true,
				SendGetBodyAs:   "GET",
			},
			disableHealthCheck: false,
			wantErr:            false,
			validateOptions: func(t *testing.T, options []elastic.ClientOptionFunc) {
				require.NotNil(t, options)
				require.NotEmpty(t, options, "Expected non-empty options slice")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options, err := tt.cfg.getESOptions(tt.disableHealthCheck)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validateOptions != nil {
					tt.validateOptions(t, options)
				}
			}
		})
	}
}

func TestGetConfigOptions(t *testing.T) {
	tmpDir := t.TempDir()
	apiKeyFile := filepath.Join(tmpDir, "apikey")
	bearerTokenFile := filepath.Join(tmpDir, "bearertoken")
	os.WriteFile(apiKeyFile, []byte("test-api-key"), 0o600)
	os.WriteFile(bearerTokenFile, []byte("file-bearer-token"), 0o600)

	tests := []struct {
		name            string
		cfg             *Configuration
		ctx             context.Context
		prepare         func()
		wantErr         bool
		wantErrContains string
	}{
		{
			name: "APIKey file error",
			cfg: &Configuration{
				Servers:  []string{"http://localhost:9200"},
				LogLevel: "info",
				Sniffing: Sniffing{Enabled: false},
				Authentication: Authentication{
					APIKeyAuthentication: newAPIKeyAuth(false, "non-existent-apikey-file"),
				},
			},
			ctx:             context.Background(),
			wantErr:         true,
			wantErrContains: "no such file or directory",
		},
		{
			name: "APIKey file present",
			cfg: &Configuration{
				Servers:  []string{"http://localhost:9200"},
				LogLevel: "info",
				Sniffing: Sniffing{Enabled: false},
				Authentication: Authentication{
					APIKeyAuthentication: newAPIKeyAuth(false, apiKeyFile),
				},
			},
			ctx:     context.Background(),
			wantErr: false,
		},
		{
			name: "APIKey context propagation",
			cfg: &Configuration{
				Servers:  []string{"http://localhost:9200"},
				LogLevel: "info",
				Sniffing: Sniffing{Enabled: false},
				Authentication: Authentication{
					APIKeyAuthentication: newAPIKeyAuth(true, ""),
				},
			},
			ctx:     apikey.ContextWithAPIKey(context.Background(), "context-api-key"),
			wantErr: false,
		},
		{
			name: "APIKey file and context both enabled",
			cfg: &Configuration{
				Servers:  []string{"http://localhost:9200"},
				LogLevel: "info",
				Sniffing: Sniffing{Enabled: false},
				Authentication: Authentication{
					APIKeyAuthentication: newAPIKeyAuth(true, apiKeyFile),
				},
			},
			ctx:     apikey.ContextWithAPIKey(context.Background(), "context-api-key"),
			wantErr: false,
		},
		{
			name: "BearerToken context propagation",
			cfg: &Configuration{
				Servers:  []string{"http://localhost:9200"},
				Sniffing: Sniffing{Enabled: false},
				Authentication: Authentication{
					BearerTokenAuthentication: newBearerTokenAuth(true, ""),
				},
				LogLevel: "info",
			},
			ctx:     bearertoken.ContextWithBearerToken(context.Background(), "context-bearer-token"),
			wantErr: false,
		},
		{
			name: "BearerToken file and context both enabled",
			cfg: &Configuration{
				Servers:  []string{"http://localhost:9200"},
				Sniffing: Sniffing{Enabled: false},
				Authentication: Authentication{
					BearerTokenAuthentication: newBearerTokenAuth(true, bearerTokenFile),
				},
				LogLevel: "info",
			},
			ctx:     bearertoken.ContextWithBearerToken(context.Background(), "context-bearer-token"),
			wantErr: false,
		},
		{
			name: "BearerToken file error",
			cfg: &Configuration{
				Servers:  []string{"http://localhost:9200"},
				TLS:      configtls.ClientConfig{Insecure: true},
				Sniffing: Sniffing{Enabled: false},
				Authentication: Authentication{
					BearerTokenAuthentication: configoptional.Some(BearerTokenAuthentication{
						FilePath: "/does/not/exist/token",
					}),
				},
				LogLevel: "info",
			},
			ctx:             context.Background(),
			wantErr:         true,
			wantErrContains: "no such file or directory",
		},
		{
			name: "No auth configured",
			cfg: &Configuration{
				Servers:  []string{"http://localhost:9200"},
				LogLevel: "info",
				Sniffing: Sniffing{Enabled: false},
			},
			ctx:     context.Background(),
			wantErr: false,
		},
		{
			name: "BasicAuth password file error",
			cfg: &Configuration{
				Servers:  []string{"http://localhost:9200"},
				Sniffing: Sniffing{Enabled: false},
				Authentication: Authentication{
					BasicAuthentication: configoptional.Some(BasicAuthentication{
						PasswordFilePath: "/does/not/exist",
					}),
				},
				LogLevel: "info",
			},
			ctx:             context.Background(),
			wantErr:         true,
			wantErrContains: "failed to load password from file",
		},
		{
			name: "BasicAuth both Password and PasswordFilePath set",
			cfg: &Configuration{
				Servers:  []string{"http://localhost:9200"},
				Sniffing: Sniffing{Enabled: false},
				Authentication: Authentication{
					BasicAuthentication: configoptional.Some(BasicAuthentication{
						Password:         "secret",
						PasswordFilePath: "/some/file/path",
					}),
				},
				LogLevel: "info",
			},
			ctx:             context.Background(),
			wantErr:         true,
			wantErrContains: "both Password and PasswordFilePath are set",
		},
		{
			name: "Invalid log level triggers addLoggerOptions error",
			cfg: &Configuration{
				Servers:  []string{"http://localhost:9200"},
				Sniffing: Sniffing{Enabled: false},
				Authentication: Authentication{
					BasicAuthentication: newBasicAuth("user", "secret", ""),
				},
				LogLevel: "invalid",
			},
			ctx:             context.Background(),
			wantErr:         true,
			wantErrContains: "unrecognized log-level",
		},
		{
			name: "Health check disabled for context-only auth",
			cfg: &Configuration{
				Servers:            []string{"http://localhost:9200"},
				LogLevel:           "info",
				DisableHealthCheck: false, // Should be overridden by context-only auth
				Sniffing:           Sniffing{Enabled: false},
				Authentication: Authentication{
					BearerTokenAuthentication: configoptional.Some(BearerTokenAuthentication{
						AllowFromContext: true,
						FilePath:         "", // No file path, only context
					}),
				},
			},
			ctx:     bearertoken.ContextWithBearerToken(context.Background(), "context-bearer-token"),
			wantErr: false,
		},
		{
			name: "Health check disabled explicitly",
			cfg: &Configuration{
				Servers:            []string{"http://localhost:9200"},
				LogLevel:           "info",
				DisableHealthCheck: true,
				Sniffing:           Sniffing{Enabled: false},
			},
			ctx:     context.Background(),
			wantErr: false,
		},
		{
			name: "HTTP compression and custom SendGetBodyAs",
			cfg: &Configuration{
				Servers:         []string{"http://localhost:9200"},
				LogLevel:        "info",
				HTTPCompression: true,
				SendGetBodyAs:   "POST",
				Sniffing:        Sniffing{Enabled: true, UseHTTPS: true},
			},
			ctx:     context.Background(),
			wantErr: false,
		},
	}

	logger := zap.NewNop()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.prepare != nil {
				tt.prepare()
			}
			options, err := tt.cfg.getConfigOptions(tt.ctx, logger)
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrContains != "" {
					require.Contains(t, err.Error(), tt.wantErrContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, options)
				require.NotEmpty(t, options, "Should have at least basic ES options")
			}
		})
	}
}

func TestGetConfigOptionsIntegration(t *testing.T) {
	// Test that getConfigOptions properly integrates with getESOptions
	cfg := &Configuration{
		Servers: []string{"http://localhost:9200"},
		Sniffing: Sniffing{
			Enabled:  true,
			UseHTTPS: false,
		},
		HTTPCompression: true,
		SendGetBodyAs:   "POST",
		LogLevel:        "info",
		QueryTimeout:    30 * time.Second,
		Authentication: Authentication{
			BasicAuthentication: configoptional.Some(BasicAuthentication{
				Username: "testuser",
				Password: "testpass",
			}),
		},
	}

	logger := zap.NewNop()
	options, err := cfg.getConfigOptions(context.Background(), logger)

	require.NoError(t, err)
	require.NotNil(t, options)

	require.Greater(t, len(options), 5, "Should have basic ES options plus additional config options")
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
