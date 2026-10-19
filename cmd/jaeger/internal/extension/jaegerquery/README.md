# jaeger_query

This extension implements traditional `jaeger-query` service (including Jaeger UI). Because it is not a part of the collector pipeline, it needs access to a storage backend, which can be potentially shared with an exporter (e.g. in order to implement the `all-in-one` functionality). For this reason it depends on the [jaeger_storage](../jaegerstorage/) extension.

## Configuration

This is work in progress, most of the usual settings of `jaeger-query` are not supported yet.

```yaml
jaeger_query:
    storage:
      traces: some_store
      traces_archive: another_store
      metrics: prometheus_store
    multi_tenancy:
      enabled: false
      # header: x-tenant
      # tenants: [acme, globex]
```

### Multi-tenancy

Optional. When `multi_tenancy.enabled` is `true`, query HTTP and gRPC APIs require a tenant header (default `x-tenant`), validate it against an optional allow-list, and attach the tenant to the request context for storage reads.

See [Multi-tenancy in Jaeger v2](../../../docs/multi-tenancy.md) for the full configuration surface, storage support matrix, and known ingest limitations.
