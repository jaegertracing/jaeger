# Jaeger Threat Model

This document describes the threat model for the Jaeger v2 binary in this
repository (`cmd/jaeger`). It is a project threat model, not a penetration
test. A control is listed only when the code implements it or when an operator
turns it on in configuration. The embedded all-in-one config does not enable
TLS or authentication.

Jaeger v1 shipped separate agent, collector, query, and ingester processes.
Those commands are not in this tree. v1-only controls (a `--tls.min-version`
flag on `jaeger`, in-process RBAC, OAuth2, Kerberos, certificate pinning, and
an audit log of trace access) are not part of this model.

## System

With no `--config` flag, `jaeger` loads the embedded all-in-one config
(`cmd/jaeger/internal/all-in-one.yaml`): one process, memory storage, and a
single traces pipeline.

```
┌──────────────────┐     OTLP gRPC/HTTP      ┌─────────────────────────────┐
│ Instrumented     │     Jaeger gRPC/HTTP    │  jaeger (one process)       │
│ applications     │──── Jaeger Thrift UDP ─▶│                             │
│                  │     Zipkin HTTP         │  receivers → batch →        │
│                  │     (Kafka, optional)   │  jaeger storage exporter    │
└──────────────────┘                         │             │               │
                                             │             ▼               │
                                             │  storage backend            │
                                             │  (memory by default)        │
                                             │             │               │
┌──────────────────┐   HTTP :16686           │             ▼               │
│ Browser, API     │◀───────────────────────▶│  jaeger_query               │
│ client, MCP      │   gRPC :16685           │  UI, /api, /api/v3,         │
│ client           │                         │  /api/ai/mcp/               │
└──────────────────┘                         └─────────────────────────────┘
```

Receivers in that default pipeline:

| Receiver | Protocols | Default bind in all-in-one |
| --- | --- | --- |
| `otlp` | gRPC, HTTP | `${JAEGER_LISTEN_HOST:-localhost}:4317` and `:4318` |
| `jaeger` | gRPC, Thrift HTTP, Thrift compact (UDP), Thrift binary (UDP) | localhost `:14250`, `:14268`, `:6831`, `:6832` |
| `zipkin` | HTTP | localhost `:9411` |

`kafkareceiver` and `kafkaexporter` are compiled in and used by
`cmd/jaeger/config-kafka-*.yaml`. They are not in the embedded pipeline.
`nopreceiver` is compiled in and is not a deployment receiver.

The query extension serves, on one HTTP port and one gRPC port:

- The UI (embedded `jaeger-ui` assets).
- Legacy HTTP `/api/` (traces, archive, dependencies, metrics, quality
  metrics, and `POST /api/transform`).
- HTTP `/api/v3/` (services, operations, traces, trace summaries, spans,
  capabilities) and gRPC `jaeger.api_v2.QueryService` and
  `jaeger.api_v3.QueryService`.
- `GET` and `POST` MCP at `/api/ai/mcp/` when `ai.mcp` is set. The embedded
  config sets `mcp: {}`, so this is on by default.
- `/api/ai/chat` and a turn-scoped `/api/ai/mcp/<id>/` only when `ai.agent_url`
  is set. The embedded config does not set it. `cmd/jaeger/config.yaml` does.
- `/api/otlp/v1/*`, a reverse proxy to a fixed OTLP HTTP target, only when
  `otlp_proxy` is set. The embedded config does not set it.
  `cmd/jaeger/config.yaml` points it at `http://127.0.0.1:4318`.

Default query listen addresses come from `ports.PortToHostPort` and are
`:16686` (HTTP) and `:16685` (gRPC). A leading colon binds every interface.
`JAEGER_LISTEN_HOST` does not change those defaults. The official image sets
`JAEGER_LISTEN_HOST=0.0.0.0`, which does change the receivers, remote
sampling, health check, expvar, and zpages endpoints in the embedded config.

Storage backends configured under `jaeger_storage`:

| Trace backend | Notes |
| --- | --- |
| `memory` | Default. Bounded by `max_traces` (100000 in the embedded config). |
| `badger` | Local disk. |
| `elasticsearch`, `opensearch` | HTTP client. TLS and basic, bearer, or API-key auth are config. |
| `cassandra` | CQL. TLS and basic auth are config. |
| `clickhouse` | Native protocol. TLS and basic auth are config. |
| `grpc` | Remote storage client. TLS via the collector gRPC client config. |

