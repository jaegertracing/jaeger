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
			name: "Invalid File provider",
			config: &Config{
				File: &FileConfig{Path: ""},
			},
			expectedErr: "File.Path: non zero value required",
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
				assert.Equal(t, tt.expectedErr, err.Error())
			}
		})
	}
}

func Test_Unmarshal(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]interface{}
		expectedCfg *Config
		expectedErr string
	}{
		{
			name: "Valid config with File",
			input: map[string]interface{}{
				"file": map[string]interface{}{
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
			input: map[string]interface{}{
				"adaptive": map[string]interface{}{
					"sampling_store": "test-store",
				},
			},
			expectedCfg: &Config{
				Adaptive: &AdaptiveConfig{SamplingStore: "test-store"},
			},
			expectedErr: "",
		},
		{
			name:  "Empty config",
			input: map[string]interface{}{},
			expectedCfg: &Config{
				File:     nil,
				Adaptive: nil,
				HTTP:     nil,
				GRPC:     nil,
			},
			expectedErr: "",
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
				assert.EqualError(t, err, tt.expectedErr)
			}
		})
	}
}
