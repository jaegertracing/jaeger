// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"flag"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"

	"github.com/jaegertracing/jaeger/internal/config"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

// basicAuth creates basic authentication component
func basicAuth(username, password, passwordFilePath string, reloadInterval time.Duration) configoptional.Optional[escfg.BasicAuthentication] {
	return configoptional.Some(escfg.BasicAuthentication{
		Username:         username,
		Password:         password,
		PasswordFilePath: passwordFilePath,
		ReloadInterval:   reloadInterval,
	})
}

// bearerAuth creates bearer token authentication component
func bearerAuth(filePath string, allowFromContext bool, reloadInterval time.Duration) configoptional.Optional[escfg.BearerTokenAuthentication] {
	return configoptional.Some(escfg.BearerTokenAuthentication{
		TokenAuthBase: escfg.TokenAuthBase{
			FilePath:         filePath,
			AllowFromContext: allowFromContext,
			ReloadInterval:   reloadInterval,
		},
	})
}

// apiKeyAuth creates api key authentication component
func apiKeyAuth(filePath string, allowFromContext bool, reloadInterval time.Duration) configoptional.Optional[escfg.APIKeyAuthentication] {
	return configoptional.Some(escfg.APIKeyAuthentication{
		TokenAuthBase: escfg.TokenAuthBase{
			FilePath:         filePath,
			AllowFromContext: allowFromContext,
			ReloadInterval:   reloadInterval,
		},
	})
}

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

