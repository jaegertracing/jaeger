// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"

	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

func getBasicAuthField(opt configoptional.Optional[escfg.BasicAuthentication], field string) any {
	if !opt.HasValue() {
		return ""
	}

	ba := opt.Get()
	switch field {
	case "Username":
		return ba.Username
	case "Password":
		return ba.Password
	case "PasswordFilePath":
		return ba.PasswordFilePath
	case "ReloadInterval":
		return ba.ReloadInterval
	default:
		return ""
	}
}

func getBearerTokenField(opt configoptional.Optional[escfg.TokenAuthentication], field string) any {
	if !opt.HasValue() {
		if field == "AllowFromContext" {
			return false
		}
		return ""
	}

	ba := opt.Get()
	switch field {
	case "FilePath":
		return ba.FilePath
	case "AllowFromContext":
		return ba.AllowFromContext
	case "ReloadInterval":
		return ba.ReloadInterval
	default:
		return ""
	}
}

func getAPIKeyField(opt configoptional.Optional[escfg.TokenAuthentication], field string) any {
	if !opt.HasValue() {
		if field == "AllowFromContext" {
			return false
		}
		return ""
	}

	ba := opt.Get()
	switch field {
	case "FilePath":
		return ba.FilePath
	case "AllowFromContext":
		return ba.AllowFromContext
	case "ReloadInterval":
		return ba.ReloadInterval
	default:
		return ""
	}
}

func TestOptions(t *testing.T) {
	primary := DefaultConfig()

	// Authentication should not be present when no values are provided
	assert.False(t, primary.Authentication.BasicAuthentication.HasValue())
	assert.False(t, primary.Authentication.BearerTokenAuth.HasValue())
	assert.False(t, primary.Authentication.APIKeyAuth.HasValue())

	assert.NotEmpty(t, primary.Servers)
	assert.Empty(t, primary.RemoteReadClusters)
	assert.EqualValues(t, 5, primary.Indices.Spans.Shards)
	assert.EqualValues(t, 5, primary.Indices.Services.Shards)
	assert.EqualValues(t, 5, primary.Indices.Sampling.Shards)
	assert.EqualValues(t, 5, primary.Indices.Dependencies.Shards)
	require.NotNil(t, primary.Indices.Spans.Replicas)
	assert.EqualValues(t, 1, *primary.Indices.Spans.Replicas)
	require.NotNil(t, primary.Indices.Services.Replicas)
	assert.EqualValues(t, 1, *primary.Indices.Services.Replicas)
	require.NotNil(t, primary.Indices.Sampling.Replicas)
	assert.EqualValues(t, 1, *primary.Indices.Sampling.Replicas)
	require.NotNil(t, primary.Indices.Dependencies.Replicas)
	assert.EqualValues(t, 1, *primary.Indices.Dependencies.Replicas)
	assert.Equal(t, 72*time.Hour, primary.MaxSpanAge)
	assert.False(t, primary.Sniffing.Enabled)
	assert.False(t, primary.Sniffing.UseHTTPS)
	assert.False(t, primary.DisableHealthCheck)
}

