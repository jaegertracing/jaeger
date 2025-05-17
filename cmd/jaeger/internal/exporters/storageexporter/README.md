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

The exporter supports configurable retry behavior for failed exports. By default, retry is enabled with the following settings:

- `enabled`: true
- `initial_interval`: 5s
- `randomization_factor`: 0.5
- `multiplier`: 1.5
- `max_interval`: 30s
- `max_elapsed_time`: 5m

#### Explanation of Retry Parameters

- `enabled`: Controls whether retry is enabled or disabled
- `initial_interval`: The initial time to wait before the first retry
- `randomization_factor`: Randomization factor to apply to the backoff interval (0.0 to 1.0)
- `multiplier`: Factor to multiply the interval by for each subsequent retry
- `max_interval`: The maximum interval between retries
- `max_elapsed_time`: The maximum total time spent retrying before giving up

You can customize the retry behavior like this:

```yaml
exporters:
  jaeger_storage_exporter:
    trace_storage: memstore
    retry_on_failure:
      enabled: true
      initial_interval: 10s
      max_interval: 1m
      max_elapsed_time: 10m
    queue:
      enabled: true
      num_consumers: 10
      queue_size: 1000
```

To disable retry:

```yaml
exporters:
  jaeger_storage_exporter:
    trace_storage: memstore
    retry_on_failure:
      enabled: false
```
