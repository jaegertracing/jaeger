import os
import asyncio
from fastapi import FastAPI
from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

app = FastAPI(title="Backend Service")

def setup_tracing():
    resource = Resource.create({
        "service.name": os.getenv("OTEL_SERVICE_NAME", "backend"),
        "service.version": "1.0.0"
    })
    
    provider = TracerProvider(resource=resource)
    trace.set_tracer_provider(provider)
    
    otlp_exporter = OTLPSpanExporter(
        endpoint=os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://jaeger:4317"),
        insecure=True
    )
    
    span_processor = BatchSpanProcessor(otlp_exporter)
    provider.add_span_processor(span_processor)
    
    FastAPIInstrumentor().instrument_app(app)
    
    return trace.get_tracer(__name__)

tracer = setup_tracing()

@app.get("/")
async def root():
    with tracer.start_as_current_span("root-handler"):
        return {"message": "Hello from backend"}

@app.get("/test")
async def test():
    with tracer.start_as_current_span("test-handler") as span:
        span.set_attribute("test.value", "success")
        await asyncio.sleep(0.1)
        return {"status": "test completed"}

@app.get("/health")
async def health():
    return {"status": "healthy"}
