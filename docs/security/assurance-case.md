# Jaeger Security Assurance Case

This document states how the Jaeger v2 binary (`cmd/jaeger`) meets its
security goals, and which controls are actually in the process. It
follows [threat-model.md](threat-model.md). A control is listed only
when the code implements it or when an operator turns it on.

The embedded all-in-one config does not enable TLS or authentication.
The query HTTP server defaults to `:16686` on every interface. MCP is
on in that config and uses that same port.

## Table of Contents

- [Threat Model Summary](#threat-model-summary)
- [Trust Boundaries](#trust-boundaries)
- [Secure Design Principles](#secure-design-principles)
- [Common Weakness Mitigations](#common-weakness-mitigations)
- [Security Controls](#security-controls)

## Threat Model Summary

Jaeger is one OpenTelemetry Collector process that receives spans,
stores them, and serves the query API, UI, and (when configured) MCP.
The primary security concerns are:

1. **Data Confidentiality**: Trace data may contain secrets or PII. TLS
   is available and off by default. Jaeger does not redact span contents.
2. **Data Integrity**: Spans can be altered on any path left without TLS.
   Jaeger Thrift compact and Thrift binary are UDP and have no TLS.
3. **Availability**: A client can fill memory, disk, or the storage
   backend. The embedded pipeline does not rate-limit requests.
4. **Access Control**: The query API, UI, and MCP are open unless the
   operator sets server auth or puts a proxy in front.

### Threat Actors

| Actor | Motivation | Capability |
| -- | -- | -- |
| Malicious or misconfigured application | DoS, data injection | Whatever receiver ports are bound |
| External attacker | Read traces, reconnaissance | Reachability of the query port, receivers, or remote-storage gRPC |
| MCP or AI client | Read traces, act on span text | Same read access as the query API when `/api/ai/mcp/` is enabled |
| Unauthorized user | Read sensitive traces | UI, `/api/`, `/api/v3/`, and MCP on the query port |

For the full analysis, see [threat-model.md](threat-model.md).

## Trust Boundaries

```
┌──────────────────┐   OTLP, Jaeger gRPC/HTTP,    ┌────────────────────────┐
│ Instrumented     │   Zipkin; Thrift UDP         │ jaeger (one process)   │
│ applications     │ ──────── BOUNDARY 1 ───────▶ │ receivers → storage    │
└──────────────────┘                              │         │              │
                                                  │         ▼ BOUNDARY 2   │
                                                  │ storage backend        │
┌──────────────────┐   HTTP :16686 / gRPC :16685  │         │              │
│ Browser, API,    │ ◀──── BOUNDARY 4 / 5 ──────▶ │ jaeger_query           │
│ MCP client       │                              │ UI, /api, /api/v3,     │
└──────────────────┘                              │ /api/ai/mcp/           │
                                                  └────────────────────────┘
```

Boundary 3 in older diagrams (storage process to a separate query
process) is the same storage client when ingest and query run together,
which is the embedded default. Separate configs can still split them.

| Boundary | From | To | What the code does |
| -- | -- | -- | -- |
| 1 | Applications | Receivers | gRPC and HTTP can use TLS and `auth.authenticator`. Neither is set in the embedded config. UDP Thrift has no TLS. There is no default request rate limit. |
| 2 | Pipeline | Storage | Backend TLS and credentials when the storage block sets them. The embedded backend is memory and does not leave the process. |
| 3 | Storage | Query | Same client as boundary 2 in the default process. The gRPC storage client can use a different `writer` endpoint. Cassandra can use a separate keyspace. |
| 4 | Users | Query/UI | TLS and `auth` are unset by default. The shipped server authenticator is `basicauth`. A reverse proxy can authenticate in front of the port. Bearer tokens are forwarded to storage when `bearer_token_propagation` is set. They are not validated. There is no in-process RBAC. |
| 5 | MCP clients | Query HTTP | `/api/ai/mcp/` is mounted on the query HTTP server. The embedded config enables it. It has no second authenticator, so it is exposed wherever the query port is exposed. |

## Secure Design Principles

### Economy of Mechanism

- **Implementation**: Ingest uses OTLP, Jaeger, Zipkin, and Kafka via
  OpenTelemetry Collector receivers, not a Jaeger-specific protocol stack.
- **Evidence**: `cmd/jaeger/internal/components.go`.

### Fail-Safe Defaults

- **Implementation**: When TLS is configured, certificate verification is
  on unless `insecure_skip_verify` is set. Collector `configtls` defaults
  `min_version` to TLS 1.2. There is no certificate pinning.
- **Evidence**: `insecure: true` and `insecure_skip_verify: true` are
  explicit. The embedded all-in-one config sets neither TLS nor auth.
- **Note**: TLS is opt-in. Production deployments need TLS on gRPC and
  HTTP listeners and on storage clients. UDP Thrift cannot provide it.

### Complete Mediation

- **Implementation**: Collector server `auth` runs for a listener only
  when that listener's config names an authenticator. The default
  listeners do not.
- **Evidence**: The binary includes `basicauth` (htpasswd) as a server
  authenticator. `sigv4auth` is a client authenticator for outbound AWS
  calls, not for Jaeger's listeners. Bearer-token propagation is not
  authentication. No OAuth2, OIDC, Kerberos, or RBAC extension is
  compiled in. Operators who need another scheme terminate it at a
  reverse proxy, or add an authenticator with `cmd/jaeger/builder.yaml`.
  A custom build is not the default binary.

### Open Design

- **Implementation**: Source and these security documents are public.
- **Evidence**: Apache-2.0 license.

### Separation of Privilege

- **Implementation**: The embedded binary both writes and reads storage.
  Separate configs can run ingest without `jaeger_query`, or query
  against remote gRPC storage.
- **Evidence**: `cmd/jaeger/config-kafka-collector.yaml` exports to Kafka
  and does not start query. `cmd/jaeger/config-query.yaml` reads via the
  gRPC storage client.

### Least Privilege

- **Implementation**: Storage credentials are whatever the operator
  puts in the backend block. They can be scoped in the database.
- **Evidence**: Cassandra config has a keyspace per backend, so archive
  and primary can differ. The gRPC storage client has a separate writer
  endpoint. Jaeger does not enforce a read-only credential itself.

### Least Common Mechanism

- **Implementation**: Receivers, query HTTP, query gRPC, health,
  metrics, expvar, and zpages are different ports.
- **Evidence**: Embedded all-in-one endpoints. `expvar` and `zpages` are
  process-introspection ports. The official image sets
  `JAEGER_LISTEN_HOST=0.0.0.0`, which binds the embedded ones on all
  interfaces. `pprof` is in `cmd/jaeger/config.yaml`, not in the
  embedded config.

### Psychological Acceptability

- **Implementation**: TLS, server auth, storage auth, and MCP are YAML
  on the collector config. MCP is a block under `jaeger_query.ai`.
- **Evidence**: `ai.mcp: {}` enables `/api/ai/mcp/`. Removing the block
  disables it. `http.auth.authenticator` names `basicauth`.

## Common Weakness Mitigations

### OWASP Top 10 / CWE Top 25 Coverage

| Weakness | What Jaeger actually does |
| -- | -- |
| **Injection (CWE-89, CWE-79)** | Receivers unmarshal OTLP, Jaeger, Zipkin, or Kafka payloads. Storage requests go through the Elasticsearch/OpenSearch, Cassandra, ClickHouse, or gRPC clients. Well-formed span values are still stored and returned. |
| **Broken Authentication (CWE-287)** | `basicauth` when a listener's `auth.authenticator` is set. mTLS when server TLS has a client CA. Neither is on by default. Bearer tokens are not checked. OAuth2 and Kerberos are not in the binary. A reverse proxy is the other place to require credentials. |
| **Sensitive Data Exposure (CWE-200)** | TLS is available on gRPC and HTTP and is off in the embedded config. UDP Thrift is plaintext. Span attributes are not redacted. `query_interceptors` can change query results; none are registered by default. |
| **XML External Entities** | Not applicable. Payloads are protobuf or JSON. |
| **Broken Access Control (CWE-284)** | No in-process RBAC. `multi_tenancy` checks a header (default `x-tenant`) against a configured list. Knowing the tenant name is enough. It is not authentication. |
| **Security Misconfiguration** | TLS verification defaults to on once TLS is configured. The embedded config leaves TLS and auth off, enables MCP, and binds query to every interface. Example configs under `cmd/jaeger/` are examples; some set `tls.insecure: true`. |
| **Cross-Site Scripting (CWE-79)** | Ordinary span text is rendered as React text. GenAI Markdown disables raw HTML, replaces images with a placeholder, and allows links only for `http`, `https`, and `mailto`. Remote GenAI media is not fetched until the user asks. There is no `Content-Security-Policy` header and no CSP meta tag. |
| **Insecure Deserialization (CWE-502)** | Receivers reject payloads that do not unmarshal. That is not a separate schema check of every attribute. `POST /api/transform` accepts OTLP JSON up to the query HTTP body limit (20 MiB when unset). |
| **Insufficient Logging** | The process writes operational logs. It does not write an audit log of who read a trace. `ui.log_access` logs static-asset hits and is off by default. |
| **SSRF (CWE-918)** | `otlp_proxy.target` is set by the operator, not by the request URL. Callers who can reach the query port can send spans to that fixed target. The proxy is off in the embedded config. |

### Go-Specific Security

| Practice | Implementation |
| -- | -- |
| Memory Safety | Go's memory safety |
| Integer Overflow | Go's bounds checking |
| Race Conditions | Go's race detector in CI |
| Dependency Security | Dependabot, daily vulnerability scans |

## Security Controls

### Build and Release

| Control | Implementation |
| -- | -- |
| Signed commits | DCO `Signed-off-by` is required. This case does not claim every commit is GPG-signed. |
| Signed releases | GPG-signed tags and artifacts |
| SBOM | Generated for each release |
| Container security | Official image runs as uid/gid 10001. Base image is the release base (alpine or scratch). |
| Supply chain | Harden-Runner, `go.sum`, UI lockfile |

### Runtime

| Control | Implementation |
| -- | -- |
| TLS/mTLS | Collector `configtls` on gRPC and HTTP receivers, query servers, storage clients, Kafka clients, and remote-storage gRPC. Off unless configured. Not available on UDP Thrift. No certificate pinning. |
| Listener authentication | `basicauth` via `auth.authenticator`. Off unless configured. A reverse proxy can authenticate in front of the query port. |
| Bearer tokens | Forwarded to storage when `bearer_token_propagation` is set. Not validated as a query credential. Default is off. |
| MCP | `/api/ai/mcp/` on the query HTTP port when `ai.mcp` is set. Embedded config sets it. Same exposure as query. No separate authenticator. |
| Volume limits | No default request rate limit and no per-service quota. Embedded memory storage uses `max_traces`. `memory_limiter`, tail sampling, and adaptive sampling are compiled in and are not in the embedded pipeline. UDP Thrift has a receiver `queue_size` (default 1000). |
| Input validation | Receiver unmarshal and collector message size limits. Query HTTP body size defaults to 20 MiB. AI chat body defaults to 1 MiB. |

## References

- [SECURITY.md](../../SECURITY.md) — vulnerability reporting
- [Threat model](threat-model.md)
- [Security architecture](architecture.md)
- [Security review 2026](security-review-2026.md) — 2026-05-17 record, plus the 2026-10-11 correction note
