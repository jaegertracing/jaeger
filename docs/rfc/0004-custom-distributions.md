# RFC 0004: Custom Distributions of Jaeger

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-06-17
- **Last Updated:** 2026-06-17

---

## Abstract

This RFC proposes a mechanism for users to build custom distributions of Jaeger that include additional OpenTelemetry Collector components (from otel-contrib or third-party sources) alongside Jaeger's own components. The goal is to achieve this without fragmenting Jaeger into a multi-module repository and without reintroducing the uncontrolled external dependency problem that the current `internal` packaging was designed to prevent.

---

## 1. Motivation

Jaeger v2 is built as an OpenTelemetry Collector distribution with custom components. All Jaeger-specific components (extensions, exporters, processors) live under `cmd/jaeger/internal/`, making them unimportable by external Go code. This was an intentional decision during the v1→v2 migration to:

1. Prevent external projects (including OTEL Collector itself) from depending on Jaeger internals, which had previously blocked refactoring.
2. Allow rapid iteration on component APIs without concern for backwards compatibility.

Now that the migration is complete, users have a legitimate need to assemble custom Jaeger distributions — for example, adding a custom exporter for a proprietary backend, including an otel-contrib receiver not in the default set, or embedding an in-house processor. Today this is impossible without forking the repository.

The standard mechanism for building custom OTel Collector distributions is [ocb](https://github.com/open-telemetry/opentelemetry-collector/tree/main/cmd/builder) (OpenTelemetry Collector Builder). In its typical usage, each component is a standalone Go module with its own `go.mod`. However, `ocb` also supports referencing multiple packages within a single module via separate `gomod` (the module to `require`) and `import` (the Go import path) fields. This means Jaeger does not need to split into multiple modules — it only needs to make component packages importable from a public path within the existing module.

---

## 2. Design Constraints

| Constraint | Rationale |
|---|---|
| Single `go.mod` | Avoid multi-module maintenance overhead and CI complexity |
| No uncontrolled external imports of Jaeger internals | Preserve freedom to refactor internal APIs |
| Compatible with `ocb` workflow | Users should not need custom tooling to build a Jaeger distribution |
| Minimal API surface | Only expose what is necessary for distribution assembly |
| No code duplication | Thin public packages must delegate to internal implementations |

---

## 3. Current Architecture

All custom Jaeger components reside in `cmd/jaeger/internal/`:

| Component Type | Package | Config Type |
|---|---|---|
| Extension | `cmd/jaeger/internal/extension/jaegerstorage` | `jaeger_storage` |
| Extension | `cmd/jaeger/internal/extension/jaegerquery` | `jaeger_query` |
| Extension | `cmd/jaeger/internal/extension/remotesampling` | `remote_sampling` |
| Extension | `cmd/jaeger/internal/extension/remotestorage` | `remote_storage` |
| Extension | `cmd/jaeger/internal/extension/jaegermcp` | `jaeger_mcp` |
| Extension | `cmd/jaeger/internal/extension/expvar` | `expvar` |
| Exporter | `cmd/jaeger/internal/exporters/storageexporter` | `jaeger_storage_exporter` |
| Processor | `cmd/jaeger/internal/processors/adaptivesampling` | `adaptive_sampling` |

These are assembled in `cmd/jaeger/internal/components.go` via factory functions:

```go
func (b builders) build() (otelcol.Factories, error) {
    // ... registers all component factories
}
```

The binary entry point (`cmd/jaeger/main.go`) calls `internal.Command()` which uses `otelcol.NewCommand()` with the assembled factories.

---

## 4. Proposed Approaches

### 4.1 Public Facade Packages (Recommended)

Create a public package tree that re-exports only the `NewFactory()` function from each internal component.

Go's `internal` visibility rule is enforced by directory tree: only packages rooted at the *parent* of the `internal/` directory can import from it. Since the internal components live under `cmd/jaeger/internal/`, the facade packages must also live under `cmd/jaeger/` to satisfy this constraint:

```
cmd/jaeger/components/
├── all.go                    # convenience: returns all Jaeger component factories
├── extension/
│   ├── jaegerstorage/
│   │   └── factory.go        # re-exports internal NewFactory()
│   ├── jaegerquery/
│   │   └── factory.go
│   ├── remotesampling/
│   │   └── factory.go
│   ├── remotestorage/
│   │   └── factory.go
│   ├── jaegermcp/
│   │   └── factory.go
│   └── expvar/
│       └── factory.go
├── exporter/
│   └── storageexporter/
│       └── factory.go
└── processor/
    └── adaptivesampling/
        └── factory.go
```

Each facade file is minimal:

```go
// cmd/jaeger/components/extension/jaegerstorage/factory.go
package jaegerstorage

import (
    "go.opentelemetry.io/collector/extension"
    impl "github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
)

func NewFactory() extension.Factory {
    return impl.NewFactory()
}
```

The convenience package aggregates all factories:

```go
// cmd/jaeger/components/all.go
package components

import (
    "go.opentelemetry.io/collector/connector"
    "go.opentelemetry.io/collector/exporter"
    "go.opentelemetry.io/collector/extension"
    "go.opentelemetry.io/collector/processor"
    "go.opentelemetry.io/collector/receiver"
)

type Factories struct {
    Extensions []extension.Factory
    Receivers  []receiver.Factory
    Exporters  []exporter.Factory
    Processors []processor.Factory
    Connectors []connector.Factory
}

func AllFactories() Factories { ... }
```

**Advantages:**
- Single `go.mod` — no structural changes to the repo.
- Public API is a single function per component (`NewFactory()`). Internal types, configs, and implementation details remain private.
- Compatible with `ocb` using the `gomod`/`import` field separation (see Section 5).
- Trivial maintenance: facade files are mechanical and can be code-generated.
- Clear contract: if a user imports `cmd/jaeger/components/...`, the only guarantee is that `NewFactory()` returns a valid factory. All config and behavior remains internal.

**Disadvantages:**
- Facade location is constrained by Go's `internal` visibility rules. Since `internal/` is under `cmd/jaeger/`, facades must also be under `cmd/jaeger/` — the import path `github.com/jaegertracing/jaeger/cmd/jaeger/components/...` is longer than a hypothetical top-level `pkg/components/...`. However, path length is largely irrelevant to users since it only appears in `builder.yaml`.
- External consumers who `go get` the Jaeger module could technically import the facades directly in their own Go code (not just via `ocb`). This is acceptable since the facade exposes only the `Factory` interface type, not internal structs.

### 4.2 Multi-Module Repository

Split each component into its own `go.mod`:

```
components/
├── extension/jaegerstorage/
│   ├── go.mod   # module github.com/jaegertracing/jaeger/components/extension/jaegerstorage
│   └── ...
├── exporter/storageexporter/
│   ├── go.mod
│   └── ...
```

**Advantages:**
- Fully compatible with `ocb` out-of-the-box (each component is a standalone module).
- Strongest isolation: each component declares its own dependencies.

**Disadvantages:**
- Significant maintenance overhead: coordinating versions across 8+ modules, managing cross-module replace directives during development, updating all go.mod files on shared dependency bumps.
- Requires CI tooling renovation (release automation, dependency graph management).
- Goes against the stated preference to avoid this approach.

**Verdict: Rejected** due to maintenance cost.

### 4.3 Code Generation with Custom Builder

Provide a Jaeger-specific builder tool (or a wrapper around `ocb`) that understands the monorepo layout and generates the correct `main.go` + `go.mod` with `replace` directives.

**Advantages:**
- Users get a familiar `ocb`-like experience.
- Can handle monorepo-specific concerns automatically.

**Disadvantages:**
- Introduces a custom tool to maintain.
- Diverges from the ecosystem standard — users familiar with `ocb` must learn a new tool.
- Still requires the facade packages (to provide importable paths).

**Verdict: Optional enhancement on top of 4.1**, not a standalone solution.

---

## 5. ocb Compatibility

The `ocb` builder manifest (`builder.yaml`) supports separate `gomod` and `import` fields per component entry. The `gomod` field specifies the Go module to add to `require` (module path + version), while the `import` field specifies the package path to import within that module. Since all Jaeger facade packages live in a single module, they share the same `gomod` value but have different `import` paths:

```yaml
dist:
  module: github.com/example/my-jaeger
  name: my-jaeger

extensions:
  - gomod: github.com/jaegertracing/jaeger v1.72.0
    import: github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/jaegerstorage
  - gomod: github.com/jaegertracing/jaeger v1.72.0
    import: github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/jaegerquery

receivers:
  - gomod: go.opentelemetry.io/collector/receiver/otlpreceiver v0.154.0
  # user adds their custom component:
  - gomod: github.com/example/my-custom-receiver v0.1.0

exporters:
  - gomod: github.com/jaegertracing/jaeger v1.72.0
    import: github.com/jaegertracing/jaeger/cmd/jaeger/components/exporter/storageexporter
```

This works with `ocb` today without any upstream changes or `replace` directives — the generated `go.mod` will have a single `require github.com/jaegertracing/jaeger v1.72.0` entry, and the generated `components.go` will import each package by its `import` path.

### Strategy B: Convenience `all` Package

For users who want *all* Jaeger components and just want to add extras, provide a single import that includes everything:

```yaml
extensions:
  - gomod: github.com/jaegertracing/jaeger v1.72.0
    import: github.com/jaegertracing/jaeger/cmd/jaeger/components/all/extensions
```

This would require a slightly different facade design where `all/extensions` exports a slice of factories rather than a single one. However, `ocb` currently expects one factory per entry, so this would require either:
- A wrapper script that expands the "all" package into individual factory calls, or
- Upstream `ocb` support for multi-factory packages.

**Recommendation:** Start with Strategy A (individual imports, shared gomod). It works with `ocb` today without upstream changes.

---

## 6. Versioning and Stability

The public facade packages expose only `NewFactory()` — a function returning an opaque interface type defined by the OTEL Collector SDK. This means:

- **The public API is exactly one function signature per component.** It cannot break unless the upstream OTEL Collector changes the `Factory` interface (which is governed by OTEL's own stability guarantees).
- **Config structs remain internal.** Users configure components via YAML, not Go code. No Config types need to be public.
- **Internal refactoring remains safe.** Moving code within `cmd/jaeger/internal/` does not affect the public API as long as `NewFactory()` continues to compile.

Jaeger can adopt Go module versioning (`v1.x.y`) for the public facades with the guarantee that `NewFactory()` signatures are stable for a given major version.

---

## 7. Implementation Plan

### Phase 1: Create Public Facade Packages

1. Create `cmd/jaeger/components/` directory tree with facade files.
2. Each file exports only `NewFactory()`, delegating to the internal implementation.
3. Create `cmd/jaeger/components/all.go` that returns all Jaeger factories grouped by type.
4. Add a test that verifies all facades compile and return non-nil factories.

### Phase 2: Documentation and Examples

1. Add a `docs/custom-distribution.md` guide with:
   - A sample `builder.yaml` for building a custom Jaeger distribution with `ocb`.
   - Instructions for adding third-party or custom components.
   - A working example that adds a contrib component not in the default set.
2. Publish a reference `builder.yaml` in the repo (e.g., `cmd/jaeger/builder.yaml`) that reproduces the default distribution using the public facades — this serves as both documentation and a CI validation artifact.

### Phase 3: CI Validation

1. Add a CI step that builds Jaeger using `ocb` + the reference `builder.yaml` and verifies the resulting binary matches the directly-compiled one (at least in terms of registered component types).
2. This ensures the facades stay in sync with the internal components.

### Phase 4 (Optional): Helper Tooling

If user feedback indicates that writing the `builder.yaml` with repeated `gomod`/`import` pairs is burdensome, provide either:
- A `jaeger-builder` wrapper that accepts a simplified manifest, or
- A Makefile target that generates a valid `builder.yaml` from the component registry.

---

## 8. Example: Building a Custom Distribution

A user wanting Jaeger + the `prometheusremotewriteexporter` from otel-contrib:

```yaml
# builder.yaml
dist:
  module: github.com/example/jaeger-custom
  name: jaeger-custom
  version: 1.0.0

extensions:
  - gomod: github.com/jaegertracing/jaeger v1.72.0
    import: github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/jaegerstorage
  - gomod: github.com/jaegertracing/jaeger v1.72.0
    import: github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/jaegerquery
  - gomod: github.com/jaegertracing/jaeger v1.72.0
    import: github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/remotesampling
  - gomod: github.com/jaegertracing/jaeger v1.72.0
    import: github.com/jaegertracing/jaeger/cmd/jaeger/components/extension/expvar

receivers:
  - gomod: go.opentelemetry.io/collector/receiver/otlpreceiver v0.154.0

exporters:
  - gomod: github.com/jaegertracing/jaeger v1.72.0
    import: github.com/jaegertracing/jaeger/cmd/jaeger/components/exporter/storageexporter
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusremotewriteexporter v0.154.0

processors:
  - gomod: go.opentelemetry.io/collector/processor/batchprocessor v0.154.0
  - gomod: github.com/jaegertracing/jaeger v1.72.0
    import: github.com/jaegertracing/jaeger/cmd/jaeger/components/processor/adaptivesampling

connectors:
  - gomod: go.opentelemetry.io/collector/connector/forwardconnector v0.154.0
```

Build with: `ocb --config builder.yaml`

---

## 9. Alternatives Considered

### 9.1 Do Nothing

Users who need custom distributions fork Jaeger and modify `components.go` directly. This works but creates maintenance burden for the user (tracking upstream changes, resolving merge conflicts on every release).

### 9.2 Plugin System (shared libraries)

Go's `plugin` package could theoretically allow loading components at runtime. However:
- Plugins require exact Go version and dependency alignment between host and plugin.
- Cross-compilation is not supported.
- The approach is widely considered fragile in the Go ecosystem.
- The OTEL Collector community explicitly rejected this approach.

### 9.3 Move Components Out of `internal`

Simply remove `internal` from the path (e.g., `cmd/jaeger/components/...`). This makes packages importable but exposes *all* types (Config structs, internal helpers, etc.), recreating the original problem of uncontrolled external dependencies on Jaeger internals.

---

## 10. Open Questions

1. **Facade location:** The facades must live under `cmd/jaeger/` due to Go's `internal` visibility rules. Within that constraint, should they be at `cmd/jaeger/components/` or a different sub-path?

2. **Which components to expose:** Should `storagecleaner` (an e2e testing helper) be exposed? Likely not — only production components should have public facades.

3. **Storage backends:** The `jaegerstorage` extension relies on storage backend implementations that are also internal. Should storage backends (Cassandra, Elasticsearch, ClickHouse, Badger) be individually importable for users who want to build a distribution with only specific backends?

4. **Telemetry factory wrapper:** Jaeger wraps the OTEL telemetry factory with custom logic (`WrapFactory` in `components.go`). Should this wrapper be part of the public API?

5. **Default config embedding:** Jaeger ships an embedded `all-in-one.yaml` configuration. Should the public API include a way to provide this default config, or is that the user's responsibility in a custom distribution?

---

## 11. References

- [OpenTelemetry Collector Builder (ocb)](https://github.com/open-telemetry/opentelemetry-collector/tree/main/cmd/builder)
- [ocb builder.yaml schema](https://github.com/open-telemetry/opentelemetry-collector/blob/main/cmd/builder/internal/builder/config.go)
- [OTEL Collector Releases](https://github.com/open-telemetry/opentelemetry-collector-releases) — example of distribution assembly
- Jaeger v2 architecture: `cmd/jaeger/internal/components.go`