func getBearerTokenField(opt configoptional.Optional[escfg.BearerTokenAuthentication], field string) any {
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

func getAPIKeyField(opt configoptional.Optional[escfg.APIKeyAuthentication], field string) any {
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
	opts := NewOptions("foo")
	primary := opts.GetConfig()

	// Authentication should not be present when no values are provided
	assert.False(t, primary.Authentication.BasicAuthentication.HasValue())
	assert.False(t, primary.Authentication.BearerTokenAuthentication.HasValue())
	assert.False(t, primary.Authentication.APIKeyAuthentication.HasValue())

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
	opts := NewOptions("es")
	v, command := config.Viperize(opts.AddFlags)
	err := command.ParseFlags([]string{
		"--es.server-urls=1.1.1.1, 2.2.2.2",
		"--es.username=hello",
		"--es.password=world",
		"--es.token-file=/foo/bar",
		"--es.password-file=/foo/bar/baz",
		"--es.bearer-token-propagation=true",
		"--es.bearer-token-reload-interval=50s",
		"--es.api-key-file=/foo/api-key",
		"--es.api-key-allow-from-context=true",
		"--es.api-key-reload-interval=30s",
		"--es.password-reload-interval=35s",
		"--es.sniffer=true",
		"--es.sniffer-tls-enabled=true",
		"--es.disable-health-check=true",
		"--es.max-span-age=48h",
		"--es.num-shards=20",
		"--es.num-replicas=10",
		"--es.index-date-separator=",
		"--es.index-rollover-frequency-spans=hour",
		"--es.index-rollover-frequency-services=day",
		"--es.remote-read-clusters=cluster_one,cluster_two",
		"--es.tls.enabled=true",
		"--es.tls.skip-host-verify=true",
		"--es.tags-as-fields.all=true",
		"--es.tags-as-fields.include=test,tags",
		"--es.tags-as-fields.config-file=./file.txt",
		"--es.tags-as-fields.dot-replacement=!",
		"--es.use-ilm=true",
		"--es.send-get-body-as=POST",
		"--es.http-compression=true",
	})
	require.NoError(t, err)
	opts.InitFromViper(v)
	primary := opts.GetConfig()

	// Now authentication should be present since values were provided
	assert.True(t, primary.Authentication.BasicAuthentication.HasValue())
	assert.True(t, primary.Authentication.BearerTokenAuthentication.HasValue())
	assert.True(t, primary.Authentication.APIKeyAuthentication.HasValue())
	// Basic Authentication
	assert.Equal(t, "hello", getBasicAuthField(primary.Authentication.BasicAuthentication, "Username"))
	assert.Equal(t, "world", getBasicAuthField(primary.Authentication.BasicAuthentication, "Password"))
	assert.Equal(t, "/foo/bar/baz", getBasicAuthField(primary.Authentication.BasicAuthentication, "PasswordFilePath"))
	assert.Equal(t, 35*time.Second, getBasicAuthField(primary.Authentication.BasicAuthentication, "ReloadInterval"))
	// Bearer Token Authentication
	assert.Equal(t, "/foo/bar", getBearerTokenField(primary.Authentication.BearerTokenAuthentication, "FilePath"))
	assert.Equal(t, true, getBearerTokenField(primary.Authentication.BearerTokenAuthentication, "AllowFromContext"))
	assert.Equal(t, 50*time.Second, getBearerTokenField(primary.Authentication.BearerTokenAuthentication, "ReloadInterval"))
	// API Key Authentication
	assert.Equal(t, "/foo/api-key", getAPIKeyField(primary.Authentication.APIKeyAuthentication, "FilePath"))
	assert.Equal(t, true, getAPIKeyField(primary.Authentication.APIKeyAuthentication, "AllowFromContext"))
	assert.Equal(t, 30*time.Second, getAPIKeyField(primary.Authentication.APIKeyAuthentication, "ReloadInterval"))
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
		name                           string
		flags                          []string
		expectBasicAuth                bool
		expectBearerAuth               bool
		expectAPIKeyAuth               bool
		expectedUsername               string
		expectedPassword               string
		expectedPasswordFilePath       string
		expectedPasswordReloadInterval time.Duration
		expectedTokenPath              string
		expectedBearerFromContext      bool
		expectedBearerReloadInterval   time.Duration
		expectedAPIKeyFilePath         string
		expectedAPIKeyFromContext      bool
		expectedAPIKeyReloadInterval   time.Duration
	}{
		{
			name:             "no authentication flags",
			flags:            []string{},
			expectBasicAuth:  false,
			expectBearerAuth: false,
			expectAPIKeyAuth: false,
		},
		{
			name:                           "only username provided",
			flags:                          []string{"--es.username=testuser"},
			expectBasicAuth:                true,
			expectBearerAuth:               false,
			expectAPIKeyAuth:               false,
			expectedUsername:               "testuser",
			expectedPasswordReloadInterval: 10 * time.Second,
		},
		{
			name:                           "only password provided",
			flags:                          []string{"--es.password=testpass"},
			expectBasicAuth:                true,
			expectBearerAuth:               false,
			expectAPIKeyAuth:               false,
			expectedPassword:               "testpass",
			expectedPasswordReloadInterval: 10 * time.Second,
		},
		{
			name:                         "only token file provided",
			flags:                        []string{"--es.token-file=/path/to/token"},
			expectBasicAuth:              false,
			expectBearerAuth:             true,
			expectAPIKeyAuth:             false,
			expectedTokenPath:            "/path/to/token",
			expectedBearerFromContext:    false,
			expectedBearerReloadInterval: 10 * time.Second,
		},
		{
			name:                           "username and password provided",
			flags:                          []string{"--es.username=testuser", "--es.password=testpass"},
			expectBasicAuth:                true,
			expectBearerAuth:               false,
			expectAPIKeyAuth:               false,
			expectedUsername:               "testuser",
			expectedPassword:               "testpass",
			expectedPasswordReloadInterval: 10 * time.Second,
		},
		{
			name:                         "only bearer token context propagation enabled",
			flags:                        []string{"--es.bearer-token-propagation=true"},
			expectBasicAuth:              false,
			expectBearerAuth:             true,
			expectAPIKeyAuth:             false,
			expectedBearerFromContext:    true,
			expectedBearerReloadInterval: 10 * time.Second,
		},
		{
			name:                         "both token file and context propagation enabled",
			flags:                        []string{"--es.token-file=/path/to/token", "--es.bearer-token-propagation=true"},
			expectBasicAuth:              false,
			expectBearerAuth:             true,
			expectAPIKeyAuth:             false,
			expectedTokenPath:            "/path/to/token",
			expectedBearerFromContext:    true,
			expectedBearerReloadInterval: 10 * time.Second,
		},
		{
			name: "bearer token with custom reload interval",
			flags: []string{
				"--es.token-file=/path/to/token",
				"--es.bearer-token-propagation=true",
				"--es.bearer-token-reload-interval=45s",
			},
			expectBasicAuth:              false,
			expectBearerAuth:             true,
			expectAPIKeyAuth:             false,
			expectedTokenPath:            "/path/to/token",
			expectedBearerFromContext:    true,
			expectedBearerReloadInterval: 45 * time.Second,
		},
		{
			name: "API key all options with zero reload interval",
			flags: []string{
				"--es.api-key-file=/path/to/keyfile",
				"--es.api-key-allow-from-context=true",
				"--es.api-key-reload-interval=0s",
			},
			expectBasicAuth:              false,
			expectBearerAuth:             false,
			expectAPIKeyAuth:             true,
			expectedAPIKeyFilePath:       "/path/to/keyfile",
			expectedAPIKeyFromContext:    true,
			expectedAPIKeyReloadInterval: 0 * time.Second,
		},
		{
			name: "API key with non-zero reload interval",
			flags: []string{
				"--es.api-key-file=/path/to/keyfile",
				"--es.api-key-allow-from-context=true",
				"--es.api-key-reload-interval=30s",
			},
			expectBasicAuth:              false,
			expectBearerAuth:             false,
			expectAPIKeyAuth:             true,
			expectedAPIKeyFilePath:       "/path/to/keyfile",
			expectedAPIKeyFromContext:    true,
			expectedAPIKeyReloadInterval: 30 * time.Second,
		},
		{
			name:                         "only API key file provided",
			flags:                        []string{"--es.api-key-file=/path/to/key"},
			expectBasicAuth:              false,
			expectBearerAuth:             false,
			expectAPIKeyAuth:             true,
			expectedAPIKeyFilePath:       "/path/to/key",
			expectedAPIKeyFromContext:    false,
			expectedAPIKeyReloadInterval: 10 * time.Second,
		},
		{
			name:                         "only API key context propagation enabled",
			flags:                        []string{"--es.api-key-allow-from-context=true"},
			expectBasicAuth:              false,
			expectBearerAuth:             false,
			expectAPIKeyAuth:             true,
			expectedAPIKeyFromContext:    true,
			expectedAPIKeyReloadInterval: 10 * time.Second,
		},
		{
			name:                         "both API key file and context enabled",
			flags:                        []string{"--es.api-key-file=/path/to/key", "--es.api-key-allow-from-context=true"},
			expectBasicAuth:              false,
			expectBearerAuth:             false,
			expectAPIKeyAuth:             true,
			expectedAPIKeyFilePath:       "/path/to/key",
			expectedAPIKeyFromContext:    true,
			expectedAPIKeyReloadInterval: 10 * time.Second,
		},
		{
			name: "all API key options provided",
			flags: []string{
				"--es.api-key-file=/path/to/key",
				"--es.api-key-allow-from-context=true",
				"--es.api-key-reload-interval=60s",
			},
			expectBasicAuth:              false,
			expectBearerAuth:             false,
			expectAPIKeyAuth:             true,
			expectedAPIKeyFilePath:       "/path/to/key",
			expectedAPIKeyFromContext:    true,
			expectedAPIKeyReloadInterval: 60 * time.Second,
		},
		{
			name: "basic auth and API key both enabled",
			flags: []string{
				"--es.username=testuser",
				"--es.password=testpass",
				"--es.api-key-file=/path/to/key",
			},
			expectBasicAuth:                true,
			expectBearerAuth:               false,
			expectAPIKeyAuth:               true,
			expectedUsername:               "testuser",
			expectedPassword:               "testpass",
			expectedPasswordReloadInterval: 10 * time.Second,
			expectedAPIKeyFilePath:         "/path/to/key",
			expectedAPIKeyReloadInterval:   10 * time.Second,
		},
		{
			name: "bearer token and API key both enabled",
			flags: []string{
				"--es.token-file=/path/to/token",
				"--es.api-key-allow-from-context=true",
			},
			expectBasicAuth:              false,
			expectBearerAuth:             true,
			expectAPIKeyAuth:             true,
			expectedTokenPath:            "/path/to/token",
			expectedBearerFromContext:    false,
			expectedBearerReloadInterval: 10 * time.Second,
			expectedAPIKeyFromContext:    true,
			expectedAPIKeyReloadInterval: 10 * time.Second,
		},
		{
			name: "basic auth password reload interval disabled",
			flags: []string{
				"--es.username=testuser",
				"--es.password-file=/path/to/password",
				"--es.password-reload-interval=0s",
			},
			expectBasicAuth:                true,
			expectBearerAuth:               false,
			expectAPIKeyAuth:               false,
			expectedUsername:               "testuser",
			expectedPasswordFilePath:       "/path/to/password",
			expectedPasswordReloadInterval: 0 * time.Second,
		},
		{
			name: "bearer token reload interval disabled",
			flags: []string{
				"--es.token-file=/path/to/token",
				"--es.bearer-token-reload-interval=0s",
			},
			expectBasicAuth:              false,
			expectBearerAuth:             true,
			expectAPIKeyAuth:             false,
			expectedTokenPath:            "/path/to/token",
			expectedBearerReloadInterval: 0 * time.Second,
		},
		{
			name: "all three authentication methods enabled",
			flags: []string{
				"--es.username=testuser",
				"--es.password=testpass",
				"--es.token-file=/path/to/token",
				"--es.bearer-token-propagation=true",
				"--es.bearer-token-reload-interval=25s",
				"--es.api-key-file=/path/to/key",
				"--es.api-key-allow-from-context=true",
				"--es.api-key-reload-interval=30s",
			},
			expectBasicAuth:                true,
			expectBearerAuth:               true,
			expectAPIKeyAuth:               true,
			expectedUsername:               "testuser",
			expectedPassword:               "testpass",
			expectedPasswordReloadInterval: 10 * time.Second,
			expectedTokenPath:              "/path/to/token",
			expectedBearerFromContext:      true,
			expectedBearerReloadInterval:   25 * time.Second,
			expectedAPIKeyFilePath:         "/path/to/key",
			expectedAPIKeyFromContext:      true,
			expectedAPIKeyReloadInterval:   30 * time.Second,
		},
		{
			name: "basic auth with custom reload interval (non-zero)",
			flags: []string{
				"--es.username=testuser",
				"--es.password-file=/path/to/password",
				"--es.password-reload-interval=15s",
			},
			expectBasicAuth:                true,
			expectBearerAuth:               false,
			expectAPIKeyAuth:               false,
			expectedUsername:               "testuser",
			expectedPasswordFilePath:       "/path/to/password",
			expectedPasswordReloadInterval: 15 * time.Second,
		},
		{
			name: "bearer token with custom reload interval (non-zero)",
			flags: []string{
				"--es.token-file=/path/to/token",
				"--es.bearer-token-reload-interval=20s",
			},
			expectBasicAuth:              false,
			expectBearerAuth:             true,
			expectAPIKeyAuth:             false,
			expectedTokenPath:            "/path/to/token",
			expectedBearerReloadInterval: 20 * time.Second,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := NewOptions("es")
			v, command := config.Viperize(opts.AddFlags)
			err := command.ParseFlags(tc.flags)
			require.NoError(t, err)
			opts.InitFromViper(v)
			primary := opts.GetConfig()

			// Assert authentication method presence
			assert.Equal(t, tc.expectBasicAuth, primary.Authentication.BasicAuthentication.HasValue())
			assert.Equal(t, tc.expectBearerAuth, primary.Authentication.BearerTokenAuthentication.HasValue())
			assert.Equal(t, tc.expectAPIKeyAuth, primary.Authentication.APIKeyAuthentication.HasValue())

			// Assert basic authentication details
			if tc.expectBasicAuth {
				basicAuth := primary.Authentication.BasicAuthentication.Get()
				assert.Equal(t, tc.expectedUsername, basicAuth.Username)
				assert.Equal(t, tc.expectedPassword, basicAuth.Password)
				assert.Equal(t, tc.expectedPasswordFilePath, basicAuth.PasswordFilePath)
				assert.Equal(t, tc.expectedPasswordReloadInterval, basicAuth.ReloadInterval)
			}

			// Assert bearer token authentication details
			if tc.expectBearerAuth {
				bearerAuth := primary.Authentication.BearerTokenAuthentication.Get()
				assert.Equal(t, tc.expectedTokenPath, bearerAuth.FilePath)
				assert.Equal(t, tc.expectedBearerFromContext, bearerAuth.AllowFromContext)
				assert.Equal(t, tc.expectedBearerReloadInterval, bearerAuth.ReloadInterval)
			}

			// Assert API key authentication details
			if tc.expectAPIKeyAuth {
				apiKeyAuth := primary.Authentication.APIKeyAuthentication.Get()
				assert.Equal(t, tc.expectedAPIKeyFilePath, apiKeyAuth.FilePath)
				assert.Equal(t, tc.expectedAPIKeyFromContext, apiKeyAuth.AllowFromContext)
				assert.Equal(t, tc.expectedAPIKeyReloadInterval, apiKeyAuth.ReloadInterval)
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
	opts := NewOptions("es")
	v, command := config.Viperize(opts.AddFlags)
	err := command.ParseFlags([]string{
		"--es.remote-read-clusters=",
	})
	require.NoError(t, err)
	opts.InitFromViper(v)

	primary := opts.GetConfig()
	assert.Equal(t, []string{}, primary.RemoteReadClusters)
}

func TestMaxSpanAgeSetErrorInArchiveMode(t *testing.T) {
	opts := NewOptions(archiveNamespace)
	_, command := config.Viperize(opts.AddFlags)
	flags := []string{"--es-archive.max-span-age=24h"}
	err := command.ParseFlags(flags)
	require.EqualError(t, err, "unknown flag: --es-archive.max-span-age")
}

func TestMaxDocCount(t *testing.T) {
	testCases := []struct {
		name            string
		flags           []string
		wantMaxDocCount int
	}{
		{"neither defined", []string{}, 10_000},
		{"max-doc-count only", []string{"--es.max-doc-count=1000"}, 1000},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := NewOptions("es")
			v, command := config.Viperize(opts.AddFlags)
			command.ParseFlags(tc.flags)
			opts.InitFromViper(v)

			primary := opts.GetConfig()
			assert.Equal(t, tc.wantMaxDocCount, primary.MaxDocCount)
		})
	}
}

func TestIndexDateSeparator(t *testing.T) {
	testCases := []struct {
		name           string
		flags          []string
		wantDateLayout string
	}{
		{"not defined (default)", []string{}, "2006-01-02"},
		{"empty separator", []string{"--es.index-date-separator="}, "20060102"},
		{"dot separator", []string{"--es.index-date-separator=."}, "2006.01.02"},
		{"crossbar separator", []string{"--es.index-date-separator=-"}, "2006-01-02"},
		{"slash separator", []string{"--es.index-date-separator=/"}, "2006/01/02"},
		{"empty string with single quotes", []string{"--es.index-date-separator=''"}, "2006''01''02"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := NewOptions("es")
			v, command := config.Viperize(opts.AddFlags)
			command.ParseFlags(tc.flags)
			opts.InitFromViper(v)

			primary := opts.GetConfig()
			assert.Equal(t, tc.wantDateLayout, primary.Indices.Spans.DateLayout)
		})
	}
}

