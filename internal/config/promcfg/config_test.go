// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package promcfg

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"

	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestValidate(t *testing.T) {
	cfg := Configuration{
		ServerURL: "localhost:1234",
	}
	err := cfg.Validate()
	require.NoError(t, err)
	cfg = Configuration{}
	err = cfg.Validate()
	require.Error(t, err)
}

func TestValidateExtraDimensions(t *testing.T) {
	tests := []struct {
		name    string
		dims    []metricstore.Dimension
		wantErr string
	}{
		{
			name: "valid single",
			dims: []metricstore.Dimension{{Name: "deployment.environment"}},
		},
		{
			name: "valid multiple",
			dims: []metricstore.Dimension{
				{Name: "deployment.environment"},
				{Name: "k8s.cluster"},
				{Name: "team_id"},
			},
		},
		{
			name:    "empty name",
			dims:    []metricstore.Dimension{{Name: ""}},
			wantErr: "invalid name",
		},
		{
			name:    "leading digit",
			dims:    []metricstore.Dimension{{Name: "0env"}},
			wantErr: "invalid name",
		},
		{
			name:    "hyphen disallowed",
			dims:    []metricstore.Dimension{{Name: "kebab-case"}},
			wantErr: "invalid name",
		},
		{
			name:    "space disallowed",
			dims:    []metricstore.Dimension{{Name: "with space"}},
			wantErr: "invalid name",
		},
		{
			name: "duplicate names rejected",
			dims: []metricstore.Dimension{
				{Name: "deployment.environment"},
				{Name: "deployment.environment"},
			},
			wantErr: "duplicate name",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Configuration{
				ServerURL:       "localhost:1234",
				ExtraDimensions: tc.dims,
			}
			err := cfg.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// TestExtraDimensionsConfmapDecode locks in the mapstructure tags on
// metricstore.Dimension. Without them, OTel's confmap strict-decode rejects
// the snake_case YAML keys (display_name, etc.) so operator configs fail to
// load.
func TestExtraDimensionsConfmapDecode(t *testing.T) {
	conf := confmap.NewFromStringMap(map[string]any{
		"endpoint": "http://prom:9090",
		"extra_dimensions": []any{
			map[string]any{
				"name":         "deployment.environment",
				"display_name": "Environment",
				"values":       []any{"prod", "staging"},
			},
		},
	})
	var cfg Configuration
	require.NoError(t, conf.Unmarshal(&cfg))
	require.Len(t, cfg.ExtraDimensions, 1)
	assert.Equal(t, "deployment.environment", cfg.ExtraDimensions[0].Name)
	assert.Equal(t, "Environment", cfg.ExtraDimensions[0].DisplayName)
	assert.Equal(t, []string{"prod", "staging"}, cfg.ExtraDimensions[0].Values)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
