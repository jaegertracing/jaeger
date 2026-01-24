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
| 1 | OTel SDK | Collector | TLS/mTLS, rate limiting |
| 2 | Collector | Storage | TLS, authentication |
| 3 | Storage | Query | TLS, authentication |
| 4 | Users | Query/UI | TLS, bearer tokens, RBAC |

## Secure Design Principles

### Economy of Mechanism

- **Implementation**: Jaeger leverages established protocols (OTLP, gRPC) rather than custom implementations
- **Evidence**: Uses OpenTelemetry Collector framework for core functionality

### Fail-Safe Defaults

- **Implementation**: TLS certificate verification is enabled by default when TLS is configured
- **Evidence**: `insecure_skip_verify` must be explicitly set to disable verification
- **Note**: TLS itself is opt-in to simplify initial testing and non-production deployments; for all production deployments, TLS (preferably mTLS where supported) MUST be enabled on all external and inter-service connections.

### Complete Mediation

- **Implementation**: All API endpoints require passing through authentication when configured
- **Evidence**: Bearer token and RBAC support at Query service level

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
| **Broken Authentication (CWE-287)** | Bearer tokens, OAuth2, mTLS support |
| **Sensitive Data Exposure (CWE-200)** | TLS for all communications, no credentials in traces |
| **XML External Entities** | Not applicable - uses protobuf/JSON |
| **Broken Access Control (CWE-284)** | RBAC support in Query service |
| **Security Misconfiguration** | Secure defaults where possible, configuration validation |
| **Cross-Site Scripting (CWE-79)** | UI built with React (auto-escaping), CSP headers |
| **Insecure Deserialization (CWE-502)** | Uses protobuf with schema validation |
| **Insufficient Logging** | Comprehensive logging in all components |
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
| TLS/mTLS | Configurable for all connections |
| Authentication | Bearer tokens, OAuth2, Kerberos |
| Rate Limiting | Configurable at collector |
| Input Validation | OTLP schema validation, size limits |

## References

- [SECURITY.md](../../SECURITY.md) - Vulnerability reporting
- [Threat Model](threat-model.md) - Detailed threat analysis
- [Security Architecture](architecture.md) - Cryptographic practices
- [Securing Jaeger Installation](https://www.jaegertracing.io/docs/latest/security/)
