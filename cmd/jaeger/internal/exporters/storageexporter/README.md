# jaeger_storage_exporter

This module implements `exporter.Traces` and writes spans into Jaeger native `spanstore.SpanWriter`, that it obtains from the [jaeger_storage](../../extension/jaegerstorage/) extension. This is primarily needed to wire a memory storage into the exporter pipeline (used for all-in-one), but the design of the exporter is such that it can do this for any V1 storage implementation.

## Configuration

### Basic Configuration

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

### Retry Configuration

The exporter supports configurable retry behavior for failed exports. By default, retry is disabled. When enabled, the following default settings are used:

- `enabled`: false (disabled by default)
- `initial_interval`: 5s
- `randomization_factor`: 0.5
- `multiplier`: 1.5
- `max_interval`: 30s
- `max_elapsed_time`: 5m

To enable and customize the retry behavior:

```yaml
exporters:
  jaeger_storage_exporter:
    trace_storage: memstore
    retry_on_failure:
      enabled: true  # Explicitly enable retries
      initial_interval: 10s
      max_interval: 1m
      max_elapsed_time: 10m
    queue:
      enabled: true
      num_consumers: 10
      queue_size: 1000
```
