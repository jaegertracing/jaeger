# Lazy Storage Factory Initialization

## Status

Implemented -- https://github.com/jaegertracing/jaeger/pull/7887

## Context

The `jaegerstorage` extension (`cmd/jaeger/internal/extension/jaegerstorage/extension.go`) is responsible for managing storage backends in Jaeger. Its configuration allows declaring arbitrary numbers of storage backends under `trace_backends` and `metric_backends`. However, not all configured storages are necessarily used: consumers request specific storages by name via `TraceStorageFactory(name)`.

### Current Behavior

Currently, the extension initializes **all** configured storage factories during `Start()`:

```go
func (s *storageExt) Start(ctx context.Context, host component.Host) error {
    // ...
    for storageName, cfg := range s.config.TraceBackends {
        factory, err := storageconfig.CreateTraceStorageFactory(...)  // Connects immediately
        s.factories[storageName] = factory
    }
    // ...
}
```

Each factory's `NewFactory()` function performs actual initialization:

| Backend       | Initialization Actions                                           |
|---------------|------------------------------------------------------------------|
| Cassandra     | Creates session, connects to cluster, optionally creates schema  |
| Elasticsearch | Creates HTTP client, establishes connection pool                 |
| ClickHouse    | Opens connection, pings server, optionally creates schema        |
| gRPC          | Establishes gRPC connections (reader and writer)                 |
| Badger        | Opens database files, starts background maintenance goroutines   |
| Memory        | Allocates in-memory store                                        |

### Problems

1. **Wasted Resources**: Storage backends that are configured but never used still consume connections, memory, and background goroutines.

2. **Startup Failures for Unused Backends**: If a configured backend is unavailable (e.g., Cassandra cluster down), the entire extension fails to start, even if that storage isn't actually needed by any pipeline component.

3. **Configuration Use Cases**: Users may want to define multiple storage backends in a shared configuration file and selectively enable them in different deployment scenarios without modifying the configuration.

### Real-World Scenario

Consider a configuration with:
```yaml
extensions:
  jaeger_storage:
    trace_backends:
      primary_es:
        elasticsearch: { ... }
      archive_cassandra:
        cassandra: { ... }
      debug_memory:
        memory: { max_traces: 10000 }

  jaeger_query:
    storage:
      traces: primary_es
      traces_archive: debug_memory
```

With current behavior, Jaeger attempts to connect to Cassandra at startup, even though `archive_cassandra` isn't used. If Cassandra is unavailable, Jaeger fails to start despite the primary storage (Elasticsearch) being fully operational.

## Decision

This ADR evaluates two approaches to implement lazy initialization.

---

## Option 1: Two-Phase Factory Framework (Configure + Initialize)

### Design

Refactor the factory framework to separate configuration validation from backend initialization:

```go
// New interface additions to tracestore.Factory
type ConfigurableFactory interface {
    Factory
    // Configure validates configuration without establishing connections.
    // Called during extension Start() for all configured backends.
    Configure(ctx context.Context) error
}

type InitializableFactory interface {
    Factory
    // Initialize establishes connections and allocates resources.
    // Called lazily when TraceStorageFactory() is invoked.
    Initialize(ctx context.Context) error
    // IsInitialized returns true if Initialize() has been called.
    IsInitialized() bool
}
```

### Extension Changes

```go
type storageExt struct {
    config           *Config
    telset           component.TelemetrySettings
    factories        map[string]tracestore.Factory
    initialized      map[string]bool
    initMu           sync.Mutex  // Serializes initialization
    // ...
}

func (s *storageExt) Start(ctx context.Context, host component.Host) error {
    for storageName, cfg := range s.config.TraceBackends {
        // Phase 1: Configuration only - validate without connecting
        factory, err := storageconfig.CreateUninitializedFactory(ctx, storageName, cfg, telset)
        if err != nil {
            return fmt.Errorf("invalid configuration for storage '%s': %w", storageName, err)
        }
        if configurable, ok := factory.(ConfigurableFactory); ok {
            if err := configurable.Configure(ctx); err != nil {
                return fmt.Errorf("configuration validation failed for storage '%s': %w", storageName, err)
            }
        }
        s.factories[storageName] = factory
    }
    return nil
}

func (s *storageExt) TraceStorageFactory(name string) (tracestore.Factory, bool) {
    s.initMu.Lock()
    defer s.initMu.Unlock()

    f, ok := s.factories[name]
    if !ok {
        return nil, false
    }

    // Phase 2: Lazy initialization on first access
    if !s.initialized[name] {
        if initializable, ok := f.(InitializableFactory); ok {
            if err := initializable.Initialize(context.Background()); err != nil {
                s.telset.Logger.Error("Failed to initialize storage",
                    zap.String("name", name), zap.Error(err))
                return nil, false
            }
        }
        s.initialized[name] = true
    }
    return f, true
}
```

