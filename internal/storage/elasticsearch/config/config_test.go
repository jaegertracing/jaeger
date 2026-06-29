// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

// basicAuth creates basic authentication component
func basicAuth(username, password, passwordFilePath string) configoptional.Optional[BasicAuthentication] {
	return configoptional.Some(BasicAuthentication{
		Username:         username,
		Password:         password,
		PasswordFilePath: passwordFilePath,
	})
}

func TestApplyDefaults(t *testing.T) {
	source := &Configuration{
		RemoteReadClusters: []string{"cluster1", "cluster2"},
		Authentication: Authentication{
			BasicAuthentication: basicAuth("sourceUser", "sourcePass", ""),
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
				Replicas: new(int64(1)),
				Priority: 10,
			},
			Services: IndexOptions{
				Shards:   5,
				Replicas: new(int64(1)),
				Priority: 20,
			},
			Dependencies: IndexOptions{
				Shards:   5,
				Replicas: new(int64(1)),
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
			name: "Some Defaults Applied",
			target: &Configuration{
				RemoteReadClusters: []string{"customCluster"},
				Authentication: Authentication{
					BasicAuthentication: basicAuth("customUser", "", ""),
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
					BasicAuthentication: basicAuth("customUser", "sourcePass", ""),
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
						Replicas: new(int64(1)),
						Priority: 10,
					},
					Services: IndexOptions{
						Shards:   5,
						Replicas: new(int64(1)),
						Priority: 20,
					},
					Dependencies: IndexOptions{
						Shards:   5,
						Replicas: new(int64(1)),
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
					BasicAuthentication: basicAuth("sourceUser", "sourcePass", ""),
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
						Replicas: new(int64(1)),
						Priority: 10,
					},
					Services: IndexOptions{
						Shards:   5,
						Replicas: new(int64(1)),
						Priority: 20,
					},
					Dependencies: IndexOptions{
						Shards:   5,
						Replicas: new(int64(1)),
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
			expected: source,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.target.ApplyDefaults(source)
			require.Equal(t, test.expected, test.target)
		})
	}
}

func TestApplyDefaults_Auth(t *testing.T) {
	source := &Configuration{
		Authentication: Authentication{
			BasicAuthentication: basicAuth("sourceUser", "sourcePass", ""),
		},
	}

	target := &Configuration{
		Authentication: Authentication{
			BasicAuthentication: basicAuth("", "", ""),
		},
	}

	expected := &Configuration{
		Authentication: Authentication{
			BasicAuthentication: basicAuth("sourceUser", "sourcePass", ""),
		},
	}

	target.ApplyDefaults(source)
	require.Equal(t, expected, target)
}

func TestTagKeysAsFields(t *testing.T) {
	const (
		pwd1 = "tag1\ntag2"
	)

	pwdFile := filepath.Join(t.TempDir(), "pwd")
	require.NoError(t, os.WriteFile(pwdFile, []byte(pwd1), 0o600))
	tests := []struct {
		name         string
		config       *Configuration
		expectedTags []string
		expectError  bool
	}{
		{
			name: "File with tags",
			config: &Configuration{
				Tags: TagsAsFields{
					File:    pwdFile,
					Include: "",
				},
			},
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
					File:    pwdFile,
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
			config:        &Configuration{Servers: []string{"localhost:8000/dummyserver"}, UseILM: configoptional.Some(true)},
			expectedError: "UseILM must always be used in conjunction with UseReadWriteAliases to ensure ES writers and readers refer to the single index mapping",
		},
		{
			name:          "ilm and create templates enabled",
			config:        &Configuration{Servers: []string{"localhost:8000/dummyserver"}, UseILM: configoptional.Some(true), CreateIndexTemplates: true, UseReadWriteAliases: configoptional.Some(true)},
			expectedError: "when UseILM is set true, CreateIndexTemplates must be set to false and index templates must be created by init process of es-rollover app",
		},
		{
			name: "explicit span aliases without UseReadWriteAliases",
			config: &Configuration{
				Servers:        []string{"localhost:8000/dummyserver"},
				SpanReadAlias:  configoptional.Some("custom-span-read"),
				SpanWriteAlias: configoptional.Some("custom-span-write"),
			},
			expectedError: "explicit aliases (span_read_alias, span_write_alias, service_read_alias, service_write_alias) require UseReadWriteAliases to be true",
		},
		{
			name: "only span read alias set",
			config: &Configuration{
				Servers:             []string{"localhost:8000/dummyserver"},
				UseReadWriteAliases: configoptional.Some(true),
				SpanReadAlias:       configoptional.Some("custom-span-read"),
			},
			expectedError: "both span_read_alias and span_write_alias must be set together",
		},
		{
			name: "only service write alias set",
			config: &Configuration{
				Servers:             []string{"localhost:8000/dummyserver"},
				UseReadWriteAliases: configoptional.Some(true),
				ServiceWriteAlias:   configoptional.Some("custom-service-write"),
			},
			expectedError: "both service_read_alias and service_write_alias must be set together",
		},
		{
			name: "all explicit aliases with UseReadWriteAliases is valid",
			config: &Configuration{
				Servers:             []string{"localhost:8000/dummyserver"},
				UseReadWriteAliases: configoptional.Some(true),
				SpanReadAlias:       configoptional.Some("custom-span-read"),
				SpanWriteAlias:      configoptional.Some("custom-span-write"),
				ServiceReadAlias:    configoptional.Some("custom-service-read"),
				ServiceWriteAlias:   configoptional.Some("custom-service-write"),
			},
		},
		{
			name: "only span aliases with UseReadWriteAliases is valid",
			config: &Configuration{
				Servers:             []string{"localhost:8000/dummyserver"},
				UseReadWriteAliases: configoptional.Some(true),
				SpanReadAlias:       configoptional.Some("custom-span-read"),
				SpanWriteAlias:      configoptional.Some("custom-span-write"),
			},
		},
		{
			name: "explicit aliases with IndexPrefix is valid",
			config: &Configuration{
				Servers:             []string{"localhost:8000/dummyserver"},
				UseReadWriteAliases: configoptional.Some(true),
				SpanReadAlias:       configoptional.Some("custom-span-read"),
				SpanWriteAlias:      configoptional.Some("custom-span-write"),
				ServiceReadAlias:    configoptional.Some("custom-service-read"),
				ServiceWriteAlias:   configoptional.Some("custom-service-write"),
				Indices: Indices{
					IndexPrefix: "prod",
				},
			},
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

func TestIndexPrefixDataStreamName(t *testing.T) {
	tests := []struct {
		prefix   IndexPrefix
		expected string
	}{
		{"", "jaeger.spans"},
		{"prod", "prod.jaeger.spans"},
		{"prod-", "prod.jaeger.spans"},
		{"prod.", "prod.jaeger.spans"},
		{"my-team", "my-team.jaeger.spans"},
	}
	for _, test := range tests {
		t.Run(string(test.prefix), func(t *testing.T) {
			require.Equal(t, test.expected, test.prefix.DataStreamName("jaeger.spans"))
		})
	}
}

func TestCustomHeaders(t *testing.T) {
	tests := []struct {
		name     string
		config   Configuration
		expected map[string]string
	}{
		{
			name: "custom headers are set correctly",
			config: Configuration{
				Servers: []string{"http://localhost:9200"},
				CustomHeaders: map[string]string{
					"Host":            "my-opensearch.amazonaws.com",
					"X-Custom-Header": "test-value",
				},
			},
			expected: map[string]string{
				"Host":            "my-opensearch.amazonaws.com",
				"X-Custom-Header": "test-value",
			},
		},
		{
			name: "empty custom headers",
			config: Configuration{
				Servers:       []string{"http://localhost:9200"},
				CustomHeaders: map[string]string{},
			},
			expected: map[string]string{},
		},
		{
			name: "nil custom headers",
			config: Configuration{
				Servers:       []string{"http://localhost:9200"},
				CustomHeaders: nil,
			},
			expected: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.expected == nil {
				assert.Nil(t, test.config.CustomHeaders)
			} else {
				assert.Equal(t, test.expected, test.config.CustomHeaders)
			}
		})
	}
}

func TestApplyDefaultsCustomHeaders(t *testing.T) {
	source := &Configuration{
		CustomHeaders: map[string]string{
			"Host":            "source-host",
			"X-Custom-Header": "source-value",
		},
	}

	tests := []struct {
		name     string
		target   *Configuration
		expected map[string]string
	}{
		{
			name:   "target has no headers, apply from source",
			target: &Configuration{},
			expected: map[string]string{
				"Host":            "source-host",
				"X-Custom-Header": "source-value",
			},
		},
		{
			name: "target has headers, keep target headers",
			target: &Configuration{
				CustomHeaders: map[string]string{
					"Host": "target-host",
				},
			},
			expected: map[string]string{
				"Host": "target-host",
			},
		},
		{
			name: "target has empty map, keep empty",
			target: &Configuration{
				CustomHeaders: map[string]string{},
			},
			expected: map[string]string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.target.ApplyDefaults(source)
			assert.Equal(t, test.expected, test.target.CustomHeaders)
		})
	}
}

func TestRotationConfig_HasRotation(t *testing.T) {
	tests := []struct {
		name     string
		rotation RotationConfig
		expected bool
	}{
		{
			name:     "empty",
			rotation: RotationConfig{},
			expected: false,
		},
		{
			name: "periodic set",
			rotation: RotationConfig{
				Periodic: configoptional.Some(PeriodicRotation{DateLayout: "2006-01-02"}),
			},
			expected: true,
		},
		{
			name: "manual_rollover set",
			rotation: RotationConfig{
				ManualRollover: configoptional.Some(ManualRolloverRotation{
					ReadAlias:  "read",
					WriteAlias: "write",
				}),
			},
			expected: true,
		},
		{
			name: "data_stream set",
			rotation: RotationConfig{
				DataStream: configoptional.Some(DataStreamRotation{PolicyName: "test"}),
			},
			expected: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.rotation.HasRotation())
		})
	}
}

func TestRotationConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		rotation    RotationConfig
		indexType   string
		expectedErr string
	}{
		{
			name:      "empty is valid",
			rotation:  RotationConfig{},
			indexType: "spans",
		},
		{
			name: "single periodic is valid",
			rotation: RotationConfig{
				Periodic: configoptional.Some(PeriodicRotation{DateLayout: "2006-01-02"}),
			},
			indexType: "spans",
		},
		{
			name: "data_stream is valid for spans",
			rotation: RotationConfig{
				DataStream: configoptional.Some(DataStreamRotation{PolicyName: "test"}),
			},
			indexType: "spans",
		},
		{
			name: "data_stream is rejected for services",
			rotation: RotationConfig{
				DataStream: configoptional.Some(DataStreamRotation{PolicyName: "test"}),
			},
			indexType:   "services",
			expectedErr: "data_stream rotation is only supported for the spans index",
		},
		{
			name: "data_stream is rejected for dependencies",
			rotation: RotationConfig{
				DataStream: configoptional.Some(DataStreamRotation{PolicyName: "test"}),
			},
			indexType:   "dependencies",
			expectedErr: "data_stream rotation is only supported for the spans index",
		},
		{
			name: "multiple rotations set",
			rotation: RotationConfig{
				Periodic:   configoptional.Some(PeriodicRotation{DateLayout: "2006-01-02"}),
				DataStream: configoptional.Some(DataStreamRotation{PolicyName: "test"}),
			},
			indexType:   "spans",
			expectedErr: "exactly one rotation strategy must be set, found 2",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rotation.validate(tc.indexType)
			if tc.expectedErr != "" {
				require.ErrorContains(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidate_RotationConflictsWithLegacyFlags(t *testing.T) {
	cfg := &Configuration{
		Servers: []string{"localhost:8000/dummyserver"},
		Indices: Indices{
			Spans: IndexOptions{
				Rotation: RotationConfig{
					Periodic: configoptional.Some(PeriodicRotation{DateLayout: "2006-01-02"}),
				},
			},
		},
		UseReadWriteAliases: configoptional.Some(true),
	}
	err := cfg.Validate()
	require.ErrorContains(t, err, "cannot use both 'rotation' config and legacy flags")
}

func TestLogDeprecationWarnings(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *Configuration
		expectedLogs []string
	}{
		{
			name:         "no legacy flags",
			cfg:          &Configuration{},
			expectedLogs: nil,
		},
		{
			name: "use_aliases flag",
			cfg: &Configuration{
				UseReadWriteAliases: configoptional.Some(true),
			},
			expectedLogs: []string{
				"Deprecated Elasticsearch configuration flag",
				"use_aliases",
				"manual_rollover",
			},
		},
		{
			name: "use_ilm flag",
			cfg: &Configuration{
				UseILM: configoptional.Some(true),
			},
			expectedLogs: []string{
				"Deprecated Elasticsearch configuration flag",
				"use_ilm",
				"auto_rollover",
			},
		},
		{
			name: "multiple flags",
			cfg: &Configuration{
				UseReadWriteAliases: configoptional.Some(true),
				SpanReadAlias:       configoptional.Some("custom"),
			},
			expectedLogs: []string{
				"use_aliases",
				"span_read_alias",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger, buf := testutils.NewLogger()
			tc.cfg.LogDeprecationWarnings(logger)
			logOutput := buf.String()
			for _, expected := range tc.expectedLogs {
				assert.Contains(t, logOutput, expected)
			}
			if len(tc.expectedLogs) == 0 {
				assert.Empty(t, logOutput)
			}
		})
	}
}

func TestConfiguration_HasAnyLegacyRotationFlags(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *Configuration
		expected bool
	}{
		{
			name:     "no legacy flags",
			cfg:      &Configuration{},
			expected: false,
		},
		{
			name: "use_aliases set",
			cfg: &Configuration{
				UseReadWriteAliases: configoptional.Some(true),
			},
			expected: true,
		},
		{
			name: "multiple legacy flags",
			cfg: &Configuration{
				UseReadWriteAliases: configoptional.Some(true),
				UseILM:              configoptional.Some(true),
				SpanReadAlias:       configoptional.Some("test"),
			},
			expected: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.cfg.hasAnyLegacyRotationFlags())
		})
	}
}

