# jaeger_query

This extension implements traditional `jaeger-query` service (including Jaeger UI). Because it is not a part of the collector pipeline, it needs access to a storage backend, which can be potentially shared with an exporter (e.g. in order to implement the `all-in-one` functionality). For this reason it depends on the [jaeger_storage](../jaegerstorage/) extension.

## Configuration

This is work in progress, most of the usual settings of `jaeger-query` are not supported yet.

```yaml
jaeger_query:
    trace_storage: cassandra_primary
    trace_archive: cassandra_archive
    dependencies: memstore
    metrics_store: prometheus_store
```
