# jaeger_storage_exporter

This module implements `exporter.Traces` and writes spans into Jaeger native `spanstore.SpanWriter`, that it obtains from the [jaeger_storage](../../extension/jaegerstorage/) extension. This is primarily needed to wire a memory storage into the exporter pipeline (used for all-in-one), but the design of the exporter is such that it can do this for any V1 storage implementation.

## Configuration

```yaml
exporters:
  jaeger_storage_exporter:
    trace_storage: memstore

extensions:
  jaeger_storage:
    memory:
      memstore:
        max_traces: 100000
```
