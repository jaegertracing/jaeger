// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracecontext

import (
	"testing"

	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestCarrierGetSetKeys(t *testing.T) {
	carrier := &Carrier{}
	carrier.Set("traceparent", "00-abc-def-01")
	carrier.Set("tracestate", "vendor=value")

	if got := carrier.Get("traceparent"); got != "00-abc-def-01" {
		t.Fatalf("expected traceparent %q, got %q", "00-abc-def-01", got)
	}
	if got := carrier.Get("tracestate"); got != "vendor=value" {
		t.Fatalf("expected tracestate %q, got %q", "vendor=value", got)
	}
	if got := carrier.Get("missing"); got != "" {
		t.Fatalf("expected empty string for missing key, got %q", got)
	}

	keys := carrier.Keys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d: %v", len(keys), keys)
	}
}

func TestCarrierSetOnNilMapAllocates(t *testing.T) {
	carrier := &Carrier{}
	carrier.Set("traceparent", "00-abc-def-01")

	if carrier.Meta == nil {
		t.Fatal("expected Set to allocate Meta when nil")
	}
	if carrier.Meta["traceparent"] != "00-abc-def-01" {
		t.Fatalf("expected Meta to contain the set value, got %+v", carrier.Meta)
	}
}

func TestPropagatorInjectExtractRoundTrip(t *testing.T) {
	// A plain map[string]any round-tripped through Inject then Extract should
	// carry the same trace id — this is the exact shape both the MCP
	// tool-call boundary and the ACP prompt boundary rely on. Requires a real
	// recording tracer: a noop tracer's span context is invalid, and the W3C
	// TraceContext propagator skips injection entirely for invalid contexts.
	provider := tracesdk.NewTracerProvider(tracesdk.WithSampler(tracesdk.AlwaysSample()))
	t.Cleanup(func() {
		if err := provider.Shutdown(t.Context()); err != nil {
			t.Fatalf("failed to shut down tracer provider: %v", err)
		}
	})
	ctx, span := provider.Tracer("test").Start(t.Context(), "test-span")
	defer span.End()

	meta := map[string]any{}
	Propagator.Inject(ctx, &Carrier{Meta: meta})
	if _, ok := meta["traceparent"]; !ok {
		t.Fatalf("expected traceparent to be injected into meta, got %+v", meta)
	}

	extractedCtx := Propagator.Extract(t.Context(), &Carrier{Meta: meta})
	extractedSpanContext := oteltrace.SpanContextFromContext(extractedCtx)
	if extractedSpanContext.TraceID() != span.SpanContext().TraceID() {
		t.Fatalf("expected extracted trace id %s, got %s", span.SpanContext().TraceID(), extractedSpanContext.TraceID())
	}
}
