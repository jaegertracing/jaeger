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
- **Fail-Safe Defaults**: TLS verification enabled by default.
- **Open Design**: Fully open-source and publicly documented.

### Trust Boundaries

Trust boundaries exist between instrumented applications and the collector, between the collector and storage, and between the query service and users. Each boundary is protected by TLS and authentication controls.

### Security Testing

- **Unit/Integration Tests**: Comprehensive test suite with high coverage requirements.
- **Static Analysis**: Uses `golangci-lint` and `gosec`.
- **Dependency Scanning**: Daily scans via Dependabot.
- **Vulnerability Reporting**: Formal process documented in `SECURITY.md`.

## Metadata

| Attribute | Details |
| -- | -- |
| Last Updated | 2026-01-16 |
| Status | Completed |
| Assessment Process | CNCF TAG Security Self-Assessment |

## Vulnerability Handling

Refer to [SECURITY.md](https://github.com/jaegertracing/jaeger/blob/main/SECURITY.md) and [Report Security Issue](https://www.jaegertracing.io/report-security-issue/).