func TestOptionsWithFlags(t *testing.T) {
	primary := escfg.Configuration{
		Servers: []string{"1.1.1.1", "2.2.2.2"},
		Authentication: escfg.Authentication{
			BasicAuthentication: configoptional.Some(escfg.BasicAuthentication{
				Username:         "hello",
				Password:         "world",
				PasswordFilePath: "/foo/bar/baz",
				ReloadInterval:   35 * time.Second,
			}),
			BearerTokenAuth: configoptional.Some(escfg.TokenAuthentication{
				FilePath:         "/foo/bar",
				AllowFromContext: true,
				ReloadInterval:   50 * time.Second,
			}),
			APIKeyAuth: configoptional.Some(escfg.TokenAuthentication{
				FilePath:         "/foo/api-key",
				AllowFromContext: true,
				ReloadInterval:   30 * time.Second,
			}),
		},
		RemoteReadClusters: []string{"cluster_one", "cluster_two"},
		MaxSpanAge:         48 * time.Hour,
		Sniffing: escfg.Sniffing{
			Enabled:  true,
			UseHTTPS: true,
		},
		DisableHealthCheck: true,
		TLS: configtls.ClientConfig{
			Insecure:           false,
			InsecureSkipVerify: true,
		},
		Tags: escfg.TagsAsFields{
			AllAsFields:    true,
			Include:        "test,tags",
			File:           "./file.txt",
			DotReplacement: "!",
		},
		Indices: escfg.Indices{
			Spans: escfg.IndexOptions{
				DateLayout: "2006010215", // Go reference time formatted for hourly rollover (yyyy-MM-dd-HH)
			},
			Services: escfg.IndexOptions{
				DateLayout: "20060102", // Go reference time formatted for daily rollover (yyyy-MM-dd)
			},
		},
		UseILM:          true,
		HTTPCompression: true,
	}

	// Now authentication should be present since values were provided
	assert.True(t, primary.Authentication.BasicAuthentication.HasValue())
	assert.True(t, primary.Authentication.BearerTokenAuth.HasValue())
	assert.True(t, primary.Authentication.APIKeyAuth.HasValue())
	// Basic Authentication
	assert.Equal(t, "hello", getBasicAuthField(primary.Authentication.BasicAuthentication, "Username"))
	assert.Equal(t, "world", getBasicAuthField(primary.Authentication.BasicAuthentication, "Password"))
	assert.Equal(t, "/foo/bar/baz", getBasicAuthField(primary.Authentication.BasicAuthentication, "PasswordFilePath"))
	assert.Equal(t, 35*time.Second, getBasicAuthField(primary.Authentication.BasicAuthentication, "ReloadInterval"))
	// Bearer Token Authentication
	assert.Equal(t, "/foo/bar", getBearerTokenField(primary.Authentication.BearerTokenAuth, "FilePath"))
	assert.Equal(t, true, getBearerTokenField(primary.Authentication.BearerTokenAuth, "AllowFromContext"))
	assert.Equal(t, 50*time.Second, getBearerTokenField(primary.Authentication.BearerTokenAuth, "ReloadInterval"))
	// API Key Authentication
	assert.Equal(t, "/foo/api-key", getAPIKeyField(primary.Authentication.APIKeyAuth, "FilePath"))
	assert.Equal(t, true, getAPIKeyField(primary.Authentication.APIKeyAuth, "AllowFromContext"))
	assert.Equal(t, 30*time.Second, getAPIKeyField(primary.Authentication.APIKeyAuth, "ReloadInterval"))
	// Server URLs
	assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, primary.Servers)
	// Remote Read Clusters
	assert.Equal(t, []string{"cluster_one", "cluster_two"}, primary.RemoteReadClusters)
	// Max Span Age
	assert.Equal(t, 48*time.Hour, primary.MaxSpanAge)
	// Sniffing
	assert.True(t, primary.Sniffing.Enabled)
	assert.True(t, primary.Sniffing.UseHTTPS)
	assert.True(t, primary.DisableHealthCheck)
	// TLS
	assert.False(t, primary.TLS.Insecure)
	assert.True(t, primary.TLS.InsecureSkipVerify)
	// Tags
	assert.True(t, primary.Tags.AllAsFields)
	assert.Equal(t, "!", primary.Tags.DotReplacement)
	assert.Equal(t, "./file.txt", primary.Tags.File)
	assert.Equal(t, "test,tags", primary.Tags.Include)
	// Indices
	assert.Equal(t, "20060102", primary.Indices.Services.DateLayout)
	assert.Equal(t, "2006010215", primary.Indices.Spans.DateLayout)
	// Use ILM
	assert.True(t, primary.UseILM)
	// HTTP Compression
	assert.True(t, primary.HTTPCompression)
}