func TestIndexRollover(t *testing.T) {
	testCases := []struct {
		name                              string
		flags                             []string
		wantSpanDateLayout                string
		wantServiceDateLayout             string
		wantSpanIndexRolloverFrequency    time.Duration
		wantServiceIndexRolloverFrequency time.Duration
	}{
		{
			name:                              "not defined (default)",
			flags:                             []string{},
			wantSpanDateLayout:                "2006-01-02",
			wantServiceDateLayout:             "2006-01-02",
			wantSpanIndexRolloverFrequency:    -24 * time.Hour,
			wantServiceIndexRolloverFrequency: -24 * time.Hour,
		},
		{
			name:                              "index day rollover",
			flags:                             []string{"--es.index-rollover-frequency-services=day", "--es.index-rollover-frequency-spans=hour"},
			wantSpanDateLayout:                "2006-01-02-15",
			wantServiceDateLayout:             "2006-01-02",
			wantSpanIndexRolloverFrequency:    -1 * time.Hour,
			wantServiceIndexRolloverFrequency: -24 * time.Hour,
		},
		{
			name:                              "index hour rollover",
			flags:                             []string{"--es.index-rollover-frequency-services=hour", "--es.index-rollover-frequency-spans=day"},
			wantSpanDateLayout:                "2006-01-02",
			wantServiceDateLayout:             "2006-01-02-15",
			wantSpanIndexRolloverFrequency:    -24 * time.Hour,
			wantServiceIndexRolloverFrequency: -1 * time.Hour,
		},
		{
			name:                              "invalid index rollover frequency falls back to default 'day'",
			flags:                             []string{"--es.index-rollover-frequency-services=hours", "--es.index-rollover-frequency-spans=hours"},
			wantSpanDateLayout:                "2006-01-02",
			wantServiceDateLayout:             "2006-01-02",
			wantSpanIndexRolloverFrequency:    -24 * time.Hour,
			wantServiceIndexRolloverFrequency: -24 * time.Hour,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := NewOptions("es")
			v, command := config.Viperize(opts.AddFlags)
			command.ParseFlags(tc.flags)
			opts.InitFromViper(v)
			primary := opts.GetConfig()
			assert.Equal(t, tc.wantSpanDateLayout, primary.Indices.Spans.DateLayout)
			assert.Equal(t, tc.wantServiceDateLayout, primary.Indices.Services.DateLayout)
			assert.Equal(t, tc.wantSpanIndexRolloverFrequency, escfg.RolloverFrequencyAsNegativeDuration(primary.Indices.Spans.RolloverFrequency))
			assert.Equal(t, tc.wantServiceIndexRolloverFrequency, escfg.RolloverFrequencyAsNegativeDuration(primary.Indices.Services.RolloverFrequency))
		})
	}
}

