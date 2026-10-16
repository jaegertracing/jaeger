// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/otelcol"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/exporters/storageexporter"
	"github.com/jaegertracing/jaeger/internal/jconfmap"
)

// TestKafkaIngesterConfigsAcceptQueueBatchPartition loads the Kafka ingester
// configs the e2e suite runs and rejects a queue.batch whose partition is the
// zero value. exporterhelper v0.162 requires partition.idle_timeout and
// partition.cache_size to be positive even when metadata_keys is empty, and an
// omitted partition unmarshals as zeros rather than NewDefaultPartitionConfig.
func TestKafkaIngesterConfigsAcceptQueueBatchPartition(t *testing.T) {
	// The dead-letter sink endpoint has no default; validation only requires
	// it to be non-empty. The e2e suite sets the same variable.
	t.Setenv("DEAD_LETTER_ENDPOINT", "http://127.0.0.1:4318/v1/traces")

	factories, err := Components()
	require.NoError(t, err)

	tests := []struct {
		file      string
		connector bool
	}{
		{file: "file:../config-kafka-collector.yaml"},
		{file: "file:../config-kafka-ingester.yaml"},
		{file: "file:../config-kafka-ingester-sync.yaml"},
		{file: "file:../config-kafka-ingester-dead-letter.yaml", connector: true},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			cfg := loadAndValidateConfig(t, factories, tt.file)
			if tt.file != "file:../config-kafka-ingester-sync.yaml" && !tt.connector {
				return
			}
			var raw any
			if tt.connector {
				raw = cfg.Connectors[storageexporter.ID]
			} else {
				raw = cfg.Exporters[storageexporter.ID]
			}
			queue, ok := raw.(*storageexporter.Config)
			require.True(t, ok, "jaeger_storage_exporter config type")
			require.True(t, queue.QueueConfig.HasValue())
			batch := queue.QueueConfig.Get().Batch
			require.True(t, batch.HasValue())
			require.Equal(t, 90*time.Second, batch.Get().Partition.IdleTimeout)
			require.Equal(t, 10000, batch.Get().Partition.CacheSize)
		})
	}
}

func loadAndValidateConfig(t *testing.T, factories otelcol.Factories, uri string) *otelcol.Config {
	t.Helper()
	settings := jconfmap.ResolverSettings()
	settings.URIs = []string{uri}
	provider, err := otelcol.NewConfigProvider(otelcol.ConfigProviderSettings{
		ResolverSettings: settings,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	cfg, err := provider.Get(context.Background(), factories)
	require.NoError(t, err)
	require.NoError(t, confmap.Validate(cfg))
	return cfg
}
