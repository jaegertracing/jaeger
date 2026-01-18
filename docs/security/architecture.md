# Jaeger Security Architecture

This document outlines the security architecture of Jaeger, focusing on cryptographic practices, input validation, and system hardening.

## TLS and Cryptographic Practices

Jaeger supports TLS for all its network communications, including span ingestion, internal component communication, and access to the Query API and UI.

### TLS Configuration

TLS can be configured for both clients and servers across all Jaeger components (Collector, Query, Ingester, Agent).

- **Supported TLS Versions**: Jaeger can be configured to use TLS 1.2 and 1.3, and these are the only versions that should be used in production. Users should configure the minimum supported version as TLS 1.2 or higher using the `--tls.min-version` flag (or corresponding YAML configuration), with TLS 1.3 recommended where available. While TLS 1.0 and 1.1 may still be technically supported for legacy interoperability, they are deprecated, have known security weaknesses, and **must not be enabled in production environments**.
- **Cipher Suites**: A custom list of allowed cipher suites can be configured to ensure only strong cryptographic algorithms are used.
- **Certificate Management**:
    - **CA Certificate**: Can be provided to verify the server's or client's certificate.
    - **Server Certificate and Key**: Required for enabling TLS on servers.
    - **Client Authentication (mTLS)**: Jaeger supports mutual TLS, requiring clients to provide a valid certificate for authentication.
- **Reloading Certificates**: Jaeger supports hot-reloading of TLS certificates and keys from the filesystem without restarting the service, controlled by a configurable reload interval.

### Secure Defaults

- **Certificate Verification**: When TLS is enabled, certificate verification is performed by default.
- **Insecure Communication**: Users must explicitly set `insecure: true` or `insecure_skip_verify: true` to bypass security controls, which is strongly discouraged for production environments.

## Input Validation

Jaeger performs strict input validation to prevent injection attacks and ensure system stability.

- **OTLP and Protobuf**: Jaeger primarily uses structured data formats like OTLP (via gRPC and HTTP) and Protobuf for internal communication. These formats provide inherent protection against many common injection vulnerabilities.
- **Schema Validation**: Inbound spans are validated against the defined schemas.
- **Size Limits**:
    - **gRPC Message Size**: Limits are enforced on the maximum size of incoming gRPC messages.
    - **HTTP Request Size**: Limits are enforced for HTTP-based ingestion.
- **Storage Queries**: Queries to storage backends (Elasticsearch, Cassandra, etc.) are constructed using parameterized queries or dedicated client libraries that prevent injection.

## System Hardening

Jaeger is designed to be deployed in a hardened manner.

### Container Security

- **Minimal Base Images**: Jaeger's official Docker images are built using minimal base images like `alpine` or `scratch` to reduce the attack surface.
- **Non-Root User**: Containers are designed to run as a non-privileged user where possible.

### Dependency Management

- **Vulnerability Scanning**: The project uses Dependabot for automated dependency monitoring and daily vulnerability scans.
- **Software Bill of Materials (SBOM)**: An SBOM is generated for each release to provide transparency into the included components.

### Secure Build Pipeline

- **Signed Commits**: All contributions are required to follow the Developer Certificate of Origin (DCO) and should be signed.
- **GitHub Actions Security**: The build pipeline uses security features like `harden-runner` to monitor and restrict network access during the build process.
- **Release Signing**: All release artifacts and Git tags are GPG-signed by the maintainers.

## Credential Management

- **No Hardcoded Credentials**: Jaeger does not contain any hardcoded credentials. All secrets (passwords, tokens, etc.) must be provided via environment variables, command-line flags, or configuration files.
- **Environment Variables**: Recommended for providing sensitive information in containerized environments.
- **Secure Storage**: Users are encouraged to use secure secret management systems (like Kubernetes Secrets or HashiCorp Vault) to manage Jaeger's credentials.

## References

- [Securing Jaeger Installation](https://www.jaegertracing.io/docs/latest/security/)
- [OpenTelemetry TLS Configuration](https://github.com/open-telemetry/opentelemetry-collector/blob/main/config/configtls/README.md)