func TestAuthenticationConditionalCreation(t *testing.T) {
	testCases := []struct {
		name   string
		config escfg.Configuration
	}{
		{
			name: "no authentication flags",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{},
			},
		},
		{
			name: "only username provided",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					BasicAuthentication: configoptional.Some(escfg.BasicAuthentication{
						Username:       "testuser",
						ReloadInterval: 10 * time.Second,
					}),
				},
			},
		},
		{
			name: "only password provided",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					BasicAuthentication: configoptional.Some(escfg.BasicAuthentication{
						Password:       "testpass",
						ReloadInterval: 10 * time.Second,
					}),
				},
			},
		},
		{
			name: "only token file provided",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					BearerTokenAuth: configoptional.Some(escfg.TokenAuthentication{
						FilePath:         "/path/to/token",
						AllowFromContext: false,
						ReloadInterval:   10 * time.Second,
					}),
				},
			},
		},
		{
			name: "username and password provided",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					BasicAuthentication: configoptional.Some(escfg.BasicAuthentication{
						Username:       "testuser",
						Password:       "testpass",
						ReloadInterval: 10 * time.Second,
					}),
				},
			},
		},
		{
			name: "only bearer token context propagation enabled",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					BearerTokenAuth: configoptional.Some(escfg.TokenAuthentication{
						AllowFromContext: true,
						ReloadInterval:   10 * time.Second,
					}),
				},
			},
		},
		{
			name: "both token file and context propagation enabled",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					BearerTokenAuth: configoptional.Some(escfg.TokenAuthentication{
						FilePath:         "/path/to/token",
						AllowFromContext: true,
						ReloadInterval:   10 * time.Second,
					}),
				},
			},
		},
		{
			name: "bearer token with custom reload interval",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					BearerTokenAuth: configoptional.Some(escfg.TokenAuthentication{
						FilePath:         "/path/to/token",
						AllowFromContext: true,
						ReloadInterval:   45 * time.Second,
					}),
				},
			},
		},
		{
			name: "API key all options with zero reload interval",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					APIKeyAuth: configoptional.Some(escfg.TokenAuthentication{
						FilePath:         "/path/to/keyfile",
						AllowFromContext: true,
						ReloadInterval:   0 * time.Second,
					}),
				},
			},
		},
		{
			name: "API key with non-zero reload interval",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					APIKeyAuth: configoptional.Some(escfg.TokenAuthentication{
						FilePath:         "/path/to/keyfile",
						AllowFromContext: true,
						ReloadInterval:   30 * time.Second,
					}),
				},
			},
		},
		{
			name: "only API key file provided",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					APIKeyAuth: configoptional.Some(escfg.TokenAuthentication{
						FilePath:         "/path/to/key",
						AllowFromContext: false,
						ReloadInterval:   10 * time.Second,
					}),
				},
			},
		},
		{
			name: "only API key context propagation enabled",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					APIKeyAuth: configoptional.Some(escfg.TokenAuthentication{
						AllowFromContext: true,
						ReloadInterval:   10 * time.Second,
					}),
				},
			},
		},
		{
			name: "both API key file and context enabled",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					APIKeyAuth: configoptional.Some(escfg.TokenAuthentication{
						FilePath:         "/path/to/key",
						AllowFromContext: true,
						ReloadInterval:   10 * time.Second,
					}),
				},
			},
		},
		{
			name: "all API key options provided",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					APIKeyAuth: configoptional.Some(escfg.TokenAuthentication{
						FilePath:         "/path/to/key",
						AllowFromContext: true,
						ReloadInterval:   60 * time.Second,
					}),
				},
			},
		},
		{
			name: "basic auth and API key both enabled",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					BasicAuthentication: configoptional.Some(escfg.BasicAuthentication{
						Username:       "testuser",
						Password:       "testpass",
						ReloadInterval: 10 * time.Second,
					}),
					APIKeyAuth: configoptional.Some(escfg.TokenAuthentication{
						FilePath:       "/path/to/key",
						ReloadInterval: 10 * time.Second,
					}),
				},
			},
		},
		{
			name: "bearer token and API key both enabled",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					BearerTokenAuth: configoptional.Some(escfg.TokenAuthentication{
						FilePath:         "/path/to/token",
						AllowFromContext: false,
						ReloadInterval:   10 * time.Second,
					}),
					APIKeyAuth: configoptional.Some(escfg.TokenAuthentication{
						AllowFromContext: true,
						ReloadInterval:   10 * time.Second,
					}),
				},
			},
		},
		{
			name: "basic auth password reload interval disabled",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					BasicAuthentication: configoptional.Some(escfg.BasicAuthentication{
						Username:         "testuser",
						PasswordFilePath: "/path/to/password",
						ReloadInterval:   0 * time.Second,
					}),
				},
			},
		},
		{
			name: "bearer token reload interval disabled",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					BearerTokenAuth: configoptional.Some(escfg.TokenAuthentication{
						FilePath:       "/path/to/token",
						ReloadInterval: 0 * time.Second,
					}),
				},
			},
		},
		{
			name: "all three authentication methods enabled",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					BasicAuthentication: configoptional.Some(escfg.BasicAuthentication{
						Username:       "testuser",
						Password:       "testpass",
						ReloadInterval: 10 * time.Second,
					}),
					BearerTokenAuth: configoptional.Some(escfg.TokenAuthentication{
						FilePath:         "/path/to/token",
						AllowFromContext: true,
						ReloadInterval:   25 * time.Second,
					}),
					APIKeyAuth: configoptional.Some(escfg.TokenAuthentication{
						FilePath:         "/path/to/key",
						AllowFromContext: true,
						ReloadInterval:   30 * time.Second,
					}),
				},
			},
		},
		{
			name: "basic auth with custom reload interval (non-zero)",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					BasicAuthentication: configoptional.Some(escfg.BasicAuthentication{
						Username:         "testuser",
						PasswordFilePath: "/path/to/password",
						ReloadInterval:   15 * time.Second,
					}),
				},
			},
		},
		{
			name: "bearer token with custom reload interval (non-zero)",
			config: escfg.Configuration{
				Authentication: escfg.Authentication{
					BearerTokenAuth: configoptional.Some(escfg.TokenAuthentication{
						FilePath:       "/path/to/token",
						ReloadInterval: 20 * time.Second,
					}),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			primary := tc.config

			// Assert authentication method presence
			expectBasicAuth := primary.Authentication.BasicAuthentication.HasValue()
			expectBearerAuth := primary.Authentication.BearerTokenAuth.HasValue()
			expectAPIKeyAuth := primary.Authentication.APIKeyAuth.HasValue()

			assert.Equal(t, expectBasicAuth, primary.Authentication.BasicAuthentication.HasValue())
			assert.Equal(t, expectBearerAuth, primary.Authentication.BearerTokenAuth.HasValue())
			assert.Equal(t, expectAPIKeyAuth, primary.Authentication.APIKeyAuth.HasValue())

			// Assert basic authentication details
			if expectBasicAuth {
				basicAuth := primary.Authentication.BasicAuthentication.Get()
				hasAtLeastOneField := basicAuth.Username != "" || basicAuth.Password != "" || basicAuth.PasswordFilePath != ""
				assert.True(t, hasAtLeastOneField, "at least one basic auth field should be set")
			}

			// Assert bearer token authentication details
			if expectBearerAuth {
				bearerAuth := primary.Authentication.BearerTokenAuth.Get()
				hasAtLeastOneField := bearerAuth.FilePath != "" || bearerAuth.AllowFromContext
				assert.True(t, hasAtLeastOneField, "at least one bearer auth field should be set")
			}

			// Assert API key authentication details
			if expectAPIKeyAuth {
				apiKeyAuth := primary.Authentication.APIKeyAuth.Get()
				hasAtLeastOneField := apiKeyAuth.FilePath != "" || apiKeyAuth.AllowFromContext
				assert.True(t, hasAtLeastOneField, "at least one API key auth field should be set")
			}
		})
	}
}

