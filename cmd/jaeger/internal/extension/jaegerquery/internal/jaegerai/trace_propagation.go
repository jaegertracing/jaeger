// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"

	"go.opentelemetry.io/otel"
)

// injectTraceContextIntoMeta injects the active span context from ctx into a
// (possibly nil) ACP _meta map, returning the map to use. Mirrors the SEP-414
// convention the MCP tracing middleware already uses for extraction at the
// tool-call boundary (mcptools), applied here to the ACP prompt boundary, so
// a sidecar that extracts it parents its own agentic-loop spans under this
// request's span instead of starting a disconnected trace.
//
// Uses the process-global propagator (otel.GetTextMapPropagator()) rather
// than a locally defined one: internal/jtracer already installs the SEP-414
// composite (W3C TraceContext + Baggage) as the global propagator on startup,
// before any request is served, so defining another one here would just be a
// second copy of the same value.
func injectTraceContextIntoMeta(ctx context.Context, meta map[string]any) map[string]any {
	carrier := &metaCarrier{meta: meta}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier.meta
}

// metaCarrier adapts a map[string]any — the shape of an ACP request's _meta
// field — to propagation.TextMapCarrier. Private: the only current caller is
// injectTraceContextIntoMeta in this file.
type metaCarrier struct {
	meta map[string]any
}

func (c *metaCarrier) Get(key string) string {
	value, _ := c.meta[key].(string)
	return value
}

func (c *metaCarrier) Set(key, value string) {
	if c.meta == nil {
		c.meta = map[string]any{}
	}
	c.meta[key] = value
}

func (c *metaCarrier) Keys() []string {
	keys := make([]string, 0, len(c.meta))
	for key := range c.meta {
		keys = append(keys, key)
	}
	return keys
}
