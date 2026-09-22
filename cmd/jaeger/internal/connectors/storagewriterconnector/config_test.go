// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagewriterconnector

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

func TestConfig_Validate(t *testing.T) {
	blocking := exporterhelper.NewDefaultQueueConfig()
	blocking.WaitForResult = true
	acknowledgingOnEnqueue := exporterhelper.NewDefaultQueueConfig()

	tests := []struct {
		name    string
		mutate  func(cfg *Config)
		wantErr string
	}{
		{name: "no queue", mutate: func(*Config) {}},
		{
			name:   "blocking queue",
			mutate: func(cfg *Config) { cfg.QueueConfig = configoptional.Some(blocking) },
		},
		{
			name:    "queue that acknowledges on enqueue",
			mutate:  func(cfg *Config) { cfg.QueueConfig = configoptional.Some(acknowledgingOnEnqueue) },
			wantErr: "queue.wait_for_result must be true",
		},
		{
			name:    "missing trace_storage",
			mutate:  func(cfg *Config) { cfg.TraceStorage = "" },
			wantErr: "TraceStorage: non zero value required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := directConfig()
			tt.mutate(cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// TestConfig_ReadmeExampleIsValid loads the connector block of the README's yaml
// example through confmap onto the default config, so the documented example is
// one that the collector accepts.
func TestConfig_ReadmeExampleIsValid(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	require.NoError(t, err)
	m := regexp.MustCompile("(?s)```yaml\n(.*?)```").FindSubmatch(readme)
	require.NotNil(t, m, "README has a yaml example")

	resolver, err := confmap.NewResolver(confmap.ResolverSettings{
		URIs:              []string{"yaml:" + string(m[1])},
		ProviderFactories: []confmap.ProviderFactory{yamlprovider.NewFactory()},
	})
	require.NoError(t, err)
	conf, err := resolver.Resolve(context.Background())
	require.NoError(t, err)
	sub, err := conf.Sub("connectors::jaeger_storage_writer")
	require.NoError(t, err)

	cfg := createDefaultConfig().(*Config)
	require.NoError(t, sub.Unmarshal(cfg))
	require.NoError(t, cfg.Validate())
	assert.True(t, cfg.QueueConfig.Get().WaitForResult)
	assert.True(t, strings.HasPrefix(cfg.TraceStorage, "some_storage"))
}
