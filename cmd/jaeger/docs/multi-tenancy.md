# Multi-tenancy in Jaeger v2

This document describes how multi-tenancy is configured and applied in Jaeger v2,
and what is (and is not) supported today. It answers [#6108](https://github.com/jaegertracing/jaeger/issues/6108).

## Summary

| Surface | Config key | Role |
| --- | --- | --- |
| `jaeger_query` extension | `multi_tenancy` | Require/validate tenant header on query HTTP and gRPC APIs; put tenant on request context |
| `remote_storage` extension | `multi_tenancy` | Same enforcement on the gRPC remote-storage server |
| `jaeger-remote-storage` binary | `multi_tenancy` | Standalone remote-storage process (same options) |
| gRPC storage backend | `backends.<name>.grpc.multi_tenancy` | Client: forward tenant from context to the remote storage server |
| Memory storage | _(no config)_ | Partitions data by `tenancy.GetTenant(ctx)` (empty string = default tenant) |
| Other storage backends | — | Do **not** partition by Jaeger tenant context |
| Ingestion receivers / `jaeger_storage_exporter` | — | Do **not** extract or attach tenant from request headers |

There is **no** top-level or `jaeger_storage`-level tenancy block in v2. Tenancy is configured on the components that speak HTTP/gRPC to clients, not on the shared storage extension.

Shared option type: [`internal/tenancy.Options`](../../../internal/tenancy/manager.go).

```go
type Options struct {
    Enabled bool     // require a tenant header
    Header  string   // default "x-tenant"
    Tenants []string // optional allow-list; empty = any non-empty tenant
}
```

## Configuration

### Query (`jaeger_query`)

Wire under the query extension ([`QueryOptions.Tenancy`](../internal/extension/jaegerquery/internal/flags.go)):

```yaml
extensions:
  jaeger_query:
    storage:
      traces: some_store
    multi_tenancy:
      enabled: true
      header: x-tenant          # optional, default x-tenant
      tenants:                  # optional allow-list
        - acme
        - globex
```

When `enabled: true`:

* HTTP API: [`ExtractTenantHTTPHandler`](../../../internal/tenancy/http.go) rejects requests missing the header or using an unknown tenant, and stores the tenant on the request context.
* gRPC API: [`NewGuardingUnaryInterceptor` / `NewGuardingStreamInterceptor`](../../../internal/tenancy/grpc.go) do the same for gRPC metadata.
* Manager is created at query startup: [`jaegerquery/server.go`](../internal/extension/jaegerquery/server.go).

Clients (including the UI via reverse proxy) must send the tenant header on every query, for example:

```http
GET /api/services HTTP/1.1
x-tenant: acme
```

### Remote storage server

**Extension** ([`remotestorage.Config`](../internal/extension/remotestorage/config.go)):

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
    multi_tenancy:
      enabled: true
      header: x-tenant
      tenants: [acme, globex]
```

**Standalone binary** (`cmd/remote-storage`, see its [README](../../remote-storage/README.md)):

```yaml
grpc:
  endpoint: :17271
multi_tenancy:
  enabled: true
  header: x-tenant
  tenants:
    - acme
    - globex
storage:
  backends:
    default-storage:
      memory:
        max_traces: 100000
```

Both paths build a tenancy manager and install the guarding gRPC interceptors so every storage RPC carries a validated tenant on the context.

### gRPC storage client (caller side)

When Jaeger talks to a remote storage server, enable multi-tenancy on the **client** so the tenant from context is copied into outgoing metadata ([`internal/storage/v2/grpc`](../../../internal/storage/v2/grpc/factory.go)):

```yaml
extensions:
  jaeger_storage:
    backends:
      some_store:
        grpc:
          endpoint: localhost:17271
          tls:
            insecure: true
          multi_tenancy:
            enabled: true
            header: x-tenant
```

Without this, a multi-tenant remote-storage server will reject calls that only have a context tenant and no header.

### Memory storage partitioning

Memory storage always keys data by `tenancy.GetTenant(ctx)` ([`WriteTraces` and readers](../../../internal/storage/v2/memory/memory.go)). There is no separate tenancy toggle: with no tenant on the context, data lands in the `""` (default) partition. `max_traces` is applied **per tenant**.

Elasticsearch, OpenSearch, Cassandra, Badger, ClickHouse, and Kafka backends do not read the Jaeger tenant context for data isolation. Enabling query-side tenancy alone does **not** give storage isolation on those backends.

## Request flow

```text
Query path (supported)
  Client --[x-tenant]--> jaeger_query (validate + WithTenant)
                      --> storage reader (memory uses tenant; grpc client forwards header)

Write / ingest path (limited)
  Client --[spans + optional header]--> OTEL receiver
                                     --> batch processor
                                     --> jaeger_storage_exporter (forwards ctx as-is)
                                     --> storage writer
```

The exporter only forwards the pipeline context ([`pushTraces`](../internal/exporters/storageexporter/exporter.go)); it does not call `GetValidTenant` or attach a tenant. Receivers in the default configs do not enable `include_metadata`, and the batch processor does not set `metadata_keys`, so tenant headers from ingest are not reliably available downstream. Even if OTEL client metadata were preserved, the storage writer path currently looks at Jaeger's direct context value (`tenancy.GetTenant`), not OTEL metadata, except where gRPC guarding interceptors promote metadata into context (query / remote-storage servers).

Practical consequence: multi-tenant **query** enforcement works; multi-tenant **ingest isolation** through the default `receivers → batch → jaeger_storage_exporter` pipeline does not, today. End-to-end tenant isolation with the batch processor still depends on OpenTelemetry Collector support for propagating selected metadata across async export (see [open-telemetry/opentelemetry-collector#2495](https://github.com/open-telemetry/opentelemetry-collector/pull/2495)).

## Where should tenancy live?

Issue #6108 asks whether tenancy should be part of storage config. Current layout:

* **Configured at the API edge** (`jaeger_query`, `remote_storage` / `jaeger-remote-storage`): validation and context attachment.
* **Consumed by storage** only where the backend understands it (memory partitions; gRPC client forwards).
* **Not** on `jaeger_storage` itself: that extension is a factory container shared by query and exporter, and most backends have no tenant dimension.

A single storage-level toggle would not be enough without (1) ingest-path promotion of the tenant into context across the batch processor, and (2) backend support for isolation. Until those exist, keeping config on the query / remote-storage API surfaces matches the code.

## v1 CLI flags (reference)

The shared package still defines CLI flags used by some non-collector binaries and tests ([`internal/tenancy/flags.go`](../../../internal/tenancy/flags.go)):

```text
--multi-tenancy.enabled
--multi-tenancy.header=x-tenant
--multi-tenancy.tenants=acme,globex
```

Jaeger v2 is configured via YAML (see above). Flag names use dashes; YAML keys use underscores (`multi_tenancy`).

## Related source map

| Piece | Path |
| --- | --- |
| Options + Manager | `internal/tenancy/manager.go` |
| HTTP extract / reject | `internal/tenancy/http.go` |
| gRPC server/client interceptors | `internal/tenancy/grpc.go` |
| Query config | `cmd/jaeger/internal/extension/jaegerquery/internal/flags.go` |
| Query wiring | `cmd/jaeger/internal/extension/jaegerquery/server.go` |
| Query HTTP/gRPC middleware | `cmd/jaeger/internal/extension/jaegerquery/internal/server.go` |
| Storage exporter | `cmd/jaeger/internal/exporters/storageexporter/exporter.go` |
| Memory tenant partitions | `internal/storage/v2/memory/memory.go` |
| gRPC storage client | `internal/storage/v2/grpc/factory.go` |
| Remote storage extension | `cmd/jaeger/internal/extension/remotestorage/` |
| Standalone remote-storage | `cmd/remote-storage/` |