func TestValidate_RejectLegacyRotationFlagsGate(t *testing.T) {
	require.NoError(t, featuregate.GlobalRegistry().Set(RejectLegacyRotationFlags.ID(), true))
	t.Cleanup(func() {
		require.NoError(t, featuregate.GlobalRegistry().Set(RejectLegacyRotationFlags.ID(), false))
	})

	cfg := &Configuration{
		Servers:             []string{"localhost:8000/dummyserver"},
		UseReadWriteAliases: configoptional.Some(true),
	}
	err := cfg.Validate()
	require.ErrorContains(t, err, "deprecated ES rotation flags")
	require.ErrorContains(t, err, "no longer supported")
}

func TestValidate_LegacyFlagsAllowedWhenGateDisabled(t *testing.T) {
	require.NoError(t, featuregate.GlobalRegistry().Set(RejectLegacyRotationFlags.ID(), false))

	cfg := &Configuration{
		Servers:              []string{"localhost:8000/dummyserver"},
		UseReadWriteAliases:  configoptional.Some(true),
		UseILM:               configoptional.Some(true),
		CreateIndexTemplates: false,
	}
	err := cfg.Validate()
	require.NoError(t, err)
}

func TestResolvedRotation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Configuration
		checkFn func(t *testing.T, rc RotationConfig)
	}{
		{
			name: "default periodic when no flags set",
			cfg:  &Configuration{},
			checkFn: func(t *testing.T, rc RotationConfig) {
				assert.True(t, rc.Periodic.HasValue())
				assert.Equal(t, "2006-01-02", rc.Periodic.Get().DateLayout)
			},
		},
		{
			name: "explicit rotation takes precedence",
			cfg: &Configuration{
				Indices: Indices{
					Spans: IndexOptions{
						Rotation: RotationConfig{
							ManualRollover: configoptional.Some(ManualRolloverRotation{
								ReadAlias:  "custom-read",
								WriteAlias: "custom-write",
							}),
						},
					},
				},
			},
			checkFn: func(t *testing.T, rc RotationConfig) {
				assert.True(t, rc.ManualRollover.HasValue())
				assert.Equal(t, "custom-read", rc.ManualRollover.Get().ReadAlias)
				assert.Equal(t, "custom-write", rc.ManualRollover.Get().WriteAlias)
			},
		},
		{
			name: "use_aliases resolves to manual_rollover",
			cfg: &Configuration{
				UseReadWriteAliases: configoptional.Some(true),
			},
			checkFn: func(t *testing.T, rc RotationConfig) {
				assert.True(t, rc.ManualRollover.HasValue())
				assert.Equal(t, "jaeger-span-read", rc.ManualRollover.Get().ReadAlias)
				assert.Equal(t, "jaeger-span-write", rc.ManualRollover.Get().WriteAlias)
			},
		},
		{
			name: "use_ilm resolves to auto_rollover",
			cfg: &Configuration{
				UseReadWriteAliases: configoptional.Some(true),
				UseILM:              configoptional.Some(true),
			},
			checkFn: func(t *testing.T, rc RotationConfig) {
				assert.True(t, rc.AutoRollover.HasValue())
				assert.Equal(t, "jaeger-span-read", rc.AutoRollover.Get().ReadAlias)
				assert.Equal(t, "jaeger-span-write", rc.AutoRollover.Get().WriteAlias)
			},
		},
		{
			name: "use_ilm with explicit aliases resolves to auto_rollover with those aliases",
			cfg: &Configuration{
				UseReadWriteAliases: configoptional.Some(true),
				UseILM:              configoptional.Some(true),
				SpanReadAlias:       configoptional.Some("my-read"),
				SpanWriteAlias:      configoptional.Some("my-write"),
			},
			checkFn: func(t *testing.T, rc RotationConfig) {
				assert.True(t, rc.AutoRollover.HasValue())
				assert.Equal(t, "my-read", rc.AutoRollover.Get().ReadAlias)
				assert.Equal(t, "my-write", rc.AutoRollover.Get().WriteAlias)
			},
		},
		{
			name: "explicit aliases without use_ilm resolves to manual_rollover",
			cfg: &Configuration{
				UseReadWriteAliases: configoptional.Some(true),
				SpanReadAlias:       configoptional.Some("my-read"),
				SpanWriteAlias:      configoptional.Some("my-write"),
			},
			checkFn: func(t *testing.T, rc RotationConfig) {
				assert.True(t, rc.ManualRollover.HasValue())
				assert.Equal(t, "my-read", rc.ManualRollover.Get().ReadAlias)
				assert.Equal(t, "my-write", rc.ManualRollover.Get().WriteAlias)
			},
		},
		{
			name: "custom alias suffixes",
			cfg: &Configuration{
				UseReadWriteAliases: configoptional.Some(true),
				ReadAliasSuffix:     "reader",
				WriteAliasSuffix:    "writer",
			},
			checkFn: func(t *testing.T, rc RotationConfig) {
				assert.True(t, rc.ManualRollover.HasValue())
				assert.Equal(t, "jaeger-span-reader", rc.ManualRollover.Get().ReadAlias)
				assert.Equal(t, "jaeger-span-writer", rc.ManualRollover.Get().WriteAlias)
			},
		},
		{
			name: "custom date_layout in legacy field",
			cfg: &Configuration{
				Indices: Indices{
					Spans: IndexOptions{
						DateLayout: configoptional.Some("2006010215"),
					},
				},
			},
			checkFn: func(t *testing.T, rc RotationConfig) {
				assert.True(t, rc.Periodic.HasValue())
				assert.Equal(t, "2006010215", rc.Periodic.Get().DateLayout)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rc := tc.cfg.ResolvedSpanRotation()
			tc.checkFn(t, rc)
		})
	}
}

