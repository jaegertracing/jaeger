# jaeger_storage

This module implements an extension that allows to configure different storage backends that implement Jaeger Storage API. The usual OpenTelemetry Collector's pipeline of receiver-processor-exporter does not require this because each exporter can be supporting just one storage backend, and many of then can be wired up via the pipeline. But in order to run `jaeger-query` service, which is not a part of the pipeline, we need the ability to access shared storage backend implementations, and this extension serves as such shared container.

See also https://www.jaegertracing.io/docs/latest/storage/.

Multi-tenancy is **not** configured on this extension. Tenant validation lives on `jaeger_query` / `remote_storage`; only some backends (memory partitions, gRPC client header forwarding) consume the tenant from context. See [Multi-tenancy in Jaeger v2](../../../docs/multi-tenancy.md).
