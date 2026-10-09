# Remote Storage Backend Extension

The Remote Storage Backend Extension exposes any Jaeger-supported storage backend through a gRPC API by
implementing the gRPC Storage API. The gRPC API currently consists of the following services:

* [TraceReader](https://github.com/jaegertracing/jaeger-idl/blob/main/proto/storage/v2/trace_storage.proto)
* [DependencyReader](https://github.com/jaegertracing/jaeger-idl/blob/main/proto/storage/v2/dependency_storage.proto)

## Overview

This extension allows you to make Jaeger storage backends accessible remotely over gRPC.
For example, you can expose the in-memory storage backend like this:

```yaml
extensions:
  jaeger_storage:
    backends:
      memory-storage:
        memory:
          max_traces: 100000
  remote_storage:
    endpoint: localhost:17271
    storage: memory-storage
    # multi_tenancy:
    #   enabled: true
    #   header: x-tenant
    #   tenants: [acme, globex]
```

## Multi-tenancy

Optional `multi_tenancy` on this extension enables guarding gRPC interceptors so each remote-storage RPC must carry a valid tenant header. Callers that use the gRPC storage backend should enable the matching client-side `multi_tenancy` block so the tenant is forwarded from context.

Details: [Multi-tenancy in Jaeger v2](../../../docs/multi-tenancy.md).
