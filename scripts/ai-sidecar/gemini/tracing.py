# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import logging

from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.google_generativeai import GoogleGenerativeAiInstrumentor
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

logger = logging.getLogger(__name__)


def init_tracing(endpoint: str, insecure: bool) -> None:
    """Initialize OpenTelemetry tracing with automatic Gemini instrumentation."""
    resource = Resource.create({"service.name": "jaeger-gemini-sidecar"})
    provider = TracerProvider(resource=resource)
    provider.add_span_processor(
        BatchSpanProcessor(OTLPSpanExporter(endpoint=endpoint, insecure=insecure))
    )
    trace.set_tracer_provider(provider)

    GoogleGenerativeAiInstrumentor().instrument()

    logger.info("Tracing initialized (endpoint=%s, insecure=%s)", endpoint, insecure)


def get_tracer() -> trace.Tracer:
    """Return the sidecar's tracer instance."""
    return trace.get_tracer("jaeger-gemini-sidecar")