Metric backends are Prometheus, Elasticsearch, OpenSearch, and ClickHouse.
The `remote_storage` extension and `cmd/remote-storage` expose a configured
backend over gRPC (default port 17271) for trace and dependency reads.

Other processes still built from this repo (`es-index-cleaner`,
`es-rollover`, `esmapping-generator`, `anonymizer`, `tracegen`) are helper
tools. They are not the query or ingest path.

## Assets

- Trace contents, including whatever applications put in attributes, events,
  and resource fields (secrets, PII, internal hostnames).
- Service and operation names, and the dependency graph.
- Storage credentials, TLS private keys, bearer tokens, and AI sidecar
  headers (`ai.agent_headers` is `configopaque` so it is redacted from
  collector config dumps).
- The query UI session in the browser.

## Trust boundaries

| Boundary | What crosses it | What the code actually does |
| --- | --- | --- |
| **B1: Applications → receivers** | Untrusted spans | OTLP, Jaeger, Zipkin, and Kafka receivers unmarshal structured payloads and enforce transport message limits. TLS and receiver `auth` are off unless configured. Thrift compact and Thrift binary are UDP and have no TLS. The UDP servers bound a packet queue (`queue_size`, default 1000 in the Jaeger receiver). |
| **B2: Pipeline → storage** | Spans and credentials | The storage client uses the backend's own TLS and auth settings. The embedded config uses memory and sends nothing off-box. |
| **B3: Storage → query** | Trace reads | Same client credentials as B2. Separate read and write endpoints exist for the gRPC storage client (`writer`). Cassandra keyspaces can be split. There is no built-in read-only mode for every backend. |
| **B4: Query/UI → users** | HTTP and gRPC | TLS and `auth.authenticator` are collector server settings and are unset by default. The shipped server authenticator is `basicauth` (htpasswd). `sigv4auth` is a client authenticator for outbound AWS calls, not a listener authenticator. |
| **B5: Query → MCP and AI clients** | Trace data and tool calls | `/api/ai/mcp/` is mounted on the query HTTP mux. It has no second authenticator. When `ai.agent_url` is set, the process dials that WebSocket and announces the turn-scoped MCP URL to the sidecar. |
| **B6: Browser rendering** | Span data and uploaded JSON | The UI renders stored traces and files dropped on the search page. See T7 and T8. |
| **B7: Debug listeners** | Process internals | Embedded config enables `healthcheckv2`, `expvar`, and `zpages` on `JAEGER_LISTEN_HOST`. `cmd/jaeger/config.yaml` also enables `pprof` on `0.0.0.0:1777`. Query gRPC registers server reflection. |

Out of scope: instrumented applications, OpenTelemetry SDKs, the internal
implementation of Elasticsearch, Cassandra, ClickHouse, Kafka, and identity
providers, and the network around the process. Jaeger still has to treat
data from those systems as untrusted.

## Actors

| Actor | Capability in a default deployment |
| --- | --- |
| Misconfigured or hostile application | Can open the receiver ports that are bound, and send spans. |
| Anyone who can reach the query port | Can read traces through the UI, `/api/`, `/api/v3/`, and `/api/ai/mcp/`. |
| MCP client or AI agent | Same read access as the query API, plus whatever the sidecar does with tool output. |
| Person who uploads a JSON file in the UI | Executes that file in their own browser. The file is not sent to the server. |
| Operator with the config and disk | Holds storage credentials, TLS keys, and the Badger directory. |
| Network attacker | Sees plaintext on any listener left without TLS, including UDP Thrift. |

## Threats

### T1: Span flooding

A client sends enough spans to exhaust memory, disk, or the storage backend.

The embedded pipeline runs the `batch` processor and stores in memory with
`max_traces: 100000`. Sampling in that config is a file strategy with
`default_sampling_probability: 1`. `memory_limiter`, `tail_sampling`, and
`adaptive_sampling` are compiled in and are not in the embedded pipeline.
Adaptive sampling is on in `cmd/jaeger/config.yaml`. There is no per-service
ingest quota and no general request rate limiter in the binary. Kafka can sit
in front of storage (`config-kafka-collector.yaml` exports to Kafka;
`config-kafka-ingester.yaml` consumes it) but that is a deployment choice,
and those examples do not set Kafka TLS or SASL.

### T2: Secrets and PII in span data

