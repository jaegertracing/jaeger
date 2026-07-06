// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package tracecontext carries W3C trace context across a protocol's _meta
// field (SEP-414) — the convention MCP and ACP both use since neither
// transport reliably carries plain HTTP headers end to end.
//
// This package is intended to be shared by the MCP tool-call boundary and the
// ACP prompt boundary to avoid duplicating carrier/propagator logic.
package tracecontext

import (
	"go.opentelemetry.io/otel/propagation"
)

// Propagator is the SEP-414 composite propagator (W3C traceparent/tracestate
// + baggage) for carrying trace context across a protocol's _meta field.
var Propagator = propagation.NewCompositeTextMapPropagator(
	propagation.TraceContext{},
	propagation.Baggage{},
)

// Carrier adapts a map[string]any — the shape of every MCP and ACP request's
// _meta field — to propagation.TextMapCarrier, so Propagator can inject into
// or extract from it directly.
type Carrier struct {
	Meta map[string]any
}

func (c *Carrier) Get(key string) string {
	value, _ := c.Meta[key].(string)
	return value
}

func (c *Carrier) Set(key, value string) {
	if c.Meta == nil {
		c.Meta = map[string]any{}
	}
	c.Meta[key] = value
}

func (c *Carrier) Keys() []string {
	keys := make([]string, 0, len(c.Meta))
	for key := range c.Meta {
		keys = append(keys, key)
	}
	return keys
}
