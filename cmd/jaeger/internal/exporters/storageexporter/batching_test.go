// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageexporter

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jconfmap"
)

type batchingWriter struct {
	*consumertest.TracesSink
}

func (w batchingWriter) WriteTraces(ctx context.Context, td ptrace.Traces) error {
	return w.ConsumeTraces(ctx, td)
}

func TestShippedExporterBatchingFlushes(t *testing.T) {
	for _, trigger := range []string{"size", "size_single_span_requests", "timeout", "shutdown"} {
		t.Run(trigger, func(t *testing.T) {
			settings := jconfmap.ResolverSettings()
			settings.URIs = []string{"file:../../../config-badger.yaml"}
			resolver, err := confmap.NewResolver(settings)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, resolver.Shutdown(context.Background())) })
			conf, err := resolver.Resolve(context.Background())
			require.NoError(t, err)
			sub, err := conf.Sub("exporters::jaeger_storage_exporter")
			require.NoError(t, err)
			cfg := createDefaultConfig().(*Config)
			require.NoError(t, sub.Unmarshal(cfg))
			require.NoError(t, confmap.Validate(cfg))
			require.True(t, cfg.QueueConfig.HasValue())
			require.True(t, cfg.QueueConfig.Get().Batch.HasValue())
			batch := cfg.QueueConfig.Get().Batch.Get()
			if trigger != "timeout" {
				// Isolate size and shutdown from the timeout trigger.
				batch.FlushTimeout = time.Hour
			}

			sink := new(consumertest.TracesSink)
			exp, err := NewFactory().CreateTraces(context.Background(), exportertest.NewNopSettings(ID.Type()), cfg)
			require.NoError(t, err)
			require.NoError(t, exp.Start(context.Background(), hostWith(cfg.TraceStorage, writerFactory(batchingWriter{sink}))))
			shutdown := sync.OnceValue(func() error { return exp.Shutdown(context.Background()) })
			t.Cleanup(func() { require.NoError(t, shutdown()) })

			spans := 1
			switch trigger {
			case "size_single_span_requests":
				spans = int(batch.MinSize)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				for i := range spans {
					require.NoError(t, exp.ConsumeTraces(ctx, batchingTraces(1)), "request %d must be enqueued without waiting for a batch timeout", i+1)
				}
			case "size":
				spans = int(batch.MinSize)
				require.NoError(t, exp.ConsumeTraces(context.Background(), batchingTraces(spans/2)))
				require.NoError(t, exp.ConsumeTraces(context.Background(), batchingTraces(spans/2)))
			default:
				require.NoError(t, exp.ConsumeTraces(context.Background(), batchingTraces(spans)))
			}
			if trigger == "shutdown" {
				require.NoError(t, shutdown())
			}
			require.Eventually(t, func() bool { return sink.SpanCount() == spans }, 5*time.Second, 10*time.Millisecond)
			require.Len(t, sink.AllTraces(), 1, "requests are merged and flushed as one batch")
		})
	}
}

func batchingTraces(count int) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "batching-test")
	spans := rs.ScopeSpans().AppendEmpty().Spans()
	for range count {
		spans.AppendEmpty().SetName("operation")
	}
	return td
}
