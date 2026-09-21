# ADR-007: Grafana Dashboard Modernization and SPM Example Validation

* **Status**: Implemented; graduated from [RFC 0010](../rfc/0010-grafana-dashboards-modernization.md)
* **Decided**: 2026-03-20 — delivered in [#8215](https://github.com/jaegertracing/jaeger/pull/8215), [#8216](https://github.com/jaegertracing/jaeger/pull/8216), [#8241](https://github.com/jaegertracing/jaeger/pull/8241), [#8240](https://github.com/jaegertracing/jaeger/pull/8240)
* **Describes the implementation as of**: 2026-07-25
* **Related Issues**: [#5833](https://github.com/jaegertracing/jaeger/issues/5833)

> **Graduation note:** The proposal behind this decision — the state of the Jsonnet mixin, the comparison of authoring toolchains, and the step-by-step migration plan — lives in [RFC 0010](../rfc/0010-grafana-dashboards-modernization.md). This ADR describes the monitoring setup as it stands today.

## Context

The Jaeger monitoring mixin dashboard was authored in Jsonnet through the `grafana-builder` library, whose `g.panel()` can only emit the Angular `graph` panel type. Grafana 12 removes Angular support outright ([#5833](https://github.com/jaegertracing/jaeger/issues/5833)), so the committed dashboard was on a path to not rendering at all. Separately, the SPM docker-compose example had no Grafana service, so there was no easy way to confirm that a dashboard change actually renders against real metrics.

Both are addressed by the same migration: authoring moved to the Go [`grafana-foundation-sdk`](https://github.com/grafana/grafana-foundation-sdk), which emits `timeseries` panels natively, and Grafana returned to the SPM example so dashboards can be checked against live data. [RFC 0010](../rfc/0010-grafana-dashboards-modernization.md) covers why the SDK was chosen over staying on Jsonnet.

## Decision

The dashboard is generated from Go source, committed as JSON, mounted into the SPM example from that one location, and kept in sync by a lint check.

### The dashboard is Go source plus its generated JSON

`monitoring/jaeger-mixin/` holds exactly three things: [`generate/`](../../monitoring/jaeger-mixin/generate/) (a standalone Go module — `main.go`, `go.mod`, `go.sum` — that defines the dashboard using the foundation SDK's builders), the generated [`dashboard-for-grafana.json`](../../monitoring/jaeger-mixin/dashboard-for-grafana.json), and a README. No Jsonnet remains: no `.libsonnet` sources, no `jsonnetfile.json`, no vendored `grafana-builder`. Contributors need no `jb` or `jsonnet` — `go run` is the whole toolchain.

`generate/` is a separate Go module deliberately, following `internal/tools/go.mod`, so the SDK never becomes a dependency of the Jaeger binary. `.codecov.yml` lists `monitoring/jaeger-mixin/generate` in its `ignore` set for the same reason: it is a build tool, not production code.

The generated dashboard carries 10 `timeseries` panels across 5 rows, and no `graph` panels — the Angular deprecation is resolved by construction rather than by patching JSON.

### The committed JSON is the single artifact, and CI enforces that it matches

`make generate-dashboards` regenerates it in place:

```
cd ./monitoring/jaeger-mixin/generate && go run . > ../dashboard-for-grafana.json
```

`make lint-monitoring` re-runs the generator and diffs its output against the committed file, failing with "dashboard-for-grafana.json is out of sync. Run 'make generate-dashboards'." The generator's output is deterministic, so no normalization is needed. That target is part of the top-level `lint` target and runs in CI from `.github/workflows/ci-lint-checks.yaml`.

Keeping the JSON committed is deliberate: an operator can import one file into Grafana without any Jaeger toolchain, which the Jsonnet-consumption path never allowed.

### The SPM example mounts that same file

`docker-compose/monitor/docker-compose.yml` runs a `grafana` service (currently `grafana/grafana:12.4.2`, digest-pinned) that mounts `../../monitoring/jaeger-mixin/dashboard-for-grafana.json` read-only into its provisioning directory — a relative path to the canonical file, not a copy. Provisioning lives in `docker-compose/monitor/grafana/provisioning/`: `datasources/prometheus.yml` points at Prometheus, and `dashboards/default.yml` registers the file provider. Anonymous auth is enabled with the Admin role so the example needs no login.

There is intentionally no Jaeger datasource: Grafana is here as a metrics dashboard, and Jaeger UI on port 16686 is the trace interface for the demo.

## Consequences

### Positive

- The dashboard cannot silently drift from its source: `lint-monitoring` fails the build if the committed JSON does not match the generator.
- `docker compose up` in `docker-compose/monitor/` gives Grafana on `http://localhost:3000` with the mixin dashboard loaded against real Jaeger metrics, so dashboard changes are verifiable locally.
- Panel definitions are type-checked at compile time, and invalid configurations fail to build rather than rendering incorrectly.
- One dashboard artifact serves both the compose example and operators importing it by hand.

### Negative

- `dashboard-for-grafana.json` is a derived artifact under version control, so every dashboard change produces a source diff and a generated diff.
- The foundation SDK is still pre-1.0 (`v0.0.x`), so its API may change under us; it arrives via Renovate like any other Go dependency.

### Neutral

- Editing the dashboard now requires Go rather than Jsonnet — a smaller toolchain for this repo's contributors, but a different one from the mixin ecosystem's convention.
- `prometheus_alerts.yml` no longer exists here: the v1 alert rules were removed as obsolete in [#8694](https://github.com/jaegertracing/jaeger/pull/8694), which also retired the open question of validating them with `promtool`.

## References

- [RFC 0010: Grafana Dashboard Modernization and SPM Example Validation](../rfc/0010-grafana-dashboards-modernization.md) — motivation, toolchain comparison, and the migration plan
- Dashboard source: [`monitoring/jaeger-mixin/generate/main.go`](../../monitoring/jaeger-mixin/generate/main.go)
- Generated dashboard: [`monitoring/jaeger-mixin/dashboard-for-grafana.json`](../../monitoring/jaeger-mixin/dashboard-for-grafana.json)
- SPM example: [`docker-compose/monitor/`](../../docker-compose/monitor/)
- Sync check: `lint-monitoring` in [`Makefile`](../../Makefile), run by [`.github/workflows/ci-lint-checks.yaml`](../../.github/workflows/ci-lint-checks.yaml)
