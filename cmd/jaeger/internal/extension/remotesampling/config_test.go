// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotesampling

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
)

func Test_Validate(t *testing.T) {
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
				File:     &FileConfig{Path: "test-path"},
				Adaptive: &AdaptiveConfig{SamplingStore: "test-store"},
			},
			expectedErr: "only one sampling strategy provider can be specified, 'adaptive' or 'file'",
		},
		{
			name: "Only File provider specified",
			config: &Config{
				File: &FileConfig{Path: "test-path"},
			},
			expectedErr: "",
		},
		{
			name: "Only Adaptive provider specified",
			config: &Config{
				Adaptive: &AdaptiveConfig{SamplingStore: "test-store"},
			},
			expectedErr: "",
		},
		{
			name: "File provider can have empty file path",
			config: &Config{
				File: &FileConfig{Path: ""},
			},
			expectedErr: "",
		},
		{
			name: "File provider has negative reload interval",
			config: &Config{
				File: &FileConfig{Path: "", ReloadInterval: -1},
			},
			expectedErr: "must be a positive value",
		},
		{
			name: "File provider has negative default sampling probability",
			config: &Config{
				File: &FileConfig{Path: "", DefaultSamplingProbability: -0.5},
			},
			expectedErr: "File.DefaultSamplingProbability: -0.5 does not validate as range(0|1)",
		},
		{
			name: "File provider has default sampling probability greater than 1",
			config: &Config{
				File: &FileConfig{Path: "", DefaultSamplingProbability: 1.5},
			},
			expectedErr: "File.DefaultSamplingProbability: 1.5 does not validate as range(0|1)",
		},
		{
			name: "Invalid Adaptive provider",
			config: &Config{
				Adaptive: &AdaptiveConfig{SamplingStore: ""},
			},
			expectedErr: "Adaptive.SamplingStore: non zero value required",
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
				File: &FileConfig{Path: "test-path"},
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
				Adaptive: &AdaptiveConfig{SamplingStore: "test-store"},
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
			err := cfg.Unmarshal(conf)
			if tt.expectedErr == "" {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedCfg, &cfg)
			} else {
				assert.ErrorContains(t, err, tt.expectedErr)
			}
		})
	}
}
