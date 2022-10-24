#!/usr/bin/env python3

# jaeger_example.py
from opentelemetry import trace
from opentelemetry.exporter.jaeger.thrift import JaegerExporter
from opentelemetry.trace import SpanKind
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.sdk.resources import Resource

resource = Resource(attributes={
    "service.name": "my_service"
})

trace.set_tracer_provider(TracerProvider(resource=resource))

jaeger_exporter = JaegerExporter(
    collector_endpoint="http://localhost:14278/api/traces",
)

trace.get_tracer_provider().add_span_processor(
    BatchSpanProcessor(jaeger_exporter)
)

tracer = trace.get_tracer(__name__)

with tracer.start_as_current_span("foo", kind=SpanKind.SERVER):
    with tracer.start_as_current_span("bar", kind=SpanKind.SERVER):
        with tracer.start_as_current_span("baz", kind=SpanKind.SERVER):
            print("Hello world from OpenTelemetry Python!")
