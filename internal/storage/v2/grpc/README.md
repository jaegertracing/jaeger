# gRPC Remote Storage

Jaeger supports a gRPC-based Remote Storage API that enables integration with custom storage backends not natively supported by Jaeger.

To use a remote storage backend, you must deploy a gRPC server at a known address that implements 
the following services:

* [TraceReader](https://github.com/jaegertracing/jaeger-idl/tree/main/proto/storage/v2/trace_storage.proto)
* [DependendencyReader](https://github.com/jaegertracing/jaeger-idl/tree/main/proto/storage/v2/dependency_storage.proto)

An example configuration for setting up a remote storage backend is available
[here](https://github.com/jaegertracing/jaeger/blob/main/cmd/jaeger/config-remote-storage.yaml).
