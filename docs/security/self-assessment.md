# Jaeger Self-Assessment

> This assessment was created by community members as part of the [Security Pals](https://github.com/cncf/tag-security/issues/1102) process. The Jaeger team thanks them for the contribution!
>
> **Canonical version**: [CNCF TAG Security](https://tag-security.cncf.io/community/assessments/projects/jaeger/self-assessment/)

## Table of Contents

- [Metadata](#metadata)
- [Overview](#overview)
- [Security Functions and Features](#security-functions-and-features)
- [Project Compliance](#project-compliance)
- [Secure Development Practices](#secure-development-practices)
- [Security Issue Resolution](#security-issue-resolution)
- [Appendix](#appendix)

## Metadata

|   |  |
| -- | -- |
| Assessment Stage | Complete |
| Software | [Jaeger Repository](https://github.com/jaegertracing/jaeger) |
| Security Provider | No, the main function is to enable distributed tracing |
| Languages | Go, Shell, Makefile, Python, Dockerfile |
| SBOM | Generated with each [release](https://github.com/jaegertracing/jaeger/releases) |

### Security Links

| Doc | URL |
| -- | -- |
| Security file | [SECURITY.md](https://github.com/jaegertracing/jaeger/blob/main/SECURITY.md) |
| Reporting Issues | [Report Security Issue](https://www.jaegertracing.io/report-security-issue/) |
| Security Docs| [Securing Jaeger Installation](https://www.jaegertracing.io/docs/latest/security/) |

## Overview

Jaeger is a cloud native, infinitely scalable, distributed data traversal performance tracker. Jaeger collector paired with a tracing SDK like OpenTelemetry's SDK is used to monitor microservice interactions through traces and "spans" on distributed systems to identify bottlenecks, timeouts and other performance issues.

### Background

Jaeger provides real-time monitoring and analytics of complex ecosystems, which is especially crucial in microservice architectures. When requests are made to microservices, the tracing SDK creates a "span" which is a logical unit of work providing information like start time, end time, and operation metadata. These span components are then used to construct traces which are stored and can be visualized via the centralized dashboard provided by Jaeger.

Jaeger is cross-platform compatible and provides client libraries for a variety of languages and frameworks. With its dashboard UI, users are able to make complex queries and gather insight from collected data.

### Actors

| Actor | Description |
| -- | -- |
| **OpenTelemetry SDK** | Installed on hosts/containers to generate tracing data |
| **Jaeger Collector** | Receives, processes, validates, cleans up/enriches and stores traces |
| **Jaeger Query** | Exposes APIs for receiving traces and hosts the web UI |
| **Jaeger Ingester** | Reads traces from Kafka and writes to a database |

[Architecture Diagram](https://www.jaegertracing.io/docs/latest/architecture/)

### Goals

**General Goals:**
- Distributed tracing across microservices
- Performance monitoring and latency analysis
- Root cause analysis for bottlenecks
- Scalability for large, complex microservices
- Compatibility with OpenTelemetry, multiple storage backends

**Security Goals:**
- Maintain security via data encryption in transit (TLS/mTLS)
- Seamless integration with development and operational workflows

### Non-Goals

- Not designed for real-time alerting or automated notifications
- Does not collect detailed system metrics (CPU, memory, disk I/O)
- No automatic anomaly detection (no ML capabilities)
- Not intended for business analytics
- Not designed for security monitoring or compliance monitoring

## Security Functions and Features

### Critical

#### Encryption
- TLS and mutual TLS (mTLS) supported for all communications
- OpenTelemetry SDK communicates to collector via gRPC/HTTP with TLS option
- Collector, Ingester, and Query support TLS to storage backends (Cassandra, Elasticsearch, Kafka)
- Elasticsearch supports bearer token propagation
- Kafka supports Kerberos and plaintext authentication

### Security Relevant

#### Authentication and Authorization
- Bearer tokens supported for authentication
- OAuth2 tokens for restricted access
- Plaintext and Kerberos authentication supported

#### Access Control
- Role-based access control (RBAC) available
- Administrators can define roles and permissions

#### Security Auditing
- Monitoring and auditing tools for user actions
- Can track logins, searches, and sensitive data access

## Project Compliance

Jaeger does not currently document meeting particular compliance standards.

## Secure Development Practices

### Development Pipeline

- Code maintained on public repository
- All commits required to be signed (DCO)
- Pull requests require one approving reviewer
- Stale PRs closed after 60 days of inactivity
- [Contributing Guidelines](https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING_GUIDELINES.md)

### Automated Security Checks

| Tool | Purpose |
| -- | -- |
| Harden-Runner | Runtime security |
| Dependabot | Dependency updates and vulnerability scanning |
| FOSSA | License compliance |
| OpenSSF Scorecard | Security practices scoring |
| CodeQL | Static analysis |

### Communication Channels

| Type | Channel |
| -- | -- |
| Internal | Monthly maintainer meetings, #jaeger-maintainers Slack |
| Inbound | [jaeger-tracing@googlegroups.com](mailto:jaeger-tracing@googlegroups.com), GitHub Issues, #jaeger Slack |
| Outbound | [jaegertracing.io](https://www.jaegertracing.io/), #jaeger Slack |

## Security Issue Resolution

### Responsible Disclosure

Refer to [SECURITY.md](https://github.com/jaegertracing/jaeger/blob/main/SECURITY.md) and [Report Security Issue](https://www.jaegertracing.io/report-security-issue/).

### Incident Response

Security Advisories are listed on the [GitHub Security tab](https://github.com/jaegertracing/jaeger/security/advisories).

## Appendix

### OpenSSF Best Practices

The Jaeger project has achieved passing level criteria: [bestpractices.dev/projects/1273](https://www.bestpractices.dev/en/projects/1273)

### Case Studies

- **Ticketmaster**: 300+ microservices, 100M+ transactions/day, uses adaptive sampling
- **Grafana Labs**: End-to-end request tracing, contextual logging integration

### Related Projects

**Zipkin** is an earlier open source distributed tracing system. While Zipkin has been around longer, Jaeger is known for scalability and CNCF backing. Jaeger has backward compatibility with Zipkin.

---

**Contributing Authors**: Cristian Panaro, Jia Lin Weng, Sameer Gori, Sarah Moughal  
**Contributing Maintainers**: Yuri Shkuro, Jonah Kowall  
**Contributing Reviewers**: Ragashree Shekar, Eddie Knight
