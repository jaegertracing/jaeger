# Jaeger Security Architecture

This document outlines the security architecture of Jaeger, focusing on cryptographic practices, input validation, and system hardening.

## TLS and Cryptographic Practices

The v2 binary is one OpenTelemetry Collector process (`cmd/jaeger`). TLS is configured with collector `configtls` blocks on gRPC and HTTP receivers, the `jaeger_query` HTTP and gRPC servers, storage clients, Kafka clients, and the remote-storage gRPC server. The v1 agent, collector, query, and ingester commands are not in this tree. A Kafka ingest path is an optional pipeline (`kafkareceiver` / `kafkaexporter`), not a separate ingester.

Jaeger Thrift `thrift_compact` and `thrift_binary` are UDP. They do not use TLS.

TLS is off in the embedded all-in-one config. Turning it on is an operator setting.

### TLS Configuration

- **Certificate Management**:
    - **CA Certificate**: Can be provided to verify the server's or client's certificate.
    - **Server Certificate and Key**: Required for enabling TLS on servers.
    - **Client Authentication (mTLS)**: Server TLS accepts a client CA (`client_ca_file`). Clients then have to present a certificate.
- **Reloading Certificates**: `reload_interval` on the collector TLS config re-reads certificate files without a process restart.
- **Helper CLIs**: `es-index-cleaner` and `es-rollover` still use the `--es.tls.*` flags, including `--es.tls.min-version`. The `jaeger` binary does not take `--tls.min-version`.

### TLS Version

When `min_version` is unset, collector `configtls` uses TLS 1.2 as the minimum. Production configurations should leave that default or set `min_version: "1.3"`. The field still accepts `"1.0"` and `"1.1"`. Those versions have known weaknesses and **must not be enabled in production**.

### Algorithm Support

Jaeger uses Go's standard `crypto/tls` implementation for TLS. Operators can configure the allowed cipher suites to ensure only strong cryptographic algorithms are used in their deployment.

### Secure Defaults

- **Certificate Verification**: When TLS is enabled, certificate verification is performed by default.
- **Insecure Communication**: Users must explicitly set `insecure: true` or `insecure_skip_verify: true` to bypass security controls, which is strongly discouraged for production environments.

### Certificate Verification

Jaeger verifies TLS certificates during the TLS handshake before application data is transmitted. Certificate verification is enabled by default when TLS is configured, and operators must explicitly opt out with insecure settings such as `insecure_skip_verify`.

## Input Validation

Receivers and the query API reject payloads that do not unmarshal, and they bound request size. That is not a guarantee against hostile but well-formed span contents.

- **OTLP, Jaeger, Zipkin, Kafka**: Inbound telemetry is unmarshaled by the OpenTelemetry Collector receivers compiled into `cmd/jaeger/internal/components.go` (OTLP gRPC/HTTP, Jaeger gRPC and Thrift, Zipkin HTTP, Kafka). Malformed payloads fail in the receiver.
- **Size Limits**:
    - **gRPC**: Receivers and the query gRPC server use the collector gRPC message limits.
    - **HTTP**: Collector HTTP servers default `max_request_body_size` to 20 MiB, including `jaeger_query`. The AI chat handler has its own default of 1 MiB (`ai.max_request_body_size`).
- **Storage Queries**: Elasticsearch/OpenSearch, Cassandra, ClickHouse, and the gRPC storage client build requests through their own client libraries. User-controlled span values still reach those backends as data.
- **Query API**: HTTP `/api/` and `/api/v3/`, plus gRPC query v2 and v3, read stored traces. They do not redact attributes. Optional `query_interceptors` can, and none are registered by default.
- **UI**: The embedded UI renders span data in the browser. Ordinary text goes through React. GenAI Markdown disables raw HTML, drops images, and allows links only for `http`, `https`, and `mailto`. Remote GenAI media is not fetched until the user asks to show it. The static handler does not send a `Content-Security-Policy` header. Trace JSON dropped on the search page is parsed only in the browser; it is not posted to Jaeger. See the [threat model](threat-model.md) for the attack side of this.
- **MCP**: `/api/ai/mcp/` returns query results to the caller. `read_skill` only opens the embedded skill tree and, when configured, `custom/` under `ai.mcp.skills_dir`, and it rejects paths that fail `fs.ValidPath`. Search and span-detail tools are capped (100 hits, 20 spans, 512 KiB skill files).

