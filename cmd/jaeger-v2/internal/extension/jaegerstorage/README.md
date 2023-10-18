# jaeger_storage

This module implements an extension that allows to configure different storage backends that implement Jaeger's V1 storage APIs. The usual OpenTelemetry Collector's pipeline of receiver-processor-exporter does not require this because each exporter can be supporting just one storage backend, and many of then can be wired up via the pipeline. But in order to run `jaeger-query` service, which is not a part of the pipeline, we need the ability to access a shared storage backend implementations, and this extension serves as such shared container.

## Configuration

The extension can declare multiple storage backend implementations, separating them by storage type first, and then by user-assigned names. Here's an example that may lets [query-service](../jaegerquery/) extension use Cassandra as the main backend, but memory storage as Dependencies store:

```yaml
jaeger_storage:
  memory:
    memstore:
      max_traces: 100000
  cassandra:
    cassandra_primary:
      servers: [...]
      namespace: jaeger
    cassandra_archive:
      servers: [...]
      namespace: jaeger_archive

jaeger_query:
    trace_storage: cassandra_primary
    trace_archive: cassandra_archive
    dependencies: memstore
    metrics_store: prometheus_store
```

NB: this is work in progress, only `memory` section is currently supported.
