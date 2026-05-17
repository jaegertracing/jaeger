# Jaeger Security Review 2026

This document records a public security review of Jaeger for the OpenSSF
Best Practices Gold `security_review` criterion. The criterion requires a
security review within the last five years that considers the project's security
requirements and security boundary.

This review is a project security review, not a third-party penetration test.
Historical third-party audit evidence remains available in the
[jaegertracing/security-audits](https://github.com/jaegertracing/security-audits)
repository.

## Review Metadata

| Field | Value |
| --- | --- |
| Review date | 2026-05-17 |
| Reviewed branch | `main` |
| Review type | Public project security review |
| Reviewers | Jaeger maintainers (led by [@jkowall](https://github.com/jkowall)), with public review by the [jaegertracing/jaeger-maintainers](https://github.com/orgs/jaegertracing/teams/jaeger-maintainers) team on the tracking pull request. |
| Tracking issue | <https://github.com/jaegertracing/jaeger/issues/8485> |
| Parent tracker | <https://github.com/jaegertracing/jaeger/issues/8481> |
| Sensitive findings | Not disclosed in this document; report and handle them through [SECURITY.md](../../SECURITY.md). |

## Inputs Reviewed

- [Security Policy](../../SECURITY.md), including vulnerability reporting,
  dependency policy, and scanner-report guidance.
- [Threat Model](threat-model.md), including trust boundaries and primary
  threat scenarios.
- [Security Architecture](architecture.md), including TLS, certificate
  verification, input validation, hardening, and credential management.
- [Security Assurance Case](assurance-case.md), including security goals,
  design principles, weakness mitigations, and controls.
- [CNCF TAG Security Self-Assessment](self-assessment.md), including project
  scope, security goals, and self-assessed weaknesses.
- [Release Verification](verifying-releases.md), including artifact and
  signature verification guidance.
- Current OpenSSF Best Practices Gold evidence in
  [OpenSSF Best Practices Gold Evidence](openssf-gold-evidence.md).
- Security-relevant CI workflows, including
  [`codeql.yml`](../../.github/workflows/codeql.yml),
  [`dependency-review.yml`](../../.github/workflows/dependency-review.yml),
  [`scorecard.yml`](../../.github/workflows/scorecard.yml), and
  [`fossa.yml`](../../.github/workflows/fossa.yml).

## Security Requirements Considered

| Requirement | Review notes |
| --- | --- |
| Trace data confidentiality | Jaeger may process sensitive trace data. Operators are expected to enable TLS or mTLS on external and internal communication paths, configure access control for Query/UI, and avoid tracing secrets or personally identifiable information. |
| Trace data integrity | TLS/mTLS, certificate verification, structured OTLP/protobuf inputs, and authenticated storage connections protect trace data in transit and reduce tampering risk across component boundaries. |
| Availability | The threat model identifies span flooding and storage pressure as availability risks. Sampling, rate limiting, buffering, storage isolation, and deployment-level quotas remain the primary mitigations. |
| Access control | Query/UI access depends on operator-configured authentication mechanisms such as bearer tokens, OAuth2, and Kerberos, with authorization enforced at the deployment layer (for example, RBAC in Kubernetes or reverse-proxy policy). Storage credentials should be scoped to the minimum privileges needed by each component. |
| Secure transport and cryptography | Jaeger supports TLS 1.2 and TLS 1.3. Certificate verification is enabled when TLS is configured, and insecure modes require explicit operator configuration. |
| Input validation | Jaeger uses structured protocols such as OTLP, gRPC, HTTP, and protobuf. Message-size limits, schema-aware parsing, and storage-client query construction reduce injection and resource-exhaustion risk. |
| Credential handling | Credentials, TLS keys, certificates, tokens, and storage passwords are supplied through configuration, environment variables, or external secret-management systems rather than hardcoded in the binary. |
| Vulnerability handling | Vulnerabilities are reported privately through GitHub Security Advisories or encrypted public channels. Continuous code scanning is performed by [`codeql.yml`](../../.github/workflows/codeql.yml), and public scanner reports are expected to be analyzed for applicability before disclosure. |
| Supply-chain and release integrity | The project uses code review, DCO, CI, dependency scanning ([`dependency-review.yml`](../../.github/workflows/dependency-review.yml), [`scorecard.yml`](../../.github/workflows/scorecard.yml), and [`fossa.yml`](../../.github/workflows/fossa.yml)), hardened GitHub Actions workflows, signed releases, SBOM generation, and documented release-verification procedures. |

## Security Boundary Considered

The reviewed security boundary includes Jaeger-owned binaries, services,
configuration handling, public APIs, storage clients, official release
artifacts, and official container images.

The boundary does not include instrumented applications, OpenTelemetry SDKs,
external identity providers, browser environments, Kubernetes clusters,
network infrastructure, or the internal implementation of external storage
systems. Jaeger still has security obligations where it accepts data from or
connects to those systems.

| Boundary | Description | Primary controls reviewed |
| --- | --- | --- |
| Instrumented application to Collector | Applications and SDKs submit untrusted telemetry to Jaeger. | TLS/mTLS, structured telemetry formats, schema validation, size limits, sampling, and rate limiting. |
| Collector or Ingester to storage | Jaeger writes trace data to configured storage backends. | TLS, storage authentication, scoped credentials, and backend-specific client libraries. |
| Storage to Query service | Query reads trace data from storage and exposes it through APIs and UI. | TLS, authenticated storage access, read-oriented credentials where supported, and parameterized or client-mediated queries. |
| Query/UI to users | Users access trace data through Jaeger APIs and browser UI. | Operator-configured authentication, authorization, RBAC-capable deployments, TLS, and guidance to avoid storing secrets in traces. |
| Project source to release artifacts | Source changes become binaries, images, charts, and release metadata. | Code review, CI, dependency scanning, DCO, signed releases, SBOMs, and release-verification documentation. |

## Review Summary

- The documented security requirements cover confidentiality, integrity,
  availability, access control, secure transport, credential handling,
  vulnerability handling, supply-chain controls, and release integrity.
- The documented trust boundaries cover telemetry ingestion, internal
  component-to-storage communication, Query/UI access, and source-to-release
  delivery.
- Existing public documentation provides linkable evidence for the security
  boundary and requirements considered by this review.
- No sensitive vulnerability details are disclosed in this public summary. Any
  vulnerability discovered during future review work must be reported and
  handled through [SECURITY.md](../../SECURITY.md).

## Follow-Up

The OpenSSF Gold `security_review` evidence should point at this document.
Separate Gold workstream items remain tracked outside this review, including
technical evidence for hardened site headers, hardening, coverage, reproducible
builds, dynamic analysis, and newcomer-task availability.

This review should be refreshed within five years or sooner after a major
security-boundary change, such as a new externally reachable service, a new
storage security model, or a major release-process change.
