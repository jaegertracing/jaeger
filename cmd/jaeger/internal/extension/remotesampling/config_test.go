// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotesampling

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/confmap"

	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy/adaptive"
)

func Test_Validate(t *testing.T) {
	defaultAdaptiveOptions := adaptive.DefaultOptions()
	zeroLeaderLeaseOptions := adaptive.DefaultOptions()
	zeroLeaderLeaseOptions.LeaderLeaseRefreshInterval = 0
	negativeFollowerLeaseOptions := adaptive.DefaultOptions()
	negativeFollowerLeaseOptions.FollowerLeaseRefreshInterval = -time.Second

	tests := []struct {
		name        string
		config      *Config
		expectedErr string
	}{
		{
			name:        "No provider specified",
			config:      &Config{},
			expectedErr: "no sampling strategy provider specified, expecting 'adaptive' or 'file'",
		},
		{
			name: "Both providers specified",
			config: &Config{
				File:     configoptional.Some(FileConfig{Path: "test-path"}),
				Adaptive: configoptional.Some(AdaptiveConfig{SamplingStore: "test-store"}),
			},
			expectedErr: "only one sampling strategy provider can be specified, 'adaptive' or 'file'",
		},
		{
			name: "Only File provider specified",
			config: &Config{
				File: configoptional.Some(FileConfig{Path: "test-path"}),
			},
			expectedErr: "",
		},
		{
			name: "Only Adaptive provider specified",
			config: &Config{
				Adaptive: configoptional.Some(AdaptiveConfig{
					SamplingStore: "test-store",
					Options:       defaultAdaptiveOptions,
				}),
			},
			expectedErr: "",
		},
		{
			name: "File provider can have empty file path",
			config: &Config{
				File: configoptional.Some(FileConfig{Path: ""}),
			},
			expectedErr: "",
		},
		{
			name: "File provider has negative reload interval",
			config: &Config{
				File: configoptional.Some(FileConfig{Path: "", ReloadInterval: -1}),
			},
			expectedErr: "must be a positive value",
		},
		{
			name: "File provider has negative default sampling probability",
			config: &Config{
				File: configoptional.Some(FileConfig{Path: "", DefaultSamplingProbability: -0.5}),
			},
			expectedErr: "DefaultSamplingProbability: -0.5 does not validate as range(0|1)",
		},
		{
			name: "File provider has default sampling probability greater than 1",
			config: &Config{
				File: configoptional.Some(FileConfig{Path: "", DefaultSamplingProbability: 1.5}),
			},
			expectedErr: "DefaultSamplingProbability: 1.5 does not validate as range(0|1)",
		},
		{
			name: "Invalid Adaptive provider",
			config: &Config{
				Adaptive: configoptional.Some(AdaptiveConfig{
					SamplingStore: "",
					Options:       defaultAdaptiveOptions,
				}),
			},
			expectedErr: "SamplingStore: non zero value required",
		},
		{
			name: "Adaptive provider has zero leader lease refresh interval",
			config: &Config{
				Adaptive: configoptional.Some(AdaptiveConfig{
					SamplingStore: "test-store",
					Options:       zeroLeaderLeaseOptions,
				}),
			},
			expectedErr: "leader lease refresh interval must be a positive value",
		},
		{
			name: "Adaptive provider has negative follower lease refresh interval",
			config: &Config{
				Adaptive: configoptional.Some(AdaptiveConfig{
					SamplingStore: "test-store",
					Options:       negativeFollowerLeaseOptions,
				}),
			},
			expectedErr: "follower lease refresh interval must be a positive value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectedErr == "" {
				require.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.expectedErr)
			}
		})
	}
}

func Test_Unmarshal(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]any
		expectedCfg *Config
		expectedErr string
	}{
		{
			name: "Valid config with File",
			input: map[string]any{
				"file": map[string]any{
					"path": "test-path",
				},
			},
			expectedCfg: &Config{
				File: configoptional.Some(FileConfig{Path: "test-path"}),
			},
			expectedErr: "",
		},
		{
			name: "Valid config with Adaptive",
			input: map[string]any{
				"adaptive": map[string]any{
					"sampling_store": "test-store",
				},
			},
			expectedCfg: &Config{
				Adaptive: configoptional.Some(AdaptiveConfig{SamplingStore: "test-store"}),
			},
			expectedErr: "",
		},
		{
			name:        "Empty config",
			input:       map[string]any{},
			expectedCfg: &Config{},
			expectedErr: "",
		},
		{
			name: "Invalid config",
			input: map[string]any{
				"foo": "bar",
			},
			expectedErr: "invalid keys: foo", // sensitive to lib implementation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := confmap.NewFromStringMap(tt.input)
			var cfg Config
			err := conf.Unmarshal(&cfg)
			if tt.expectedErr == "" {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedCfg, &cfg)
			} else {
				assert.ErrorContains(t, err, tt.expectedErr)
			}
		})
	}
}