func TestAddFlags(t *testing.T) {
	tests := []struct {
		name               string
		setupConfig        func() *namespaceConfig
		expectedUsername   string
		expectedPassword   string
		expectedTokenPath  string
		expectedAPIKeyPath string
	}{
		{
			name: "no authentication",
			setupConfig: func() *namespaceConfig {
				return &namespaceConfig{
					namespace: "es",
					Configuration: escfg.Configuration{
						Servers: []string{"http://localhost:9200"},
					},
				}
			},
			expectedUsername:   "",
			expectedPassword:   "",
			expectedTokenPath:  "",
			expectedAPIKeyPath: "",
		},
		{
			name: "basic authentication",
			setupConfig: func() *namespaceConfig {
				return &namespaceConfig{
					namespace: "es",
					Configuration: escfg.Configuration{
						Servers: []string{"http://localhost:9200"},
						Authentication: escfg.Authentication{
							BasicAuthentication: configoptional.Some(escfg.BasicAuthentication{
								Username:         "testuser",
								Password:         "testpass",
								PasswordFilePath: "/path/to/pass",
							}),
						},
					},
				}
			},
			expectedUsername:   "testuser",
			expectedPassword:   "testpass",
			expectedTokenPath:  "",
			expectedAPIKeyPath: "",
		},
		{
			name: "bearer token authentication",
			setupConfig: func() *namespaceConfig {
				return &namespaceConfig{
					namespace: "es",
					Configuration: escfg.Configuration{
						Servers: []string{"http://localhost:9200"},
						Authentication: escfg.Authentication{
							BearerTokenAuthentication: bearerAuth("/path/to/token", false, 10*time.Second),
						},
					},
				}
			},
			expectedUsername:   "",
			expectedPassword:   "",
			expectedTokenPath:  "/path/to/token",
			expectedAPIKeyPath: "",
		},
		{
			name: "api key authentication",
			setupConfig: func() *namespaceConfig {
				return &namespaceConfig{
					namespace: "es",
					Configuration: escfg.Configuration{
						Servers: []string{"http://localhost:9200"},
						Authentication: escfg.Authentication{
							APIKeyAuthentication: apiKeyAuth("/path/to/apikey", true, 10*time.Second),
						},
					},
				}
			},
			expectedUsername:   "",
			expectedPassword:   "",
			expectedTokenPath:  "",
			expectedAPIKeyPath: "/path/to/apikey",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setupConfig()
			flagSet := flag.NewFlagSet("test", flag.ContinueOnError)
			addFlags(flagSet, cfg)

			// Verify flags were registered with correct default values
			usernameFlag := flagSet.Lookup("es.username")
			require.NotNil(t, usernameFlag, "username flag not registered")
			assert.Equal(t, tt.expectedUsername, usernameFlag.DefValue)

			passwordFlag := flagSet.Lookup("es.password")
			require.NotNil(t, passwordFlag, "password flag not registered")
			assert.Equal(t, tt.expectedPassword, passwordFlag.DefValue)

			tokenFlag := flagSet.Lookup("es.token-file")
			require.NotNil(t, tokenFlag, "token-file flag not registered")
			assert.Equal(t, tt.expectedTokenPath, tokenFlag.DefValue)

			apiKeyFlag := flagSet.Lookup("es.api-key-file")
			require.NotNil(t, apiKeyFlag, "api-key-file flag not registered")
			assert.Equal(t, tt.expectedAPIKeyPath, apiKeyFlag.DefValue)
		})
	}
}

