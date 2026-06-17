# ADR-011: Custom Distributions via Public Facade Packages

* **Status**: Implemented
* **Date**: 2026-06-17

## Context

Jaeger v2 is built as an OpenTelemetry Collector distribution with custom components.
All Jaeger-specific components live under `cmd/jaeger/internal/`, making them
unimportable by external Go code. This was intentional during the v1→v2 migration to
prevent external projects from depending on Jaeger internals.

Now that the migration is complete, users need to assemble custom Jaeger distributions —
adding a custom exporter for a proprietary backend, including an otel-contrib receiver
not in the default set, or embedding an in-house processor. The standard mechanism for
this is [ocb](https://github.com/open-telemetry/opentelemetry-collector/tree/main/cmd/builder)
(OpenTelemetry Collector Builder), which requires importable Go packages exposing
component factory functions.

## Decision

Use a **double-dispatch public facade** pattern: two layers of thin packages that
re-export `NewFactory` from internal implementations.

```
components/                          # Layer 1: user-facing (clean import paths)
├── extension/jaegerstorage/         #   → delegates to Layer 2
├── ext/                             #   third-party aliases (single layer)
│   ├── receiver/otlpreceiver/
│   └── ...
cmd/jaeger/components/               # Layer 2: bridge (can access internal/)
├── extension/jaegerstorage/         #   → imports cmd/jaeger/internal/...
```

Each facade is a single `var NewFactory = impl.NewFactory` — no executable code,
no wrapping functions.

### Key Decisions

1. **Single `go.mod` preserved.** No multi-module split. All facades share the main
   Jaeger module. The `ocb` manifest uses the `gomod`/`import` field separation to
   reference multiple packages from one module.

2. **Double-dispatch required by Go's `internal` visibility rule.** Only code under
   `cmd/jaeger/` can import `cmd/jaeger/internal/`. The top-level `components/` cannot
   reach internal directly, so it delegates through `cmd/jaeger/components/`.

3. **Third-party components get a single-layer alias in `components/ext/`.** These
   directly re-export upstream `NewFactory` — no bridge needed since they don't touch
   `internal/`. They exist so `builder.yaml` can reference all components via one
   `gomod` entry without pinning individual upstream versions.

4. **Telemetry factory in a subpackage.** Jaeger wraps the default telemetry factory
   to filter TracerProviders (preventing recursive self-tracing). This logic lives in
   `cmd/jaeger/internal/telemetryfactory/` — a subpackage that breaks what would
   otherwise be an import cycle between `components.go` and the telemetry facade.

5. **Native binary imports the top-level `components/` layer.** This ensures the full
   facade chain is exercised by the standard `go build`, not just the ocb CI job.

### API Commitment

The facades expose exactly one symbol per component: `var NewFactory`. This does not
create new API commitments beyond what already exists:

- **Component IDs** (`jaeger_query`, `jaeger_storage`, `remote_sampling`, etc.) are
  already a de facto public API — every user's config YAML references them. Renaming
  any ID is a breaking change regardless of whether facades exist.
- **Config behavior** is consumed via YAML, not Go types. Config structs remain in
  `internal/` and are not exposed through the facades.
- **Telemetry behavior** is internal to the factory implementation. The facade returns
  an opaque `Factory` value; consumers cannot depend on its internals.
- **Transitive dependencies** are already committed via `go.mod`. The facades don't
  add new dependencies — they re-export factories that the binary already links.

The facades make an existing implicit contract (component IDs + config schema) easier
to consume via ocb, but do not expand the API surface beyond what shipping a binary
with these components already commits to.

## Alternatives Considered

### Multi-Module Repository

Split each component into its own `go.mod`. Rejected due to maintenance cost:
coordinating versions across 8+ modules, managing cross-module replace directives,
updating all go.mod files on shared dependency bumps.

### Plugin System (shared libraries)

Go's `plugin` package for runtime loading. Rejected: requires exact Go version and
dependency alignment, no cross-compilation, widely considered fragile, explicitly
rejected by the OTEL Collector community.

### Move Components Out of `internal`

Remove `internal` from paths. Rejected: exposes all types (Config structs, internal
helpers), recreating the original problem of uncontrolled external dependencies.

## Consequences

- Users can build custom Jaeger distributions with `ocb` using a `builder.yaml` that
  references `github.com/jaegertracing/jaeger` as a single `gomod` entry.
- CI validates the ocb build via `ci-ocb-build.yml` (builds binary, runs integration
  tests for health checks, UI serving, trace ingestion, and sampling API).
- Facade maintenance is minimal: each file is a one-liner that can be code-generated.
- Adding a new Jaeger component requires adding corresponding facade files in both
  `cmd/jaeger/components/` and `components/`.

## References

- [OpenTelemetry Collector Builder (ocb)](https://github.com/open-telemetry/opentelemetry-collector/tree/main/cmd/builder)
- Reference manifest: `cmd/jaeger/builder.yaml`
- Implementation: `components/`, `cmd/jaeger/components/`