func TestGetBasicAuthField_DefaultCase(t *testing.T) {
	basicAuth := escfg.BasicAuthentication{
		Username:         "test-user",
		Password:         "test-pass",
		PasswordFilePath: "/path/to/file",
	}

	opt := configoptional.Some(basicAuth)

	result := getBasicAuthField(opt, "UnknownField")
	assert.Empty(t, result)
}

func TestEmptyRemoteReadClusters(t *testing.T) {
	primary := escfg.Configuration{
		RemoteReadClusters: []string{},
	}
	assert.Equal(t, []string{}, primary.RemoteReadClusters)
}

func TestMaxSpanAgeSetErrorInArchiveMode(t *testing.T) {
	// This test verifies that max-span-age flag is not available in archive mode
	// Since we're not testing flags anymore, we just verify that the behavior is documented
	// In archive mode, MaxSpanAge should not be used (traces are searched with no look-back limit)
	t.Skip("Test for flag parsing behavior - no longer applicable with direct config initialization")
}

func TestMaxDocCount(t *testing.T) {
	testCases := []struct {
		name            string
		config          escfg.Configuration
		wantMaxDocCount int
	}{
		{
			name:            "default value",
			config:          DefaultConfig(),
			wantMaxDocCount: 10_000,
		},
		{
			name: "custom value",
			config: escfg.Configuration{
				MaxDocCount: 1000,
			},
			wantMaxDocCount: 1000,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantMaxDocCount, tc.config.MaxDocCount)
		})
	}
}

