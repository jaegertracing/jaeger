# Jaeger Security Assurance Case

This document provides a security assurance case for the Jaeger project, demonstrating how security requirements are met through the application of secure design principles and mitigation of common implementation weaknesses.

## Table of Contents

- [Threat Model Summary](#threat-model-summary)
- [Trust Boundaries](#trust-boundaries)
- [Secure Design Principles](#secure-design-principles)
- [Common Weakness Mitigations](#common-weakness-mitigations)
- [Security Controls](#security-controls)

## Threat Model Summary

Jaeger is a distributed tracing system that collects, stores, and visualizes trace data from instrumented applications. The primary security concerns are:

1. **Data Confidentiality**: Trace data may contain sensitive information (service names, endpoints, timing data)
2. **Data Integrity**: Trace data should not be tampered with
3. **Availability**: The tracing infrastructure should not become a DoS vector
4. **Access Control**: Only authorized users should access trace data

### Threat Actors

| Actor | Motivation | Capability |
| -- | -- | -- |
| Malicious Internal Service | DoS, data injection | Network access to collector |
| External Attacker | Data exfiltration, reconnaissance | Varies based on deployment |
| Unauthorized User | Access to sensitive traces | UI/API access |

For detailed threat analysis, see [threat-model.md](threat-model.md).

## Trust Boundaries

```
┌─────────────────────────────────────────────────────────────────┐
│                    External Network                              │
│  ┌──────────────┐                                               │
│  │ Instrumented │                                               │
│  │ Applications │ ─────────── BOUNDARY 1 ───────────────────┐   │
│  │ (OTel SDK)   │                                           │   │
│  └──────────────┘                                           ▼   │
│                                                    ┌────────────┤
│                                                    │  Jaeger    │
│                                                    │  Collector │
│                                                    └─────┬──────┤
│                                                          │      │
│                              ─────── BOUNDARY 2 ─────────┤      │
│                                                          ▼      │
│                                                    ┌────────────┤
│                                                    │  Storage   │
│                                                    │  Backend   │
│                                                    └─────┬──────┤
│                                                          │      │
│                              ─────── BOUNDARY 3 ─────────┤      │
│                                                          ▼      │
│  ┌──────────────┐                                 ┌────────────┤
│  │   Users      │ ─────────── BOUNDARY 4 ────────▶│   Jaeger   │
│  │  (Browser)   │                                 │   Query/UI │
│  └──────────────┘                                 └────────────┤
└─────────────────────────────────────────────────────────────────┘
```

| Boundary | From | To | Security Controls |
| -- | -- | -- | -- |
| 1 | OTel SDK | Collector | Configurable TLS/mTLS on TCP/gRPC/HTTP (no TLS on UDP Thrift); rate limiting is opt-in (not default) |
| 2 | Collector | Storage | TLS, authentication |
| 3 | Storage | Query | TLS, authentication |
| 4 | Users | Query/UI/MCP | No default TLS or auth; basicauth is shipped server authenticator; bearer tokens forwarded to storage (not validated); production auth expected at reverse proxy |

## Secure Design Principles

### Economy of Mechanism

- **Implementation**: Jaeger leverages established protocols (OTLP, gRPC) rather than custom implementations
- **Evidence**: Uses OpenTelemetry Collector framework for core functionality

### Fail-Safe Defaults

- **Implementation**: TLS certificate verification is enabled by default when TLS is configured
- **Evidence**: `insecure_skip_verify` must be explicitly set to disable verification
- **Note**: The embedded configuration leaves TLS and authentication disabled by default, binds the query server to `:16686`, and enables the `/api/ai/mcp/` endpoint. For production, TLS and authentication (or an authenticating reverse proxy) must be configured.

### Complete Mediation

- **Implementation**: Endpoints do not authenticate callers by default; authentication is enforced when configured on the listener or by a fronting reverse proxy
- **Evidence**: `basicauth` server authenticator support; bearer tokens are forwarded to storage without validation; no in-process RBAC

### Open Design

- **Implementation**: All source code is publicly available on GitHub
- **Evidence**: Apache 2.0 license, public security documentation

### Separation of Privilege

- **Implementation**: Different components (Collector, Query) can be deployed with different access levels
- **Evidence**: Collector only writes, Query only reads from storage

### Least Privilege

- **Implementation**: Storage credentials can be scoped to minimum required permissions
- **Evidence**: Separate read/write keyspaces supported for Cassandra

### Least Common Mechanism

- **Implementation**: Admin endpoints separated from data endpoints
- **Evidence**: Separate ports for admin, metrics, and data APIs

### Psychological Acceptability

- **Implementation**: Security is configurable via standard YAML configuration
- **Evidence**: Consistent TLS configuration across all components

## Common Weakness Mitigations

### OWASP Top 10 / CWE Top 25 Coverage

| Weakness | Mitigation |
| -- | -- |
| **Injection (CWE-89, CWE-79)** | Structured data formats (protobuf/OTLP), parameterized storage queries |
| **Broken Authentication (CWE-287)** | `basicauth` authenticator; production authentication expected at a reverse proxy/gateway; bearer tokens forwarded to storage (not validated by Jaeger) |
| **Sensitive Data Exposure (CWE-200)** | TLS for all communications, no credentials in traces |
| **XML External Entities** | Not applicable - uses protobuf/JSON |
| **Broken Access Control (CWE-284)** | No in-process RBAC. Multi-tenancy header checks enforce tenant presence, but an omitted/empty `tenants` list accepts any non-empty header (tenant partitioning, not authentication) |
| **Security Misconfiguration** | Secure defaults where possible, configuration validation |
| **Cross-Site Scripting (CWE-79)** | UI built with React (auto-escaping text); GenAI Markdown disables raw HTML and restricts link schemes. No UI CSP header or meta tag |
| **Insecure Deserialization (CWE-502)** | Uses protobuf with schema validation |
| **Insufficient Logging** | Standard operational logging. No audit log of trace reads; `ui.log_access` is off by default and logs only static asset requests |
| **SSRF (CWE-918)** | No user-controlled URLs in backend requests |

### Go-Specific Security

| Practice | Implementation |
| -- | -- |
| Memory Safety | Go's inherent memory safety |
| Integer Overflow | Go's bounds checking |
| Race Conditions | Go's race detector in CI |
| Dependency Security | Dependabot, daily vulnerability scans |

## Security Controls

### Build and Release

| Control | Implementation |
| -- | -- |
| Signed Commits | DCO required for all contributions |
| Signed Releases | GPG-signed tags and artifacts |
| SBOM | Generated for each release |
| Container Security | Minimal base images (alpine/scratch) |
| Supply Chain | Harden-Runner, pinned dependencies |

### Runtime

| Control | Implementation |
| -- | -- |
| TLS/mTLS | Configurable on HTTP/gRPC listeners and storage backends. Off by default. Not supported on UDP Thrift. No certificate pinning |
| Authentication | `basicauth` via collector `auth.authenticator` on HTTP/gRPC listeners. Production auth expected at reverse proxy. Bearer tokens forwarded to storage, not validated as credentials |
| MCP | `/api/ai/mcp/` enabled on query HTTP port in embedded config; inherits query-port exposure with no separate authenticator |
| Rate Limiting | No request rate limit or per-service quota by default. Optional `memory_limiter` and sampling processors available in custom pipelines |
| Input Validation | OTLP schema validation, size limits |

## References

- [SECURITY.md](../../SECURITY.md) - Vulnerability reporting
- [Threat Model](threat-model.md) - Detailed threat analysis
- [Security Architecture](architecture.md) - Cryptographic practices
- [Security Review 2026](security-review-2026.md) - 2026-05-17 record, plus the 2026-10-11 correction note
- [Securing Jaeger Installation](https://www.jaegertracing.io/docs/latest/security/)
