# jaeger_storage

This module implements an extension that allows to configure different storage backends that implement Jaeger Storage API. The usual OpenTelemetry Collector's pipeline of receiver-processor-exporter does not require this because each exporter can be supporting just one storage backend, and many of then can be wired up via the pipeline. But in order to run `jaeger-query` service, which is not a part of the pipeline, we need the ability to access a shared storage backend implementations, and this extension serves as such shared container.

See also https://www.jaegertracing.io/docs/latest/storage/.
