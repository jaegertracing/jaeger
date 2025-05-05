# gRPC Remote Storage

Jaeger supports a gRPC-based Remote Storage API that enables integration with custom storage backends not natively supported by Jaeger.

To use a remote storage backend, you must deploy a gRPC server that implements
the following services:

* [TraceReader](https://github.com/jaegertracing/jaeger-idl/tree/main/proto/storage/v2/trace_storage.proto)
* [DependendencyReader](https://github.com/jaegertracing/jaeger-idl/tree/main/proto/storage/v2/dependency_storage.proto)
* [TraceService](https://github.com/open-telemetry/opentelemetry-proto/blob/main/opentelemetry/proto/collector/trace/v1/trace_service.proto). This service can be deployed at a different port.

An example configuration for setting up a remote storage backend is available
[here](https://github.com/jaegertracing/jaeger/blob/main/cmd/jaeger/config-remote-storage.yaml).
Note: In this example, the `TraceService` is configured to run on a different port (0.0.0.0:4316), which is overridden in the config file.
