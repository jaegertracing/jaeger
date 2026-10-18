// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"errors"
	"testing"
	"time"

	promreg "github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/metrics/otelmetrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
)

func TestBulkWriterLatencyBuckets(t *testing.T) {
	tests := []struct {
		name      string
		newWriter func(*testing.T, metrics.Factory) *spanstoremetrics.WriteMetrics
	}{
		{
			name: "async",
			newWriter: func(t *testing.T, factory metrics.Factory) *spanstoremetrics.WriteMetrics {
				writer, err := NewBulkIndexer(makeClient(t, "http://localhost:1", "", ""), BulkIndexerConfig{}, factory, zap.NewNop())
				require.NoError(t, err)
				t.Cleanup(func() { assert.NoError(t, writer.Close()) })
				return writer.metrics
			},
		},
		{
			name: "sync",
			newWriter: func(t *testing.T, factory metrics.Factory) *spanstoremetrics.WriteMetrics {
				return NewSyncBulkWriter(makeClient(t, "http://localhost:1", "", ""), 0, false, factory, zap.NewNop()).metrics
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := promreg.NewPedanticRegistry()
			exporter, err := prometheus.New(prometheus.WithRegisterer(registry), prometheus.WithoutScopeInfo())
			require.NoError(t, err)
			provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
			t.Cleanup(func() { assert.NoError(t, provider.Shutdown(context.Background())) })
			tags := map[string]string{"kind": "elasticsearch", "name": "primary", "role": "tracestore"}
			factory := otelmetrics.NewFactory(provider).Namespace(metrics.NSOptions{Name: "jaeger.storage", Tags: tags})
			writer := tt.newWriter(t, factory)
			writer.Emit(nil, 30*time.Millisecond)
			writer.Emit(errors.New("write failed"), 30*time.Millisecond)

			families, err := registry.Gather()
			require.NoError(t, err)
			latencies := map[string]bool{
				"jaeger_storage_bulk_index_latency_ok_seconds":  false,
				"jaeger_storage_bulk_index_latency_err_seconds": false,
			}
			counters := map[string]float64{
				"jaeger_storage_bulk_index_attempts_total": 2,
				"jaeger_storage_bulk_index_inserts_total":  1,
				"jaeger_storage_bulk_index_errors_total":   1,
			}
			for _, family := range families {
				if _, ok := latencies[family.GetName()]; !ok {
					if value, ok := counters[family.GetName()]; ok {
						require.Len(t, family.GetMetric(), 1)
						assert.InDelta(t, value, family.GetMetric()[0].GetCounter().GetValue(), 1e-9)
						delete(counters, family.GetName())
					}
					continue
				}
				latencies[family.GetName()] = true
				require.Len(t, family.GetMetric(), 1)
				data := family.GetMetric()[0]
				labels := make(map[string]string)
				for _, label := range data.GetLabel() {
					labels[label.GetName()] = label.GetValue()
				}
				assert.Equal(t, tags, labels)
				histogram := data.GetHistogram()
				assert.Equal(t, uint64(1), histogram.GetSampleCount())
				assert.InDelta(t, 0.03, histogram.GetSampleSum(), 1e-9)
				boundaries := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
				require.Len(t, histogram.GetBucket(), len(boundaries))
				for i, bucket := range histogram.GetBucket() {
					assert.InDelta(t, boundaries[i], bucket.GetUpperBound(), 1e-9)
					var count uint64
					if boundaries[i] >= 0.03 {
						count = 1
					}
					assert.Equal(t, count, bucket.GetCumulativeCount())
				}
			}
			assert.Empty(t, counters, "all existing write counters are exported")
			for name, found := range latencies {
				assert.True(t, found, "expected histogram %s", name)
			}
		})
	}
}