### Factory Implementation Changes

Each factory needs refactoring. Example for Cassandra:

```go
type Factory struct {
    config     cassandra.Options
    telset     telemetry.Settings
    session    cassandra.Session  // nil until initialized
    configured bool
    initialized bool
}

func NewFactory(opts cassandra.Options, telset telemetry.Settings) (*Factory, error) {
    return &Factory{
        config: opts,
        telset: telset,
    }, nil
}

func (f *Factory) Configure(ctx context.Context) error {
    // Validate configuration without connecting
    if err := f.config.Configuration.Validate(); err != nil {
        return err
    }
    f.configured = true
    return nil
}

func (f *Factory) Initialize(ctx context.Context) error {
    if f.initialized {
        return nil
    }
    // Establish actual connection
    session, err := cassandra.NewSession(&f.config.Configuration)
    if err != nil {
        return err
    }
    f.session = session
    f.initialized = true
    return nil
}

func (f *Factory) IsInitialized() bool {
    return f.initialized
}
```

### Pros

1. **Early Configuration Validation**: Invalid configurations are caught at startup, even for unused storages. This prevents runtime surprises.

2. **Clear Separation of Concerns**: Configuration validation and resource initialization are distinct phases with clear semantics.

3. **Predictable Startup Behavior**: All configuration errors surface during `Start()`, making debugging easier.

4. **Consistent Interface**: All factories follow the same lifecycle pattern.

### Cons

1. **Significant Refactoring**: All 6+ factory implementations require changes:
   - `internal/storage/v2/cassandra/factory.go`
   - `internal/storage/v2/elasticsearch/factory.go`
   - `internal/storage/v2/clickhouse/factory.go`
   - `internal/storage/v2/grpc/factory.go`
   - `internal/storage/v2/badger/factory.go`
   - `internal/storage/v2/memory/factory.go`
   - `cmd/internal/storageconfig/factory.go`

2. **API Breaking Change**: The `tracestore.Factory` interface changes, potentially affecting external consumers.

3. **Complex State Management**: Factories must track configuration vs. initialization state.

4. **Testing Complexity**: Tests need to account for the two-phase lifecycle.

### Implementation Effort

| Component | Effort |
|-----------|--------|
| Interface definitions | Low |
| Extension refactoring | Medium |
| Cassandra factory | Medium |
| Elasticsearch factory | Medium |
| ClickHouse factory | Medium |
| gRPC factory | Medium |
| Badger factory | Medium |
| Memory factory | Low |
| storageconfig helper | Medium |
| Test updates | High |
| **Total** | **High** |

---

## Option 2: Simple Lazy Initialization (Defer Everything)

### Design

Move all factory creation to `TraceStorageFactory()` without modifying the factory interfaces:

```go
type storageExt struct {
    config           *Config
    telset           component.TelemetrySettings
    host             component.Host
    factories        map[string]tracestore.Factory
    factoryMu        sync.Mutex
    // ...
}

func (s *storageExt) Start(ctx context.Context, host component.Host) error {
    s.host = host  // Store for later use
    s.factories = make(map[string]tracestore.Factory)
    // No factory initialization - just validation that config keys exist
    return nil
}

// Changed signature: (Factory, bool) -> (Factory, error)
// This allows callers to distinguish "not configured" from "initialization failed"
func (s *storageExt) TraceStorageFactory(name string) (tracestore.Factory, error) {
    s.factoryMu.Lock()
    defer s.factoryMu.Unlock()

    // Return cached factory if already created
    if f, ok := s.factories[name]; ok {
        return f, nil
    }

    // Check if configuration exists
    cfg, ok := s.config.TraceBackends[name]
    if !ok {
        return nil, fmt.Errorf(
            "storage '%s' not declared in '%s' extension configuration",
            name, componentType,
        )
    }

    // Create factory on demand
    telset := telemetry.FromOtelComponent(s.telset, s.host)
    factory, err := storageconfig.CreateTraceStorageFactory(
        context.Background(),
        name,
        cfg,
        telset,
        func(authCfg config.Authentication, backendType, backendName string) (extensionauth.HTTPClient, error) {
            return s.resolveAuthenticator(s.host, authCfg, backendType, backendName)
        },
    )
    if err != nil {
        return nil, fmt.Errorf("failed to initialize storage '%s': %w", name, err)
    }

    s.factories[name] = factory
    return factory, nil
}
```

### Pros

1. **Minimal Code Changes**: Only `extension.go` and its callers need modification.

