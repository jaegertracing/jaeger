// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"

	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

// injectTraceContextIntoMeta injects the active span context from ctx into a
// (possibly nil) ACP _meta map, returning the map to use. Mirrors the SEP-414
// convention the MCP tracing middleware already uses for extraction at the
// tool-call boundary (mcptools), applied here to the ACP prompt boundary, so
// a sidecar that extracts it parents its own agentic-loop spans under this
// request's span instead of starting a disconnected trace.
func injectTraceContextIntoMeta(ctx context.Context, meta map[string]any) map[string]any {
	carrier := &otelsemconv.TraceContextCarrier{Meta: meta}
	otelsemconv.TraceContextPropagator.Inject(ctx, carrier)
	return carrier.Meta
}