Applications put tokens, passwords, or personal data in attributes. Jaeger
stores and returns that data. It does not redact span contents. Query
interceptors (`jaeger_query.query_interceptors`) can change results before
they are returned; none are registered by default.

Retention is backend-specific. `cmd/es-index-cleaner` and `cmd/es-rollover`
exist for Elasticsearch and OpenSearch. Memory storage drops traces only by
the `max_traces` cap. There is no product-wide retention policy.

### T3: Network interception and tampering

TLS is available on gRPC and HTTP receivers, the query servers, storage
clients, Kafka clients, and the remote-storage server, using collector
`configtls`. It is not configured in the embedded all-in-one file. When a
`tls` block is set and `min_version` is left empty, the collector default
minimum is TLS 1.2. Setting `min_version` to `1.0` or `1.1` is still
accepted by `configtls` and should not be used. `insecure: true` turns client
TLS off. `insecure_skip_verify: true` skips certificate verification. Both
have to be set explicitly. The server does not implement certificate pinning.

Jaeger Thrift over UDP cannot use TLS. Those ports are a plaintext ingest
path whenever they are bound.

`es-index-cleaner` and `es-rollover` still take `--es.tls.min-version` and
the other `--es.tls.*` flags. The `jaeger` binary does not.

### T4: Unauthenticated query and UI access

The query HTTP and gRPC servers do not require credentials unless
`http.auth` or `grpc.auth` names a server authenticator. The binary ships
`basicauth` for that role. Bearer-token support in the query process is
`bearer_token_propagation`: the incoming `Authorization: Bearer` value is
copied onto storage calls. It is not checked. The flag defaults to false.
`header_forwarding` copies other caller headers to storage the same way.
Turned on, either one lets a query client choose a credential the storage
backend may trust.

`multi_tenancy` checks a header (default `x-tenant`) against a configured
tenant list. It is not authentication. A client that knows an allowed tenant
name is accepted.

Nothing in the binary implements RBAC, OAuth2, OIDC, Kerberos, or an audit
log of who read a trace. Static-asset access logging (`ui.log_access`) is
off by default and is not that audit log. gRPC reflection is enabled on the
query gRPC server.

### T5: Storage and remote-storage access

Elasticsearch, OpenSearch, Cassandra, and ClickHouse authenticate only when
their config says so. Several checked-in examples set `tls.insecure: true`
and, for Cassandra, the username and password `cassandra`. Those files are
examples, not the code defaults.

The gRPC `remote_storage` server uses collector gRPC server config. Without
`tls` and `auth`, anyone who can dial it can read the backend it wraps.
Badger data is whatever the process user can read on disk. The container
image runs as uid/gid 10001.

### T6: Supply chain

Dependencies are in `go.mod` / `go.sum` and the UI lockfile. Dependabot runs
on the repo. Releases are GPG-signed and an SPDX SBOM is published. Commits
require a DCO `Signed-off-by` trailer. This model does not claim that every
commit is GPG-signed.

### T7: XSS and content injection from span data

Span fields are attacker-controlled once an application or a file is
accepted. The UI is React, and ordinary attribute text is rendered as text.
Two paths do more:

- GenAI span content can be shown as Markdown. The shared Markdown options
  set `disableParsingRawHTML`, replace images with a text placeholder, and
  allow links only for `http:`, `https:`, and `mailto:` (`jaeger-ui`
  `sharedMarkdownOptions`).
- Image and audio parts may use a `data:` URI or an `http(s)` URL.
  `javascript:` URLs are not treated as loadable. A remote URL is not
  fetched until the user chooses to show it. Images set
  `referrerPolicy="no-referrer"`. Audio elements do not have that attribute.

The query static handler does not set a `Content-Security-Policy` header, and
the UI has no CSP meta tag. Operator UI `linkPatterns` turn span values into
`href`s. A pattern whose template is not a normal web URL can produce a
dangerous link. That config is trusted operator input.

### T8: Trace JSON upload

The search page file control (`FileLoader`) accepts `.json` and `.jsonl` in
the browser. `loadTracesFromFile` parses the file, runs the same trace
transform as API data, and caches it locally. The file is not uploaded to
Jaeger. A hostile file can still trip the rendering paths in T7, or hang the
tab. There is no server-side size limit on that path because the server
never sees the file.

`POST /api/transform` is a different, server-side conversion of an OTLP JSON
body. It is on the query port and subject to the collector HTTP
`max_request_body_size` (20 MiB when unset).

