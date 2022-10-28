#!/usr/bin/env python3

from opentelemetry import trace
from opentelemetry.trace import SpanKind
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.sdk.resources import Resource

resource = Resource(attributes={
    "service.name": "my_service"
})

trace.set_tracer_provider(TracerProvider(resource=resource))

otlp_exporter = OTLPSpanExporter(endpoint="http://localhost:4317", insecure=True)


trace.get_tracer_provider().add_span_processor(
    BatchSpanProcessor(otlp_exporter)
)

tracer = trace.get_tracer(__name__)

with tracer.start_as_current_span("foo", kind=SpanKind.SERVER):
    with tracer.start_as_current_span("bar", kind=SpanKind.SERVER):
        with tracer.start_as_current_span("baz", kind=SpanKind.SERVER):
            print("Hello world from OpenTelemetry Python!")
