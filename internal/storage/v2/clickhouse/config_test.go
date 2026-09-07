// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"
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
			name:    "negative max search depth",
			mutate:  func(cfg *Configuration) { cfg.MaxSearchDepth = -1 },
			wantErr: "max_search_depth must be a positive number",
		},
		{
			name:    "negative attribute metadata cache TTL",
			mutate:  func(cfg *Configuration) { cfg.AttributeMetadataCacheTTL = -time.Second },
			wantErr: "attribute_metadata_cache_ttl must be a non-negative duration",
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

func TestDefaultConfiguration(t *testing.T) {
	cfg := DefaultConfiguration()

	require.Equal(t, defaultProtocol, cfg.Protocol)
	require.Equal(t, defaultDatabase, cfg.Database)
	require.Equal(t, defaultSearchDepth, cfg.DefaultSearchDepth)
	require.Equal(t, defaultMaxSearchDepth, cfg.MaxSearchDepth)
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