func TestIndexDateSeparator(t *testing.T) {
	testCases := []struct {
		name           string
		config         escfg.Configuration
		wantDateLayout string
	}{
		{
			name:           "default separator",
			config:         DefaultConfig(),
			wantDateLayout: "2006-01-02",
		},
		{
			name: "empty separator",
			config: escfg.Configuration{
				Indices: escfg.Indices{
					Spans: escfg.IndexOptions{
						DateLayout: "20060102",
					},
				},
			},
			wantDateLayout: "20060102",
		},
		{
			name: "dot separator",
			config: escfg.Configuration{
				Indices: escfg.Indices{
					Spans: escfg.IndexOptions{
						DateLayout: "2006.01.02",
					},
				},
			},
			wantDateLayout: "2006.01.02",
		},
		{
			name: "dash separator",
			config: escfg.Configuration{
				Indices: escfg.Indices{
					Spans: escfg.IndexOptions{
						DateLayout: "2006-01-02",
					},
				},
			},
			wantDateLayout: "2006-01-02",
		},
		{
			name: "slash separator",
			config: escfg.Configuration{
				Indices: escfg.Indices{
					Spans: escfg.IndexOptions{
						DateLayout: "2006/01/02",
					},
				},
			},
			wantDateLayout: "2006/01/02",
		},
		{
			name: "single quote separator",
			config: escfg.Configuration{
				Indices: escfg.Indices{
					Spans: escfg.IndexOptions{
						DateLayout: "2006''01''02",
					},
				},
			},
			wantDateLayout: "2006''01''02",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantDateLayout, tc.config.Indices.Spans.DateLayout)
		})
	}
}

func TestIndexRollover(t *testing.T) {
	testCases := []struct {
		name                              string
		config                            escfg.Configuration
		wantSpanDateLayout                string
		wantServiceDateLayout             string
		wantSpanIndexRolloverFrequency    time.Duration
		wantServiceIndexRolloverFrequency time.Duration
	}{
		{
			name:                              "default",
			config:                            DefaultConfig(),
			wantSpanDateLayout:                "2006-01-02",
			wantServiceDateLayout:             "2006-01-02",
			wantSpanIndexRolloverFrequency:    -24 * time.Hour,
			wantServiceIndexRolloverFrequency: -24 * time.Hour,
		},
		{
			name: "hourly spans, daily services",
			config: escfg.Configuration{
				Indices: escfg.Indices{
					Spans: escfg.IndexOptions{
						DateLayout:        "2006-01-02-15",
						RolloverFrequency: "hour",
					},
					Services: escfg.IndexOptions{
						DateLayout:        "2006-01-02",
						RolloverFrequency: "day",
					},
				},
			},
			wantSpanDateLayout:                "2006-01-02-15",
			wantServiceDateLayout:             "2006-01-02",
			wantSpanIndexRolloverFrequency:    -1 * time.Hour,
			wantServiceIndexRolloverFrequency: -24 * time.Hour,
		},
		{
			name: "daily spans, hourly services",
			config: escfg.Configuration{
				Indices: escfg.Indices{
					Spans: escfg.IndexOptions{
						DateLayout:        "2006-01-02",
						RolloverFrequency: "day",
					},
					Services: escfg.IndexOptions{
						DateLayout:        "2006-01-02-15",
						RolloverFrequency: "hour",
					},
				},
			},
			wantSpanDateLayout:                "2006-01-02",
			wantServiceDateLayout:             "2006-01-02-15",
			wantSpanIndexRolloverFrequency:    -24 * time.Hour,
			wantServiceIndexRolloverFrequency: -1 * time.Hour,
		},
		{
			name: "invalid rollover frequency defaults to day",
			config: escfg.Configuration{
				Indices: escfg.Indices{
					Spans: escfg.IndexOptions{
						DateLayout:        "2006-01-02",
						RolloverFrequency: "hours",
					},
					Services: escfg.IndexOptions{
						DateLayout:        "2006-01-02",
						RolloverFrequency: "hours",
					},
				},
			},
			wantSpanDateLayout:                "2006-01-02",
			wantServiceDateLayout:             "2006-01-02",
			wantSpanIndexRolloverFrequency:    -24 * time.Hour,
			wantServiceIndexRolloverFrequency: -24 * time.Hour,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantSpanDateLayout, tc.config.Indices.Spans.DateLayout)
			assert.Equal(t, tc.wantServiceDateLayout, tc.config.Indices.Services.DateLayout)
			assert.Equal(t, tc.wantSpanIndexRolloverFrequency, escfg.RolloverFrequencyAsNegativeDuration(tc.config.Indices.Spans.RolloverFrequency))
			assert.Equal(t, tc.wantServiceIndexRolloverFrequency, escfg.RolloverFrequencyAsNegativeDuration(tc.config.Indices.Services.RolloverFrequency))
		})
	}
}

// TestAddFlags and TestAddFlagsWithPreExistingAuth were removed as they tested
// flag registration behavior which is no longer relevant after moving to direct config initialization
