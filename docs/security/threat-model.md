# Jaeger Threat Model

This document describes the threat model for the Jaeger distributed tracing system.

## Overview

Jaeger is a distributed tracing platform that collects, processes, and visualizes trace data from instrumented applications. This threat model identifies potential threats and the controls implemented to mitigate them.

## System Architecture

```
┌─────────────────┐      ┌─────────────────┐      ┌─────────────────┐
│  Instrumented   │      │     Jaeger      │      │    Storage      │
│  Applications   │─────▶│    Collector    │─────▶│    Backend      │
│  (OTel SDK)     │      │                 │      │ (ES/Cassandra)  │
└─────────────────┘      └─────────────────┘      └────────┬────────┘
                                                           │
                         ┌─────────────────┐               │
                         │     Jaeger      │◀──────────────┘
                         │   Query + UI    │
                         └────────┬────────┘
                                  │
                         ┌────────▼────────┐
                         │      Users      │
                         │    (Browser)    │
                         └─────────────────┘
```

## Trust Boundaries

| Boundary | Description | Security Controls |
| -- | -- | -- |
| **B1: SDK → Collector** | External applications sending spans | TLS/mTLS, rate limiting, schema validation |
| **B2: Collector → Storage** | Internal service to database | TLS, authentication, authorized credentials |
| **B3: Storage → Query** | Database to internal service | TLS, authentication, read-only access |
| **B4: Query → Users** | Internal service to end users | TLS, bearer tokens, RBAC |

## Threat Actors

| Actor | Description | Motivation |
| -- | -- | -- |
| **Malicious Application** | Compromised or rogue service sending traces | Data poisoning, DoS, information injection |
| **External Attacker** | Attacker with network access | Data exfiltration, reconnaissance, DoS |
| **Malicious Insider** | User with legitimate access | Unauthorized data access, privilege escalation |
| **Man-in-the-Middle** | Attacker on network path | Data interception, tampering |

## Threats and Mitigations

### T1: Denial of Service via Span Flooding

**Description**: Malicious or misconfigured application sends excessive spans.

| Attribute | Value |
| -- | -- |
| Threat Actor | Malicious Application |
| Impact | High - Can overwhelm collector and storage |
| Likelihood | Medium |

**Mitigations**:
- Rate limiting at collector
- Adaptive sampling to reduce volume
- Resource quotas per service
- Kafka buffering for burst handling

### T2: Sensitive Data Exposure in Traces

**Description**: Traces may inadvertently contain sensitive data (PII, credentials).

| Attribute | Value |
| -- | -- |
| Threat Actor | External Attacker, Malicious Insider |
| Impact | High - Data breach |
| Likelihood | Medium |

**Mitigations**:
- TLS encryption for all connections
- Access control (RBAC) on Query service
- Data retention policies
- Guidance for users on what not to trace

### T3: Man-in-the-Middle Attack

**Description**: Attacker intercepts unencrypted trace traffic.

| Attribute | Value |
| -- | -- |
| Threat Actor | Man-in-the-Middle |
| Impact | High - Data interception and tampering |
| Likelihood | Low (with TLS) |

**Mitigations**:
- TLS/mTLS for all communications
- Certificate verification enabled by default
- Certificate pinning optional

### T4: Unauthorized Access to Trace Data

**Description**: Unauthorized user accesses the Query UI/API.

| Attribute | Value |
| -- | -- |
| Threat Actor | External Attacker, Malicious Insider |
| Impact | Medium - Information disclosure |
| Likelihood | Medium |

**Mitigations**:
- Bearer token authentication
- OAuth2 integration
- RBAC for access control
- Audit logging

### T5: Storage Backend Compromise

**Description**: Attacker gains access to the storage backend directly.

| Attribute | Value |
| -- | -- |
| Threat Actor | External Attacker |
| Impact | High - Full data access |
| Likelihood | Low |

**Mitigations**:
- Storage-level authentication
- Network isolation
- Encrypted connections to storage
- Storage-level access controls

### T6: Supply Chain Attack

**Description**: Compromised dependency introduced into build.

| Attribute | Value |
| -- | -- |
| Threat Actor | External Attacker |
| Impact | Critical - Code execution |
| Likelihood | Low |

**Mitigations**:
- Dependabot vulnerability scanning
- Signed commits (DCO)
- GPG-signed releases
- SBOM generation
- Pinned dependencies with checksums

## Security Recommendations

### For Operators

1. **Enable TLS everywhere** - Use `tls.insecure: false` for all connections
2. **Use mTLS** - Especially for collector ingestion
3. **Configure authentication** - Enable bearer tokens or OAuth2
4. **Set up RBAC** - Limit who can access trace data
5. **Enable audit logging** - Track access to sensitive traces
6. **Use network segmentation** - Isolate Jaeger components

### For Developers Instrumenting Applications

1. **Never trace credentials** - Avoid logging passwords, tokens, API keys
2. **Sanitize PII** - Don't include personal information in spans
3. **Use sampling** - Reduce volume and exposure
4. **Review span content** - Audit what data is being traced

## References

- [SECURITY.md](../../SECURITY.md) - Security policy and vulnerability reporting
- [Security Architecture](architecture.md) - Cryptographic practices
- [Assurance Case](assurance-case.md) - Security assurance case
- [Securing Jaeger Installation](https://www.jaegertracing.io/docs/latest/security/)
- [OpenSSF Threat Modeling Standards](https://github.com/ossf/security-insights-spec/tree/main/docs/threat-model)
