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
```

## Trust boundary

This port is a trusted interface. It is the same gRPC server as [`jaeger-remote-storage`](../../../../remote-storage/README.md#trust-boundary). The extension's default endpoint is `:17271`, which listens on every interface. Unless `auth` names an authenticator, a caller who can reach the port can read and write the storage backend named by `storage`.

The tenant header is not authentication. `multi_tenancy` identifies the tenant for the call (header default `x-tenant`). It does not authenticate the caller. A client that can reach the port and present a tenant name the server accepts is served.

Deployments must protect the port with mTLS and network policy, so only the Jaeger processes that use the backend can open it. Set `tls` on this extension. `client_ca_file` makes the server require and verify a client certificate.

grpc-go's receive limit applies to every request before decode. It is 4 MB by default, and it is configurable on this server with `max_recv_msg_size_mib` (unset means the grpc-go default). protobuf-go's unmarshal recursion limit bounds how deeply a message may be nested. These are bounds on the payload. They are not a substitute for mTLS or network policy.
