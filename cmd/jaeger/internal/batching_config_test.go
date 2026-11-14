// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/kafkaexporter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/pipeline"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/exporters/storageexporter"
	"github.com/jaegertracing/jaeger/internal/jconfmap"
)

func TestShippedConfigsUseQueueBatching(t *testing.T) {
	const gateID = "pkg.exporterhelper.queueBatchEnabled"
	registry := featuregate.GlobalRegistry()
	var gate *featuregate.Gate
	registry.VisitAll(func(g *featuregate.Gate) {
		if g.ID() == gateID {
			gate = g
		}
	})
	require.NotNil(t, gate)
	original := gate.IsEnabled()
	t.Cleanup(func() { require.NoError(t, registry.Set(gateID, original)) })
	t.Setenv("DEAD_LETTER_ENDPOINT", "http://127.0.0.1:4318/v1/traces")
	t.Setenv("MY_POD_IP", "127.0.0.1")

	files, err := filepath.Glob("../config*.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	files = append(files,
		"all-in-one.yaml",
		"../../../docker-compose/kafka/jaeger-ingester-remote-storage.yaml",
		"../../../examples/oci/config.yaml",
		"../../../examples/otel-demo/jaeger-config.yaml",
	)

	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("future_batching_default=%t", enabled), func(t *testing.T) {
			require.NoError(t, registry.Set(gateID, enabled))
			factories, err := Components()
			require.NoError(t, err)
			for _, file := range files {
				// Tail-sampling files are processor-only merge snippets, not standalone configs.
				if strings.Contains(file, "config-tail-sampling-") {
					continue
				}
				t.Run(file, func(t *testing.T) {
					if strings.HasSuffix(file, "-data-stream.yaml") {
						assertDataStreamBatching(t, factories, file)
						return
					}
					cfg := loadAndValidateConfig(t, factories, "file:"+file)
					assert.NotContains(t, cfg.Processors, component.MustNewID("batch"))
					traces := cfg.Service.Pipelines[pipeline.NewID(pipeline.SignalTraces)]
					require.NotNil(t, traces)
					assert.NotContains(t, traces.Processors, component.MustNewID("batch"))

					if raw, ok := cfg.Processors[component.MustNewID("queue_batch")]; ok {
						shared := raw.(*exporterhelper.QueueBatchConfig)
						assertStorageBatchQueue(t, shared)
						assert.Equal(t, []component.ID{component.MustNewID("queue_batch")}, traces.Processors)
						assert.Equal(t, []component.ID{storageexporter.ID, component.MustNewID("spanmetrics")}, traces.Exporters)
						storage := cfg.Exporters[storageexporter.ID].(*storageexporter.Config)
						assert.False(t, storage.QueueConfig.HasValue(), "shared batching must not be followed by exporter batching")
						return
					}
					if raw, ok := cfg.Exporters[storageexporter.ID]; ok {
						queue := raw.(*storageexporter.Config).QueueConfig
						require.True(t, queue.HasValue())
						if strings.HasSuffix(file, "config-kafka-ingester-sync.yaml") {
							assert.True(t, queue.Get().WaitForResult)
							assert.Equal(t, exporterhelper.RequestSizerTypeBytes, queue.Get().Sizer)
							return
						}
						assertStorageBatchQueue(t, queue.Get())
						if filepath.Base(file) == "config.yaml" || filepath.Base(file) == "config-cassandra.yaml" {
							assert.Equal(t, []component.ID{component.MustNewID("adaptive_sampling")}, traces.Processors)
						}
						return
					}
					if raw, ok := cfg.Exporters[component.MustNewID("kafka")]; ok {
						queue := raw.(*kafkaexporter.Config).QueueBatchConfig
						require.True(t, queue.HasValue())
						assertStandardExporterBatchQueue(t, queue.Get())
						return
					}
					assert.Empty(t, traces.Processors)
				})
			}
			for _, file := range []string{
				"../../../docker-compose/monitor/otel-collector-config-connector.yml",
				"../../../examples/otel-demo/otel-demo-values.yaml",
			} {
				t.Run(file, func(t *testing.T) {
					conf := resolveBatchingConfig(t, file)
					exporterID := "otlp"
					if strings.HasSuffix(file, "otel-demo-values.yaml") {
						nested, err := conf.Sub("opentelemetry-collector::alternateConfig")
						require.NoError(t, err)
						conf = nested
						exporterID = "otlp/jaeger"
					}
					assert.NotContains(t, conf.Get("service::pipelines::traces::processors"), "batch")
					exporterConf, err := conf.Sub("exporters::" + exporterID)
					require.NoError(t, err)
					cfg := otlpexporter.NewFactory().CreateDefaultConfig().(*otlpexporter.Config)
					require.NoError(t, exporterConf.Unmarshal(cfg))
					require.NoError(t, confmap.Validate(cfg))
					require.True(t, cfg.QueueConfig.HasValue())
					assertStandardExporterBatchQueue(t, cfg.QueueConfig.Get())
				})
			}
		})
	}
}

