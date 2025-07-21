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

func getBasicAuthField(opt configoptional.Optional[escfg.BasicAuthentication], field string) string {
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
	}
	return ""
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

	assert.Equal(t, "hello", getBasicAuthField(primary.Authentication.BasicAuthentication, "Username"))
	assert.Equal(t, "world", getBasicAuthField(primary.Authentication.BasicAuthentication, "Password"))
	assert.Equal(t, "/foo/bar", getBearerTokenField(primary.Authentication.BearerTokenAuthentication, "FilePath"))
	assert.Equal(t, true, getBearerTokenField(primary.Authentication.BearerTokenAuthentication, "AllowFromContext"))

	assert.Equal(t, "/foo/bar/baz", getBasicAuthField(primary.Authentication.BasicAuthentication, "PasswordFilePath"))

	assert.Equal(t, []string{"1.1.1.1", "2.2.2.2"}, primary.Servers)
	assert.Equal(t, []string{"cluster_one", "cluster_two"}, primary.RemoteReadClusters)
	assert.Equal(t, 48*time.Hour, primary.MaxSpanAge)
	assert.True(t, primary.Sniffing.Enabled)
	assert.True(t, primary.Sniffing.UseHTTPS)
	assert.True(t, primary.DisableHealthCheck)
	assert.False(t, primary.TLS.Insecure)
	assert.True(t, primary.TLS.InsecureSkipVerify)
	assert.True(t, primary.Tags.AllAsFields)
	assert.Equal(t, "!", primary.Tags.DotReplacement)
	assert.Equal(t, "./file.txt", primary.Tags.File)
	assert.Equal(t, "test,tags", primary.Tags.Include)
	assert.Equal(t, "20060102", primary.Indices.Services.DateLayout)
	assert.Equal(t, "2006010215", primary.Indices.Spans.DateLayout)
	assert.True(t, primary.UseILM)
	assert.True(t, primary.HTTPCompression)
}

func TestAuthenticationConditionalCreation(t *testing.T) {
	testCases := []struct {
		name              string
		flags             []string
		expectBasicAuth   bool
		expectBearerAuth  bool
		expectedUsername  string
		expectedPassword  string
		expectedTokenPath string
	}{
		{
			name:             "no authentication flags",
			flags:            []string{},
			expectBasicAuth:  false,
			expectBearerAuth: false,
		},
		{
			name:             "only username provided",
			flags:            []string{"--es.username=testuser"},
			expectBasicAuth:  true,
			expectBearerAuth: false,
			expectedUsername: "testuser",
		},
		{
			name:             "only password provided",
			flags:            []string{"--es.password=testpass"},
			expectBasicAuth:  true,
			expectBearerAuth: false,
			expectedPassword: "testpass",
		},
		{
			name:              "only token file provided",
			flags:             []string{"--es.token-file=/path/to/token"},
			expectBasicAuth:   false,
			expectBearerAuth:  true,
			expectedTokenPath: "/path/to/token",
		},
		{
			name:             "username and password provided",
			flags:            []string{"--es.username=testuser", "--es.password=testpass"},
			expectBasicAuth:  true,
			expectBearerAuth: false,
			expectedUsername: "testuser",
			expectedPassword: "testpass",
		},
		{
			name:             "only bearer token context propagation enabled",
			flags:            []string{"--es.bearer-token-propagation=true"},
			expectBasicAuth:  false,
			expectBearerAuth: true,
		},
		{
			name:              "both token file and context propagation enabled",
			flags:             []string{"--es.token-file=/path/to/token", "--es.bearer-token-propagation=true"},
			expectBasicAuth:   false,
			expectBearerAuth:  true,
			expectedTokenPath: "/path/to/token",
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

			assert.Equal(t, tc.expectBasicAuth, primary.Authentication.BasicAuthentication.HasValue())
			assert.Equal(t, tc.expectBearerAuth, primary.Authentication.BearerTokenAuthentication.HasValue())

			if tc.expectBasicAuth {
				assert.Equal(t, tc.expectedUsername, getBasicAuthField(primary.Authentication.BasicAuthentication, "Username"))
				assert.Equal(t, tc.expectedPassword, getBasicAuthField(primary.Authentication.BasicAuthentication, "Password"))
			}

			if tc.expectBearerAuth {
				assert.Equal(t, tc.expectedTokenPath, getBearerTokenField(primary.Authentication.BearerTokenAuthentication, "FilePath"))
			}
		})
	}
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
		name              string
		setupConfig       func() *namespaceConfig
		expectedUsername  string
		expectedPassword  string
		expectedTokenPath string
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
			expectedUsername:  "",
			expectedPassword:  "",
			expectedTokenPath: "",
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
			expectedUsername:  "testuser",
			expectedPassword:  "testpass",
			expectedTokenPath: "",
		},
		{
			name: "bearer token authentication",
			setupConfig: func() *namespaceConfig {
				return &namespaceConfig{
					namespace: "es",
					Configuration: escfg.Configuration{
						Servers: []string{"http://localhost:9200"},
						Authentication: escfg.Authentication{
							BearerTokenAuthentication: configoptional.Some(escfg.BearerTokenAuthentication{
								FilePath: "/path/to/token",
							}),
						},
					},
				}
			},
			expectedUsername:  "",
			expectedPassword:  "",
			expectedTokenPath: "/path/to/token",
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
		})
	}
}
