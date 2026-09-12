# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import logging
from typing import Any

from opentelemetry import context as otel_context
from opentelemetry import trace
from opentelemetry.baggage.propagation import W3CBaggagePropagator
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.propagators.composite import CompositePropagator
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator

logger = logging.getLogger(__name__)

SERVICE_NAME = "jaeger-ollama-sidecar"

# Mirrors the SEP-414 composite (traceparent/tracestate + baggage) the Go
# gateway's MCP tracing middleware already uses at the tool-call boundary, so
# a _meta dict carrying an injected trace context is read the same way
# regardless of which boundary (MCP tool call or ACP prompt) it crossed.
_meta_propagator = CompositePropagator([
    TraceContextTextMapPropagator(),
    W3CBaggagePropagator(),
])


def init_tracing(endpoint: str, insecure: bool) -> None:
    """Initialize OpenTelemetry tracing.

    Unlike the Gemini sidecar there is no auto-instrumentation to install: the
    Ollama client isn't covered by an OTel instrumentation package, so the
    model call is described by the sidecar's own spans (see the GenAI
    attributes set on `sidecar.agentic_loop`).
    """
    resource = Resource.create({"service.name": SERVICE_NAME})
    provider = TracerProvider(resource=resource)
    provider.add_span_processor(
        BatchSpanProcessor(OTLPSpanExporter(endpoint=endpoint, insecure=insecure))
    )
    trace.set_tracer_provider(provider)

    logger.info("Tracing initialized (endpoint=%s, insecure=%s)", endpoint, insecure)


def tracer() -> trace.Tracer:
    """Return the sidecar's tracer instance."""
    return trace.get_tracer(SERVICE_NAME)


def extract_trace_context(meta: Any) -> otel_context.Context:
    """Extract a parent trace context from an ACP request's _meta dict.

    The Python ACP router spreads _meta into the handler's **kwargs by inner
    key, so callers pass the handler kwargs dict directly (same convention
    _extract_contextual_tools uses). Returns the current context unchanged
    when meta is absent or carries no recognizable trace context, so a prompt
    with no injected context still gets a (disconnected) root span instead of
    raising.
    """
    if not isinstance(meta, dict):
        return otel_context.get_current()
    return _meta_propagator.extract(carrier=meta)