func assertDataStreamBatching(t *testing.T, factories otelcol.Factories, file string) {
	t.Helper()
	// RFC 0004 Phase 2 examples already fail full validation because the backend
	// mode is unimplemented. Preserve that exact error and validate the migrated
	// exporter separately instead of changing storage behavior in this migration.
	settings := jconfmap.ResolverSettings()
	settings.URIs = []string{"file:" + file}
	provider, err := otelcol.NewConfigProvider(otelcol.ConfigProviderSettings{ResolverSettings: settings})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	cfg, err := provider.Get(context.Background(), factories)
	require.NoError(t, err)
	backend := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(file), "config-"), "-data-stream.yaml")
	require.EqualError(t, confmap.Validate(cfg), "extensions::jaeger_storage::config::backends::some_storage::"+backend+
		": indices.spans.rotation: data_stream is not yet implemented")

	assert.Empty(t, cfg.Service.Pipelines[pipeline.NewID(pipeline.SignalTraces)].Processors)
	assert.Empty(t, cfg.Processors)
	storageCfg := cfg.Exporters[storageexporter.ID].(*storageexporter.Config)
	require.NoError(t, confmap.Validate(storageCfg))
	require.True(t, storageCfg.QueueConfig.HasValue())
	assertStorageBatchQueue(t, storageCfg.QueueConfig.Get())
}

func assertStorageBatchQueue(t *testing.T, cfg *exporterhelper.QueueBatchConfig) {
	t.Helper()
	assert.Equal(t, exporterhelper.RequestSizerTypeRequests, cfg.Sizer)
	assert.Equal(t, 1, cfg.NumConsumers)
	assert.EqualValues(t, 8192, cfg.QueueSize)
	assert.True(t, cfg.BlockOnOverflow)
	assert.False(t, cfg.WaitForResult)
	assertSpanBatch(t, cfg)
}

func assertStandardExporterBatchQueue(t *testing.T, cfg *exporterhelper.QueueBatchConfig) {
	t.Helper()
	assert.Equal(t, exporterhelper.RequestSizerTypeRequests, cfg.Sizer)
	assert.Equal(t, 10, cfg.NumConsumers)
	assert.EqualValues(t, 1000, cfg.QueueSize)
	assert.False(t, cfg.BlockOnOverflow)
	assert.False(t, cfg.WaitForResult)
	assertSpanBatch(t, cfg)
}

func assertSpanBatch(t *testing.T, cfg *exporterhelper.QueueBatchConfig) {
	t.Helper()
	require.True(t, cfg.Batch.HasValue())
	batch := cfg.Batch.Get()
	assert.Equal(t, exporterhelper.RequestSizerTypeItems, batch.Sizer)
	assert.EqualValues(t, 8192, batch.MinSize)
	assert.Zero(t, batch.MaxSize)
	assert.Equal(t, 200*time.Millisecond, batch.FlushTimeout)
}

func resolveBatchingConfig(t *testing.T, file string) *confmap.Conf {
	t.Helper()
	settings := jconfmap.ResolverSettings()
	settings.URIs = []string{"file:" + file}
	resolver, err := confmap.NewResolver(settings)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resolver.Shutdown(context.Background())) })
	conf, err := resolver.Resolve(context.Background())
	require.NoError(t, err)
	return conf
}
