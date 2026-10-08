// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/confmap"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		// mutate changes DefaultConfiguration() into the case under test.
		mutate  func(cfg *Configuration)
		wantErr string
	}{
		{
			name:   "defaults with native protocol",
			mutate: func(cfg *Configuration) { cfg.Protocol = "native" },
		},
		{
			name:   "http protocol",
			mutate: func(cfg *Configuration) { cfg.Protocol = "http" },
		},
		{
			name:   "empty protocol",
			mutate: func(cfg *Configuration) { cfg.Protocol = "" },
		},
		{
			name: "multiple addresses",
			mutate: func(cfg *Configuration) {
				cfg.Addresses = []string{"localhost:9000", "localhost:9001"}
			},
		},
		{
			name: "caching disabled and entries kept forever",
			mutate: func(cfg *Configuration) {
				cfg.AttributeMetadataCacheMaxSize = 0
				cfg.AttributeMetadataCacheTTL = 0
			},
		},
		{
			name:    "unsupported protocol",
			mutate:  func(cfg *Configuration) { cfg.Protocol = "grpc" },
			wantErr: "Protocol",
		},
		{
			name:    "empty addresses",
			mutate:  func(cfg *Configuration) { cfg.Addresses = []string{} },
			wantErr: "Addresses",
		},
		{
			name:    "nil addresses",
			mutate:  func(cfg *Configuration) { cfg.Addresses = nil },
			wantErr: "Addresses",
		},
		{
			name:    "zero default search depth",
			mutate:  func(cfg *Configuration) { cfg.DefaultSearchDepth = 0 },
			wantErr: "default_search_depth must be a positive number",
		},
		{
			name:    "zero max search depth",
			mutate:  func(cfg *Configuration) { cfg.MaxSearchDepth = 0 },
			wantErr: "max_search_depth must be a positive number",
		},
		{
			name: "default search depth exceeds max search depth",
			mutate: func(cfg *Configuration) {
				cfg.DefaultSearchDepth = 15000
				cfg.MaxSearchDepth = 10000
			},
			wantErr: "default_search_depth cannot exceed max_search_depth",
		},
		{
			name: "default search depth equals max search depth",
			mutate: func(cfg *Configuration) {
				cfg.DefaultSearchDepth = 10000
				cfg.MaxSearchDepth = 10000
			},
		},
		{
			name:    "negative dial timeout",
			mutate:  func(cfg *Configuration) { cfg.DialTimeout = -time.Second },
			wantErr: "dial_timeout must be a non-negative duration",
		},
		{
			name:   "positive dial timeout",
			mutate: func(cfg *Configuration) { cfg.DialTimeout = 5 * time.Second },
		},
		{
			name:    "negative attribute metadata cache TTL",
			mutate:  func(cfg *Configuration) { cfg.AttributeMetadataCacheTTL = -time.Second },
			wantErr: "attribute_metadata_cache_ttl must be a non-negative duration",
		},
		{
			name: "create_schema without table_engine",
			mutate: func(cfg *Configuration) {
				cfg.CreateSchema = true
			},
			wantErr: "create_schema requires table_engine with exactly one of merge_tree or replicated",
		},
		{
			name: "create_schema with merge_tree",
			mutate: func(cfg *Configuration) {
				cfg.CreateSchema = true
				cfg.TableEngine.MergeTree = configoptional.Some(MergeTreeEngine{})
			},
		},
		{
			name: "create_schema with replicated and server defaults",
			mutate: func(cfg *Configuration) {
				cfg.CreateSchema = true
				cfg.TableEngine.Replicated = configoptional.Some(ReplicatedEngine{})
			},
		},
		{
			name: "create_schema with replicated keeper path and replica name",
			mutate: func(cfg *Configuration) {
				cfg.CreateSchema = true
				cfg.TableEngine.Replicated = configoptional.Some(ReplicatedEngine{
					KeeperPath:  "/clickhouse/tables/{shard}/{database}/{table}",
					ReplicaName: "{replica}",
				})
			},
		},
		{
			name: "table_engine not validated without create_schema",
			mutate: func(cfg *Configuration) {
				cfg.CreateSchema = false
				cfg.TableEngine.MergeTree = configoptional.Some(MergeTreeEngine{})
				cfg.TableEngine.Replicated = configoptional.Some(ReplicatedEngine{KeeperPath: "it's"})
			},
		},
		{
			name: "both table_engine variants",
			mutate: func(cfg *Configuration) {
				cfg.CreateSchema = true
				cfg.TableEngine.MergeTree = configoptional.Some(MergeTreeEngine{})
				cfg.TableEngine.Replicated = configoptional.Some(ReplicatedEngine{})
			},
			wantErr: "table_engine must set only one of merge_tree or replicated",
		},
		{
			name: "replicated keeper path without replica name",
			mutate: func(cfg *Configuration) {
				cfg.CreateSchema = true
				cfg.TableEngine.Replicated = configoptional.Some(ReplicatedEngine{
					KeeperPath: "/clickhouse/tables/{shard}/{database}/{table}",
				})
			},
			wantErr: "keeper_path and replica_name together or neither",
		},
		{
			name: "replicated keeper path with a quote",
			mutate: func(cfg *Configuration) {
				cfg.CreateSchema = true
				cfg.TableEngine.Replicated = configoptional.Some(ReplicatedEngine{
					KeeperPath:  "/clickhouse/tables/it's",
					ReplicaName: "{replica}",
				})
			},
			wantErr: "must not contain a single quote",
		},
		{
			name:    "negative attribute metadata cache size",
			mutate:  func(cfg *Configuration) { cfg.AttributeMetadataCacheMaxSize = -1 },
			wantErr: "attribute_metadata_cache_max_size must be a non-negative number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfiguration()
			cfg.Addresses = []string{"localhost:9000"}
			tt.mutate(&cfg)

			err := cfg.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

// TestTableEngine_Unmarshal checks that each variant is selected by its key the way the
// collector's confmap unmarshals an optional block, including the empty merge_tree: {},
// and that the two likely mistakes, an empty block and a misspelled key, are rejected.
func TestTableEngine_Unmarshal(t *testing.T) {
	tests := []struct {
		name             string
		yaml             map[string]any
		want             TableEngine
		wantUnmarshalErr string
		wantValidateErr  string
	}{
		{
			name: "merge_tree",
			yaml: map[string]any{"merge_tree": map[string]any{}},
			want: TableEngine{MergeTree: configoptional.Some(MergeTreeEngine{})},
		},
		{
			name: "replicated with server defaults",
			yaml: map[string]any{"replicated": map[string]any{}},
			want: TableEngine{Replicated: configoptional.Some(ReplicatedEngine{})},
		},
		{
			name: "replicated with arguments",
			yaml: map[string]any{"replicated": map[string]any{
				"keeper_path":  "/clickhouse/tables/{shard}/{database}/{table}",
				"replica_name": "{replica}",
			}},
			want: TableEngine{Replicated: configoptional.Some(ReplicatedEngine{
				KeeperPath:  "/clickhouse/tables/{shard}/{database}/{table}",
				ReplicaName: "{replica}",
			})},
		},
		{
			name:            "empty block",
			yaml:            map[string]any{},
			wantValidateErr: "create_schema requires table_engine",
		},
		{
			name:             "unknown variant",
			yaml:             map[string]any{"mergetree": map[string]any{}},
			wantUnmarshalErr: "mergetree",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfiguration()
			conf := confmap.NewFromStringMap(map[string]any{
				"addresses":     []any{"localhost:9000"},
				"create_schema": true,
				"table_engine":  tt.yaml,
			})
			err := conf.Unmarshal(&cfg)
			if tt.wantUnmarshalErr != "" {
				require.ErrorContains(t, err, tt.wantUnmarshalErr)
				return
			}
			require.NoError(t, err)
			require.True(t, cfg.CreateSchema)
			require.Equal(t, tt.want, cfg.TableEngine)
			err = cfg.Validate()
			if tt.wantValidateErr != "" {
				require.ErrorContains(t, err, tt.wantValidateErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDefaultConfiguration(t *testing.T) {
	cfg := DefaultConfiguration()

	require.Equal(t, defaultProtocol, cfg.Protocol)
	require.Equal(t, defaultDatabase, cfg.Database)
	require.EqualValues(t, defaultSearchDepth, cfg.DefaultSearchDepth)
	require.EqualValues(t, defaultMaxSearchDepth, cfg.MaxSearchDepth)
	require.Equal(t, defaultAttributeMetadataCacheTTL, cfg.AttributeMetadataCacheTTL)
	require.Equal(t, defaultAttributeMetadataCacheMaxSize, cfg.AttributeMetadataCacheMaxSize)
}

func TestConfiguration_TLS(t *testing.T) {
	tests := []struct {
		name string
		tls  configoptional.Optional[configtls.ClientConfig]
	}{
		{
			name: "TLS omitted (plaintext)",
		},
		{
			name: "TLS enabled with default verification",
			tls:  configoptional.Some(configtls.ClientConfig{}),
		},
		{
			name: "TLS enabled with InsecureSkipVerify",
			tls: configoptional.Some(configtls.ClientConfig{
				InsecureSkipVerify: true,
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfiguration()
			cfg.Addresses = []string{"localhost:9000"}
			cfg.TLS = tt.tls
			require.NoError(t, cfg.Validate())
		})
	}
}

func TestConfiguration_Validate_TTL(t *testing.T) {
	tests := []struct {
		name     string
		ttl      time.Duration
		errorMsg string
	}{
		{
			name: "Zero TTL (Disabled) is valid",
			ttl:  0,
		},
		{
			name: "Positive TTL is valid",
			ttl:  1 * time.Hour,
		},
		{
			name:     "Negative TTL is invalid",
			ttl:      -1 * time.Hour,
			errorMsg: "ttl must be a non-negative duration",
		},
		{
			name:     "Sub-second fraction TTL is invalid",
			ttl:      1500 * time.Millisecond,
			errorMsg: "ttl must be a whole number of seconds",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := DefaultConfiguration()
			cfg.Addresses = []string{"localhost:9000"}
			cfg.TTL = test.ttl
			err := cfg.Validate()
			if test.errorMsg != "" {
				require.ErrorContains(t, err, test.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