func TestAddFlagsWithPreExistingAuth(t *testing.T) {
	tests := []struct {
		name             string
		setupConfig      func() *namespaceConfig
		expectedDefaults map[string]string
	}{
		{
			name: "existing basic auth with reload interval",
			setupConfig: func() *namespaceConfig {
				return &namespaceConfig{
					namespace: "es",
					Configuration: escfg.Configuration{
						Authentication: escfg.Authentication{
							BasicAuthentication: configoptional.Some(escfg.BasicAuthentication{
								Username:         "existing_user",
								Password:         "existing_pass",
								PasswordFilePath: "/existing/path",
								ReloadInterval:   30 * time.Second,
							}),
						},
					},
				}
			},
			expectedDefaults: map[string]string{
				"es.username":      "existing_user",
				"es.password":      "existing_pass",
				"es.password-file": "/existing/path",
			},
		},
		{
			name: "existing bearer token with reload interval",
			setupConfig: func() *namespaceConfig {
				return &namespaceConfig{
					namespace: "es",
					Configuration: escfg.Configuration{
						Authentication: escfg.Authentication{
							BearerTokenAuthentication: bearerAuth("/existing/token", true, 60*time.Second),
						},
					},
				}
			},
			expectedDefaults: map[string]string{
				"es.token-file": "/existing/token",
			},
		},
		{
			name: "existing api key with reload interval",
			setupConfig: func() *namespaceConfig {
				return &namespaceConfig{
					namespace: "es",
					Configuration: escfg.Configuration{
						Authentication: escfg.Authentication{
							APIKeyAuthentication: apiKeyAuth("/existing/apikey", false, 45*time.Second),
						},
					},
				}
			},
			expectedDefaults: map[string]string{
				"es.api-key-file": "/existing/apikey",
			},
		},
		{
			name: "existing api key with context enabled",
			setupConfig: func() *namespaceConfig {
				return &namespaceConfig{
					namespace: "es",
					Configuration: escfg.Configuration{
						Authentication: escfg.Authentication{
							APIKeyAuthentication: apiKeyAuth("/path/to/key", true, 20*time.Second),
						},
					},
				}
			},
			expectedDefaults: map[string]string{
				"es.api-key-file": "/path/to/key",
			},
		},
		{
			name: "existing API key with disabled reload interval",
			setupConfig: func() *namespaceConfig {
				return &namespaceConfig{
					namespace: "es",
					Configuration: escfg.Configuration{
						Authentication: escfg.Authentication{
							APIKeyAuthentication: apiKeyAuth("/existing/apikey", false, 0*time.Second),
						},
					},
				}
			},
			expectedDefaults: map[string]string{
				"es.api-key-file": "/existing/apikey",
			},
		},
		{
			name: "existing basic auth with disabled password reload",
			setupConfig: func() *namespaceConfig {
				return &namespaceConfig{
					namespace: "es",
					Configuration: escfg.Configuration{
						Authentication: escfg.Authentication{
							BasicAuthentication: configoptional.Some(escfg.BasicAuthentication{
								Username:         "existing_user",
								PasswordFilePath: "/existing/password",
								ReloadInterval:   0 * time.Second,
							}),
						},
					},
				}
			},
			expectedDefaults: map[string]string{
				"es.username":      "existing_user",
				"es.password-file": "/existing/password",
			},
		},
		{
			name: "existing bearer token with disabled reload",
			setupConfig: func() *namespaceConfig {
				return &namespaceConfig{
					namespace: "es",
					Configuration: escfg.Configuration{
						Authentication: escfg.Authentication{
							BearerTokenAuthentication: bearerAuth("/existing/token", true, 0*time.Second),
						},
					},
				}
			},
			expectedDefaults: map[string]string{
				"es.token-file": "/existing/token",
			},
		},
		{
			name: "all authentication methods configured",
			setupConfig: func() *namespaceConfig {
				return &namespaceConfig{
					namespace: "es",
					Configuration: escfg.Configuration{
						Authentication: escfg.Authentication{
							BasicAuthentication:       basicAuth("multi_user", "multi_pass", "/multi/path", 15*time.Second),
							BearerTokenAuthentication: bearerAuth("/multi/token", true, 25*time.Second),
							APIKeyAuthentication:      apiKeyAuth("/multi/apikey", false, 35*time.Second),
						},
					},
				}
			},
			expectedDefaults: map[string]string{
				"es.username":      "multi_user",
				"es.password":      "multi_pass",
				"es.password-file": "/multi/path",
				"es.token-file":    "/multi/token",
				"es.api-key-file":  "/multi/apikey",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setupConfig()
			flagSet := flag.NewFlagSet("test", flag.ContinueOnError)
			addFlags(flagSet, cfg)

			for flagName, expectedDefault := range tt.expectedDefaults {
				flag := flagSet.Lookup(flagName)
				require.NotNil(t, flag, "flag %s not found", flagName)
				assert.Equal(t, expectedDefault, flag.DefValue, "wrong default for %s", flagName)
			}
		})
	}
}
