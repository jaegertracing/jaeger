# Jaeger Self-Assessment

This document is a local copy of the Jaeger project's security self-assessment, originally conducted following the CNCF TAG Security assessment process.

## Project Overview

Jaeger is a distributed tracing system originally developed at Uber Technologies and now a graduated project within the Cloud Native Computing Foundation (CNCF).

## Security Profile

| Attribute | Value |
| -- | -- |
| Security Policy | [SECURITY.md](https://github.com/jaegertracing/jaeger/blob/main/SECURITY.md) |
| Threat Model | [threat-model.md](threat-model.md) |
| Assurance Case | [assurance-case.md](assurance-case.md) |
| Security file | [SECURITY.md](https://github.com/jaegertracing/jaeger/blob/main/SECURITY.md) |

## Self-Assessment Summary

### Secure Design Principles

Jaeger adheres to established secure design principles:
- **Economy of Mechanism**: Uses standard protocols (OTLP, gRPC).
- **Fail-Safe Defaults**: When TLS is configured, certificate verification is on unless `insecure_skip_verify` is set. TLS itself is opt-in. Collector `configtls` defaults the minimum version to TLS 1.2.
- **Open Design**: Fully open-source and publicly documented.

### Trust Boundaries

Trust boundaries exist between instrumented applications and the receivers, between the process and storage, between the query/UI surface and users, and between that same HTTP server and MCP or AI clients. TLS and authentication are collector configuration. They are not on in the embedded all-in-one config. The v2 binary does not implement RBAC, OAuth2, or Kerberos. See [threat-model.md](threat-model.md).

### Security Testing

- **Unit/Integration Tests**: Comprehensive test suite with high coverage requirements.
- **Static Analysis**: Uses `golangci-lint` and `gosec`.
- **Dependency Scanning**: Daily scans via Dependabot.
- **Vulnerability Reporting**: Formal process documented in `SECURITY.md`.

## Metadata

| Attribute | Details |
| -- | -- |
| Last Updated | 2026-10-11 |
| Status | Local summary aligned with the v2 threat model. This edit is not a new CNCF TAG Security assessment. |
| Assessment Process | CNCF TAG Security Self-Assessment (historical). The assessment linked from `SECURITY-INSIGHTS.yml` was not changed here. |

## Vulnerability Handling

Refer to [SECURITY.md](https://github.com/jaegertracing/jaeger/blob/main/SECURITY.md) and [Report Security Issue](https://www.jaegertracing.io/report-security-issue/).
