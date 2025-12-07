# Jaeger Remote Storage

The `jaeger-remote-storage` binary allows sharing single-node storage implementations like memory or Badger over gRPC. It implements the Jaeger Remote Storage gRPC API, enabling Jaeger components to use these storage backends remotely.

## Configuration

### YAML Configuration

Configure remote-storage using a YAML configuration file with the `--config-file` flag:

```bash
./jaeger-remote-storage --config-file config.yaml
```

#### Configuration File Structure

```yaml
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
  host-port: :17271

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
