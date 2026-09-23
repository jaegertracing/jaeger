// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageexporter

import (
	"context"
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/connector/connectortest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

// TestConnector_QueueMustWaitForResult covers the one rule the connector form adds
// on top of the exporter's Config: an enabled queue must block on the write result.
func TestConnector_QueueMustWaitForResult(t *testing.T) {
	blocking := exporterhelper.NewDefaultQueueConfig()
	blocking.WaitForResult = true
	acknowledgingOnEnqueue := exporterhelper.NewDefaultQueueConfig()

	tests := []struct {
		name    string
		mutate  func(cfg *Config)
		wantErr error
	}{
		{name: "no queue", mutate: func(*Config) {}},
		{
			name:   "blocking queue",
			mutate: func(cfg *Config) { cfg.QueueConfig = configoptional.Some(blocking) },
		},
		{
			name:    "queue that acknowledges on enqueue",
			mutate:  func(cfg *Config) { cfg.QueueConfig = configoptional.Some(acknowledgingOnEnqueue) },
			wantErr: errQueueWithoutResult,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := directConfig()
			tt.mutate(cfg)
			require.NoError(t, cfg.Validate(), "the shared Config accepts every shape; only the connector form objects")
			conn, err := createTracesToTraces(context.Background(), connectortest.NewNopSettings(componentType), cfg, consumertest.NewNop())
			if tt.wantErr == nil {
				require.NoError(t, err)
				require.NotNil(t, conn)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
			assert.Nil(t, conn)
		})
	}
}

func TestConnector_ReadmeExampleIsValid(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	require.NoError(t, err)
	m := regexp.MustCompile("(?s)```yaml\n(connectors:\n.*?)```").FindSubmatch(readme)
	require.NotNil(t, m, "README has a yaml example whose top-level key is connectors")

	resolver, err := confmap.NewResolver(confmap.ResolverSettings{
		URIs:              []string{"yaml:" + string(m[1])},
		ProviderFactories: []confmap.ProviderFactory{yamlprovider.NewFactory()},
	})
	require.NoError(t, err)
	conf, err := resolver.Resolve(context.Background())
	require.NoError(t, err)
	sub, err := conf.Sub("connectors::jaeger_storage_exporter")
	require.NoError(t, err)

	cfg := createDefaultConfig().(*Config)
	require.NoError(t, sub.Unmarshal(cfg))
	require.NoError(t, cfg.Validate())
	_, err = createTracesToTraces(context.Background(), connectortest.NewNopSettings(componentType), cfg, consumertest.NewNop())
	require.NoError(t, err)
	assert.True(t, cfg.QueueConfig.Get().WaitForResult)
	assert.Equal(t, "some_storage", cfg.TraceStorage)
}
