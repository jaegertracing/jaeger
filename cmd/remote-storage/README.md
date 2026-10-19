# Jaeger Remote Storage

The `jaeger-remote-storage` binary allows sharing single-node storage implementations like memory or Badger over gRPC. It implements the Jaeger Remote Storage gRPC API, enabling Jaeger components to use these storage backends remotely.

## Configuration

### YAML Configuration

Configure remote-storage using a YAML configuration file with the `--config-file` flag:

```bash
./jaeger-remote-storage --config-file config.yaml
```

The file is the only place the service is configured: `--config-file` and `--feature-gates` are its only flags. It is read the same way the `jaeger` binary reads its configuration, so a value can refer to an environment variable as `${env:VAR}`, `${VAR}` or `${env:VAR:-default}`, and a key the configuration does not define is rejected. Every section is optional and falls back to the defaults shown below.

#### Configuration File Structure

```yaml
# Admin server: health check on /, metrics, /debug/pprof (default :17270)
admin:
  endpoint: :17270
  # tls:
  #   cert_file: /path/to/cert.pem
  #   key_file: /path/to/key.pem

# Logging (defaults shown)
logging:
  level: info        # debug, info, warn, error
  encoding: json     # json or console

# Metrics (defaults shown)
metrics:
  backend: prometheus  # prometheus or none
  http_route: /metrics

# Server configuration
grpc:
  endpoint: :17271  # gRPC endpoint for remote storage API

# Storage configuration
storage:
  backends:
    default-storage:
      memory:
        max_traces: 100000

# Multi-tenancy configuration (optional)
multi_tenancy:
  enabled: false
```

#### Storage Backends

The storage configuration follows the same format as the `jaeger_storage` extension in Jaeger v2. All official backends are supported.

##### Memory Storage
```yaml
storage:
  backends:
    memory-storage:
      memory:
        max_traces: 100000
```

##### Badger Storage
```yaml
storage:
  backends:
    badger-storage:
      badger:
        directories:
          keys: /tmp/jaeger/badger/keys
          values: /tmp/jaeger/badger/values
        ephemeral: false
        ttl:
          spans: 168h  # 7 days
```

##### gRPC Storage
```yaml
storage:
  backends:
    grpc-storage:
      grpc:
        endpoint: remote-server:17271
        tls:
          insecure: true
```

See example configuration files:
- `config.yaml` - Memory storage example
- `config-badger.yaml` - Badger storage example

## Usage

### Start with Memory Backend

```bash
./jaeger-remote-storage --config-file config.yaml
```

### Start with Badger Backend

```bash
./jaeger-remote-storage --config-file config-badger.yaml
```

### Multi-tenancy

To enable multi-tenancy:

```yaml
grpc:
  endpoint: :17271

multi_tenancy:
  enabled: true
  header: x-tenant
  tenants:
    - tenant1
    - tenant2

storage:
  backends:
    default-storage:
      memory:
        max_traces: 100000
```

Enabling `multi_tenancy` does not authenticate callers. See [Trust boundary](#trust-boundary).

## Trust boundary

The gRPC port is a trusted interface. `jaeger-remote-storage` and the [`remote_storage` extension](../jaeger/internal/extension/remotestorage/README.md#trust-boundary) serve the same API. The default endpoint is `:17271`, which listens on every interface. Unless `grpc.auth` names an authenticator, a caller who can reach the port can read and write the storage backend behind it.

The tenant header is not authentication. When `multi_tenancy` is enabled, the header (default `x-tenant`) identifies the tenant for the call. It does not authenticate the caller. A client that can reach the port and present a tenant name the server accepts is served, including a name listed under `tenants`.

Deployments must protect this port with mTLS and network policy, so only the Jaeger processes that use the backend can open it. Server TLS is the `grpc.tls` block. Setting `client_ca_file` makes the server require and verify a client certificate.

grpc-go's receive limit applies to every request before decode. It is 4 MB by default, and it is configurable on this server with `grpc.max_recv_msg_size_mib` (unset means the grpc-go default). protobuf-go's unmarshal recursion limit bounds how deeply a message may be nested. These are bounds on the payload. They are not a substitute for mTLS or network policy.

## Integration with Jaeger

To use remote-storage with Jaeger components, configure them to use the gRPC storage backend:

```yaml
extensions:
  jaeger_storage:
    backends:
      some-storage:
        grpc:
          endpoint: localhost:17271
          tls:
            insecure: true
```

For more details, see the [gRPC storage documentation](../../internal/storage/v2/grpc/README.md).