## System Hardening

Jaeger is designed to be deployed in a hardened manner.

### Container Security

- **Minimal Base Images**: Official `jaeger` images are built `FROM` the release base image (alpine or scratch, selected by the image build).
- **Non-Root User**: `cmd/jaeger/Dockerfile` sets `USER` to uid/gid 10001.
- **Debug listeners**: The embedded all-in-one config enables healthcheck, expvar, and zpages. `cmd/jaeger/config.yaml` also enables pprof on `0.0.0.0:1777`. Those are process-introspection ports, not trace APIs, and they should not be exposed on an untrusted network. The official image sets `JAEGER_LISTEN_HOST=0.0.0.0`, which binds the embedded debug and receiver ports on all interfaces.

### Dependency Management

- **Vulnerability Scanning**: The project uses Dependabot for automated dependency monitoring and daily vulnerability scans.
- **Software Bill of Materials (SBOM)**: An SBOM is generated for each release to provide transparency into the included components.

### Secure Build Pipeline

- **Signed Commits**: Contributions require a DCO `Signed-off-by` trailer. Release tags and artifacts are GPG-signed. This document does not claim that every commit is GPG-signed.
- **GitHub Actions Security**: The build pipeline uses security features like `harden-runner` to monitor and restrict network access during the build process.
- **Release Signing**: All release artifacts and Git tags are GPG-signed by the maintainers.

## Authentication

Listener authentication is the collector server `auth.authenticator` setting on receivers, on `jaeger_query` HTTP and gRPC, on the `remote_sampling` HTTP and gRPC servers, and on the `remote_storage` gRPC server. Those servers are started with collector `ToServer`, which resolves that authenticator. The binary includes `basicauth` (htpasswd) as a server authenticator and as a client authenticator, and `sigv4auth` as a client authenticator for outbound AWS requests. No OAuth2, OIDC, Kerberos, or RBAC extension is compiled in. A custom distribution can add collector extensions via `cmd/jaeger/builder.yaml`; that is not the default binary.

`jaeger_query.bearer_token_propagation` copies an incoming bearer token onto storage calls. It does not accept or reject the caller. `multi_tenancy`, when enabled, requires the tenant header (default `x-tenant`). If `tenants` is non-empty, the value must be in that list. If `tenants` is omitted or empty, any non-empty value is accepted. It is not authentication.

The MCP endpoint is a route on the query HTTP server. It does not have its own authenticator. The embedded all-in-one config enables it (`ai.mcp: {}`) and does not set query `auth`.

## Credential Management

- **No Hardcoded Credentials**: The `jaeger` binary does not embed production passwords or tokens. Storage passwords, TLS keys, htpasswd contents, and `ai.agent_headers` come from configuration, files, or environment expansion. Checked-in example configs under `cmd/jaeger/` are examples: some set `tls.insecure: true` and sample Cassandra credentials, and those values are not secure defaults.
- **Environment Variables**: Recommended for providing sensitive information in containerized environments.
- **Secure Storage**: Users are encouraged to use secure secret management systems (like Kubernetes Secrets or HashiCorp Vault) to manage Jaeger's credentials.
- **Credential Agility**: TLS certificates, private keys, passwords, and tokens are provided outside the compiled binary. Operators can rotate these credentials by updating referenced files, configuration, environment variables, or secret-manager entries.

## References

- [Threat model](threat-model.md)
- [Securing Jaeger Installation](https://www.jaegertracing.io/docs/latest/security/)
- [OpenTelemetry TLS Configuration](https://github.com/open-telemetry/opentelemetry-collector/blob/main/config/configtls/README.md)
