// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageexporter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				TraceStorage: "test",
				RetryConfig: configretry.BackOffConfig{
					Enabled:         true,
					InitialInterval: 5 * time.Second,
					MaxInterval:     30 * time.Second,
					MaxElapsedTime:  5 * time.Minute,
				},
			},
			wantErr: false,
		},
		{
			name: "missing trace storage",
			config: &Config{
				TraceStorage: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func createTestExporterSettings() exporter.Settings {
	return exporter.Settings{
		ID:                component.MustNewIDWithName("jaeger_storage", "test"),
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}
}

func TestCreateTracesExporter(t *testing.T) {
	t.Run("with default config", func(t *testing.T) {
		defaultCfg := createDefaultConfig().(*Config)
		defaultCfg.TraceStorage = "test"

		exp, err := createTracesExporter(
			context.Background(),
			createTestExporterSettings(),
			defaultCfg,
		)

		require.NoError(t, err)
		require.NotNil(t, exp)
		assert.False(t, defaultCfg.RetryConfig.Enabled, "Retry should be disabled by default")
	})

	t.Run("with custom retry config", func(t *testing.T) {
		cfg := &Config{
			TraceStorage: "test",
			RetryConfig: configretry.BackOffConfig{
				Enabled:             true,
				InitialInterval:     10 * time.Second,
				RandomizationFactor: 0.7,
				Multiplier:          2.0,
				MaxInterval:         60 * time.Second,
				MaxElapsedTime:      10 * time.Minute,
			},
		}

		exp, err := createTracesExporter(
			context.Background(),
			createTestExporterSettings(),
			cfg,
		)

		require.NoError(t, err)
		require.NotNil(t, exp)

		assert.True(t, cfg.RetryConfig.Enabled, "Retry should be enabled")
		assert.Equal(t, 10*time.Second, cfg.RetryConfig.InitialInterval, "InitialInterval should be 10s")
		assert.InDelta(t, 0.7, cfg.RetryConfig.RandomizationFactor, 1e-6, "RandomizationFactor should be 0.7")
		assert.InDelta(t, 2.0, cfg.RetryConfig.Multiplier, 1e-6, "Multiplier should be 2.0")
		assert.Equal(t, 60*time.Second, cfg.RetryConfig.MaxInterval, "MaxInterval should be 60s")
		assert.Equal(t, 10*time.Minute, cfg.RetryConfig.MaxElapsedTime, "MaxElapsedTime should be 10m")
	})

	t.Run("with disabled retry", func(t *testing.T) {
		cfg := &Config{
			TraceStorage: "test",
			RetryConfig: configretry.BackOffConfig{
				Enabled: false,
			},
		}

		exp, err := createTracesExporter(
			context.Background(),
			createTestExporterSettings(),
			cfg,
		)

		require.NoError(t, err)
		require.NotNil(t, exp)

		assert.False(t, cfg.RetryConfig.Enabled)
	})
}

func TestCreateTracesExporter_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				TraceStorage: "test",
				RetryConfig: configretry.BackOffConfig{
					Enabled: true,
				},
			},
			wantErr: false,
		},
		{
			name: "missing trace storage",
			config: &Config{
				RetryConfig: configretry.BackOffConfig{
					Enabled: true,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
