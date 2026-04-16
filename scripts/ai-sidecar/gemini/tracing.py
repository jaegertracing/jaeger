# Copyright (c) 2026 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

import logging

from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.grpc.metric_exporter import OTLPMetricExporter
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from traceloop.sdk import Traceloop

logger = logging.getLogger(__name__)


def init_tracing(endpoint: str, insecure: bool) -> None:
    """Initialize OpenTelemetry tracing with OpenLLMetry for automatic Gemini instrumentation."""
    exporter = OTLPSpanExporter(endpoint=endpoint, insecure=insecure)
    metrics_exporter = OTLPMetricExporter(endpoint=endpoint, insecure=insecure)

    Traceloop.init(
        app_name="jaeger-gemini-sidecar",
        exporter=exporter,
        metrics_exporter=metrics_exporter,
        disable_batch=False,
    )

    logger.info("Tracing initialized (endpoint=%s, insecure=%s)", endpoint, insecure)


def get_tracer() -> trace.Tracer:
    """Return the sidecar's tracer instance."""
    return trace.get_tracer("jaeger-gemini-sidecar")
