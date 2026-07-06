// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestInjectTraceContextIntoMetaNilMapCreatesOne(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := tracesdk.NewTracerProvider(
		tracesdk.WithSyncer(exporter),
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
	)
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	ctx, span := provider.Tracer("test").Start(context.Background(), "test-span")
	defer span.End()

	meta := injectTraceContextIntoMeta(ctx, nil)

	require.NotNil(t, meta)
	traceparent, ok := meta["traceparent"].(string)
	require.True(t, ok, "expected traceparent to be injected as a string, got %+v", meta)
	assert.Contains(t, traceparent, span.SpanContext().TraceID().String())
}

func TestInjectTraceContextIntoMetaPreservesExistingEntries(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := tracesdk.NewTracerProvider(
		tracesdk.WithSyncer(exporter),
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
	)
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	ctx, span := provider.Tracer("test").Start(context.Background(), "test-span")
	defer span.End()

	existing := map[string]any{"jaegertracing.io/contextual-tools": "keep-me"}
	meta := injectTraceContextIntoMeta(ctx, existing)

	assert.Equal(t, "keep-me", meta["jaegertracing.io/contextual-tools"])
	assert.Contains(t, meta, "traceparent")
}

func TestInjectTraceContextIntoMetaNoSpanInContextLeavesMetaNil(t *testing.T) {
	meta := injectTraceContextIntoMeta(context.Background(), nil)
	// A background context carries no recording span, so the propagator
	// has nothing valid to inject; the carrier's map stays nil/empty rather
	// than being force-allocated with junk.
	assert.Empty(t, meta)
}