func TestResolvedServiceRotation(t *testing.T) {
	cfg := &Configuration{
		UseReadWriteAliases: configoptional.Some(true),
		ServiceReadAlias:    configoptional.Some("svc-read"),
		ServiceWriteAlias:   configoptional.Some("svc-write"),
	}
	rc := cfg.ResolvedServiceRotation()
	assert.True(t, rc.ManualRollover.HasValue())
	assert.Equal(t, "svc-read", rc.ManualRollover.Get().ReadAlias)
	assert.Equal(t, "svc-write", rc.ManualRollover.Get().WriteAlias)
}

func TestResolvedDependencyRotation(t *testing.T) {
	cfg := &Configuration{
		UseReadWriteAliases: configoptional.Some(true),
	}
	rc := cfg.ResolvedDependencyRotation()
	assert.True(t, rc.ManualRollover.HasValue())
	assert.Equal(t, "jaeger-dependencies-read", rc.ManualRollover.Get().ReadAlias)
}

func TestResolvedSamplingRotation(t *testing.T) {
	cfg := &Configuration{}
	rc := cfg.ResolvedSamplingRotation()
	assert.True(t, rc.Periodic.HasValue())
	assert.Equal(t, "2006-01-02", rc.Periodic.Get().DateLayout)
}

func TestResolvedRotation_UseILMWithCustomSuffixes(t *testing.T) {
	cfg := &Configuration{
		UseReadWriteAliases: configoptional.Some(true),
		UseILM:              configoptional.Some(true),
		ReadAliasSuffix:     "reader",
		WriteAliasSuffix:    "writer",
	}
	rc := cfg.ResolvedSpanRotation()
	assert.True(t, rc.AutoRollover.HasValue())
	assert.Equal(t, "jaeger-span-reader", rc.AutoRollover.Get().ReadAlias)
	assert.Equal(t, "jaeger-span-writer", rc.AutoRollover.Get().WriteAlias)
}

