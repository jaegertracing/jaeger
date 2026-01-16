# Jaeger Security Architecture

This document describes Jaeger's cryptographic and network security practices, satisfying OpenSSF Best Practices Silver badge requirements.

## Table of Contents

- [TLS Configuration](#tls-configuration)
- [Cryptographic Practices](#cryptographic-practices)
- [Input Validation](#input-validation)
- [Hardening](#hardening)
- [Credential Management](#credential-management)

## TLS Configuration

### Protocol Support

Jaeger supports TLS for all network communications:

| Connection | TLS Support | mTLS Support |
| -- | -- | -- |
| OTel SDK → Collector | ✅ | ✅ |
| Collector → Storage | ✅ | ✅ |
| Query → Storage | ✅ | ✅ |
| UI → Query | ✅ | ✅ |
| Admin endpoints | ✅ | ✅ |

### TLS Version

- **Minimum**: TLS 1.2 (configurable via `tls.min_version`)
- **Recommended**: TLS 1.3
- **Maximum**: TLS 1.3 (configurable via `tls.max_version`)

Older protocols (SSL, TLS 1.0, TLS 1.1) are not supported by default.

### Certificate Verification

| Setting | Default | Description |
| -- | -- | -- |
| Certificate verification | Enabled | Validates server certificates by default |
| `insecure_skip_verify` | `false` | Must be explicitly set to skip verification |
| System CA pool | Used | When no CA file specified, uses system certificates |
| Client certificates | Optional | Supported for mTLS |

### Configuration Example

```yaml
# Secure TLS configuration
extensions:
  jaeger_storage:
    backends:
      my_storage:
        cassandra:
          connection:
            servers: ["cassandra:9042"]
            tls:
              insecure: false
              ca_file: /path/to/ca.crt
              cert_file: /path/to/client.crt
              key_file: /path/to/client.key
              min_version: "1.2"
              server_name: cassandra.example.com
```

## Cryptographic Practices

### Algorithm Support

Jaeger uses Go's standard `crypto/tls` library, which provides:

| Category | Algorithms |
| -- | -- |
| Key Exchange | ECDHE (preferred), RSA |
| Cipher Suites | AES-GCM (preferred), ChaCha20-Poly1305 |
| Hash Functions | SHA-256, SHA-384, SHA-512 |
| Signature | ECDSA, RSA-PSS, Ed25519 |

### Cipher Suite Configuration

Custom cipher suites can be configured:

```yaml
tls:
  cipher_suites:
    - TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
    - TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
```

### Known Weak Algorithms

The following are **NOT** used in default configurations:
- SHA-1 for signatures
- MD5
- RC4
- DES/3DES
- CBC mode (where avoidable)

## Input Validation

### Span/Trace Validation

The Jaeger Collector validates incoming data:

| Validation | Description |
| -- | -- |
| Schema validation | OTLP/protobuf schema enforced |
| Size limits | Configurable maximum span size |
| Field validation | Required fields checked |
| Timestamp validation | Reasonable time bounds |

### Rate Limiting

Collectors support rate limiting to prevent DoS:

```yaml
processors:
  batch:
    send_batch_size: 10000
    timeout: 10s
```

### Sanitization

- Service names sanitized for storage
- Tag values validated for type correctness
- Binary data length-limited

## Hardening

### Container Images

| Image Type | Base | Security Properties |
| -- | -- | -- |
| Release | Alpine Linux | Minimal packages, regular updates |
| Scratch | `scratch` | No shell, no package manager |
| Debug | Alpine | Includes debugging tools |

All images:
- Run as non-root user
- Have read-only root filesystem (when possible)
- Use pinned base image digests

### Build Security

| Practice | Implementation |
| -- | -- |
| Reproducible builds | Pinned dependencies via `go.sum` |
| Binary hardening | Go's built-in stack protection |
| CI security | Harden-Runner action |
| Dependency scanning | Dependabot daily scans |

### Go Security Features

Go provides inherent security features:
- Memory safety (no buffer overflows)
- Bounds checking on arrays
- No pointer arithmetic
- Race detector in CI tests

## Credential Management

### Storage Separation

Credentials are stored separately from configuration:

| Credential Type | Storage Method |
| -- | -- |
| TLS certificates | File paths in config |
| TLS private keys | File paths in config |
| Storage passwords | Environment variables or files |
| API tokens | Environment variables |

### Configuration Example

```yaml
connection:
  auth:
    basic:
      username: "${env:CASSANDRA_USERNAME}"
      password: "${env:CASSANDRA_PASSWORD}"
  tls:
    cert_file: /secrets/tls/client.crt
    key_file: /secrets/tls/client.key
```

### Hot Reload

TLS certificates can be reloaded without restart:

```yaml
tls:
  reload_interval: 24h
```

### No Credentials in Traces

Jaeger does not capture or store:
- Authentication headers from traced requests
- Credential information in spans
- Private keys or certificates

## Verification

To verify TLS is properly configured:

```bash
# Check TLS version and cipher
openssl s_client -connect jaeger-collector:4317 -tls1_2

# Verify certificate
openssl s_client -connect jaeger-collector:4317 -verify 5
```

## References

- [Securing Jaeger Installation](https://www.jaegertracing.io/docs/latest/security/)
- [OTEL Collector TLS Configuration](https://opentelemetry.io/docs/collector/configuration/#tls-configuration)
- [Go crypto/tls documentation](https://pkg.go.dev/crypto/tls)