2. **No Factory Interface Changes**: Existing factory implementations (`tracestore.Factory`) remain unchanged.

3. **No External API Breaking Changes**: The extension is internal; external consumers are unaffected.

4. **Simple Mental Model**: Factories are created when needed, cached for reuse.

5. **Quick Implementation**: Can be completed in a single PR.

6. **Clear Error Messages**: Changing signature from `(Factory, bool)` to `(Factory, error)` allows callers to distinguish "storage not configured" from "initialization failed" and provides actionable error messages.

### Cons

1. **Deferred Configuration Errors**: Invalid configurations for unused storages are never detected. A typo in an unused backend's config silently passes.

2. **Runtime Initialization Failures**: Connection failures happen when a pipeline component first requests the storage, not at startup. This could cause unexpected failures during operation.

3. **Less Predictable Startup**: Startup succeeds even with broken configurations, potentially masking issues.

4. **Interface Signature Change**: The `Extension` interface methods change from `(Factory, bool)` to `(Factory, error)`. While not a breaking change for external consumers (the extension is internal), it requires updating all callers within the codebase.

### Potential Mitigation

Add optional configuration validation at startup:

```go
func (s *storageExt) Start(ctx context.Context, host component.Host) error {
    s.host = host
    // Optional: validate configurations without initializing
    for name, cfg := range s.config.TraceBackends {
        if err := cfg.Validate(); err != nil {
            return fmt.Errorf("invalid configuration for storage '%s': %w", name, err)
        }
    }
    return nil
}
```

This requires adding `Validate()` methods to backend configs, which is simpler than full two-phase factories.

### Implementation Effort

| Component | Effort |
|-----------|--------|
| Extension interface change | Low |
| Extension lazy init logic | Low |
| Update callers (GetTraceStoreFactory, etc.) | Low |
| Config validation (optional) | Low-Medium |
| Test updates | Low |
| **Total** | **Low** |

---

## Comparison Summary

| Criterion | Option 1: Two-Phase | Option 2: Simple Lazy |
|-----------|--------------------|-----------------------|
| Implementation effort | High | Low |
| Factory interface changes | Yes (breaking) | No |
| Extension interface changes | No | Yes (minor) |
| Early config validation | Yes | Partial (with mitigation) |
| Runtime failure risk | Low | Medium |
| Code complexity | Higher | Lower |
| Factory changes required | All backends | None |
| Error message clarity | Good | Good (with error return) |
| Time to implement | Weeks | Days |

## Recommendation

**Option 2 (Simple Lazy Initialization) with Config Validation Mitigation** is recommended as the initial implementation because:

1. It solves the primary problem (wasted resources, startup failures for unused backends) with minimal risk.
2. It can be implemented quickly without breaking changes.
3. Adding `Validate()` methods to configs provides early error detection without the complexity of two-phase factories.
4. Option 1 can be pursued later if stronger guarantees are needed.

### Suggested Implementation Steps

1. Change `Extension` interface signatures from `(Factory, bool)` to `(Factory, error)` for `TraceStorageFactory()` and `MetricStorageFactory()`.
2. Refactor `extension.go` to defer factory creation to `TraceStorageFactory()`/`MetricStorageFactory()`.
3. Update all callers (`GetTraceStoreFactory`, `GetMetricStorageFactory`, `GetSamplingStoreFactory`, `GetPurger`) to handle the new error return.
4. Add `Validate()` methods to `TraceBackend` and `MetricBackend` config structs.
5. Call validation in `Start()` to catch configuration errors early.
6. Update tests to verify lazy initialization behavior.
7. Document the behavior change in release notes.

## Consequences

### Positive

- Unused storage backends no longer consume resources.
- Startup succeeds even when unused backends are unavailable.
- Minimal code changes reduce risk of regressions.

### Negative

- Configuration errors for unused storages may go unnoticed (mitigated by validation).
- First access to a storage may fail if the backend becomes unavailable after startup.
- Requires updating all callers to handle the new `error` return type.

### Neutral

- Logging will shift from startup to first-access for storage initialization messages.
- Shutdown logic must handle partially-initialized factory maps.

---

## References

- Extension implementation: `cmd/jaeger/internal/extension/jaegerstorage/extension.go`
- Factory creation: `cmd/internal/storageconfig/factory.go`
- Factory interface: `internal/storage/v2/api/tracestore/factory.go`
- Backend implementations:
  - `internal/storage/v2/cassandra/factory.go`
  - `internal/storage/v2/elasticsearch/factory.go`
  - `internal/storage/v2/clickhouse/factory.go`
  - `internal/storage/v2/grpc/factory.go`
  - `internal/storage/v2/badger/factory.go`
  - `internal/storage/v2/memory/factory.go`