func TestValidateRotationConfig_DateLayoutConflict(t *testing.T) {
	cfg := &Configuration{
		Servers: []string{"localhost:8000/dummyserver"},
		Indices: Indices{
			Spans: IndexOptions{
				DateLayout: configoptional.Some("2006010215"),
				Rotation: RotationConfig{
					Periodic: configoptional.Some(PeriodicRotation{DateLayout: "2006-01-02"}),
				},
			},
		},
	}
	err := cfg.Validate()
	require.ErrorContains(t, err, "cannot use both 'rotation' config and legacy")
	require.ErrorContains(t, err, "date_layout")
}

func TestValidateRotationConfig_RolloverFrequencyConflict(t *testing.T) {
	cfg := &Configuration{
		Servers: []string{"localhost:8000/dummyserver"},
		Indices: Indices{
			Spans: IndexOptions{
				RolloverFrequency: configoptional.Some("hour"),
				Rotation: RotationConfig{
					Periodic: configoptional.Some(PeriodicRotation{DateLayout: "2006-01-02"}),
				},
			},
		},
	}
	err := cfg.Validate()
	require.ErrorContains(t, err, "cannot use both 'rotation' config and legacy")
	require.ErrorContains(t, err, "rollover_frequency")
}

func TestRotationConfig_ValidateAutoRollover(t *testing.T) {
	rc := RotationConfig{
		AutoRollover: configoptional.Some(AutoRolloverRotation{
			ReadAlias:  "read",
			WriteAlias: "write",
		}),
	}
	require.NoError(t, rc.validate("spans"))
}

func TestRotationConfig_ValidateMultipleWithAutoRollover(t *testing.T) {
	rc := RotationConfig{
		AutoRollover: configoptional.Some(AutoRolloverRotation{}),
		Periodic:     configoptional.Some(PeriodicRotation{}),
	}
	err := rc.validate("spans")
	require.ErrorContains(t, err, "exactly one rotation strategy must be set, found 2")
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