### T9: MCP and AI agent access

`/api/ai/mcp/` serves streamable HTTP MCP on the query port. The embedded
config enables it. Tools are `get_services`, `get_span_names`,
`search_traces`, `get_span_details`, `get_trace_errors`,
`get_trace_topology`, `get_critical_path`, `get_service_dependencies`, and
`read_skill`. They call the same `QueryService` as the UI.

Limits compiled into the handler: 100 search hits, 20 spans of detail per
call, and 512 KiB for a skill file. Idle MCP sessions expire after 5
minutes. `read_skill` rejects paths that fail `fs.ValidPath` (which rejects
`..`) and only opens the embedded skill tree, or `custom/<path>` from
`ai.mcp.skills_dir` when that directory is set.

There is no MCP-specific authentication, allowlist of clients, or filter of
span text before it is returned. Tool arguments and tool results are copied
onto Jaeger's own spans, truncated to 65536 characters, so a query for a
sensitive trace can be exported again in Jaeger's telemetry.

Span text returned to an agent can contain instructions aimed at the model.
Jaeger does not detect that.

When `ai.agent_url` is set, `/api/ai/chat` dials that ACP WebSocket and can
pass `ai.agent_headers`. The chat body limit defaults to 1 MiB. The gateway
also mounts `/api/ai/mcp/<id>/` on the same MCP server and can dispatch
per-turn UI tools into the browser session that opened the chat. Anyone who
can reach the query port can open these routes. The embedded all-in-one
config does not set `agent_url`. `cmd/jaeger/config.yaml` sets it to
`ws://localhost:16688` and also enables `mcp` and `otlp_proxy`.

### T10: OTLP proxy and debug ports

`otlp_proxy.target` is chosen by the operator, not by the request URL, so
this is not an open redirect. It does make the query port an ingest path to
that target for `/api/otlp/v1/*`. Callers who can reach query can submit
spans there.

`expvar` and `zpages` expose process internals. `pprof` exposes profiles and
is present in `cmd/jaeger/config.yaml` on `0.0.0.0:1777`, not in the
embedded config. In the official image, `JAEGER_LISTEN_HOST=0.0.0.0` binds
the embedded expvar, zpages, health, and sampling ports on all interfaces.

## What this model does not claim

The previous revision of this file listed the following as mitigations. They
are not implemented in the v2 binary:

- RBAC inside Jaeger
- OAuth2 or OIDC
- Kerberos
- Certificate pinning
- An audit log of trace reads
- A Content-Security-Policy on the UI
- Per-service resource quotas
- A collector-wide rate limit that is on by default
- TLS on Jaeger Thrift UDP
- Authentication on `/api/ai/mcp/` separate from the query HTTP server

Deployment-layer controls (network policy, a reverse proxy, Kubernetes RBAC
around the namespace) are operator choices. They are not Jaeger features.

## Operator notes

These follow the settings above. They are not a new policy.

1. Treat `:16686` as a read API for every trace the process can see,
   including MCP, unless `http.tls` and `http.auth` (or a proxy in front)
   are configured. `basicauth` is the server authenticator shipped in the
   binary. Remove `ai.mcp` when no MCP client should be able to query.
2. Set `ai.agent_url` and `otlp_proxy` only when those paths should be
   reachable by everyone who can reach the query port.
3. Leave receivers on localhost, or set TLS and `auth`, before binding them
   beyond the host. Do not rely on UDP Thrift for a confidential ingest path.
4. Set storage TLS and credentials in the backend block. Do not copy the
   `insecure: true` example snippets into production.
5. Keep `expvar`, `zpages`, and `pprof` off untrusted networks.
6. Do not put secrets in span attributes. Query interceptors are the
   extension point for redaction; the default build does not redact.
7. For TLS, leave `min_version` unset (TLS 1.2) or set `1.3`. Do not set
   `1.0` or `1.1`. Do not set `insecure` or `insecure_skip_verify`.

## References

- [SECURITY.md](../../SECURITY.md)
- [Security architecture](architecture.md)
- [Assurance case](assurance-case.md)
- [Self-assessment](self-assessment.md)
- [Security review 2026](security-review-2026.md) — 2026-05-17 review
  record, with a 2026-10-11 correction note for controls this model
  does not claim
- Embedded default: `cmd/jaeger/internal/all-in-one.yaml`
- Component list: `cmd/jaeger/internal/components.go`
