# ADR-007: Grafana Dashboard Modernization and SPM Example Validation

* **Status**: Implemented
* **Date**: 2026-03-20
* **Related Issues**: [#5833](https://github.com/jaegertracing/jaeger/issues/5833)

---

## Context

### Current State of the Monitoring Mixin

`monitoring/jaeger-mixin/` contains Jsonnet source (`dashboards.libsonnet`) and a pre-built `dashboard-for-grafana.json` for monitoring Jaeger itself (collector ingestion/export rates, storage latency, query rates, system resources).

#### Role of `dashboard-for-grafana.json`

The monitoring mixin is designed to be consumed via the [jsonnet-bundler](https://github.com/jsonnet-bundler/jsonnet-bundler) + [jsonnet](https://github.com/google/go-jsonnet) toolchain (see `monitoring/jaeger-mixin/README.md`). This is the canonical way to compose monitoring mixins into a full Prometheus/Grafana/Alertmanager stack (e.g. with kube-prometheus). However, it requires installing and understanding two additional tools.

`dashboard-for-grafana.json` is a pre-built artifact — the rendered output of the Jsonnet source — committed directly to the repository so that users can import it into Grafana without needing any Jsonnet tooling. It is the "quick start" path: download one JSON file, import it in Grafana's UI or drop it into a provisioning directory, done. This is a deliberate convenience, not an oversight, and is valuable for operators who just want to stand up a dashboard quickly.

The trade-off is that the JSON is a derived artifact that could drift from the source if not regenerated when the source changes. A CI check (see Step 3 below) is the mitigation. The docker-compose SPM example mounts this same file directly (see Step 1), so it also serves as the live validation artifact — there is only one copy.

#### Panel type problem

The Jsonnet source uses the `grafana-builder` library (`grafana-builder/grafana.libsonnet`) via `g.panel()`, which generates `"type": "graph"` panels — the deprecated Angular-based "Graph (Old)" panel type.

**Verified state of `dashboard-for-grafana.json`:**
- 10 panels of type `"graph"` (Angular, deprecated)
- 0 panels of type `"timeseries"`

**Verified state of `dashboards.libsonnet`:**
- Already uses `otelcol_*` metric names (e.g. `otelcol_receiver_accepted_spans_total`, `otelcol_exporter_sent_spans_total`) and Jaeger-internal metrics (`jaeger_storage_*`, `http_server_request_duration_seconds_*`), compatible with Jaeger v2/OTel Collector.
- However, the `grafana-builder` library's `g.panel()` always emits `"type": "graph"`, so the generated JSON is Angular-based regardless of metric names.

**Issue #5833** reports that Grafana 12 removes Angular support entirely (no toggle to re-enable). Newer Grafana versions already show deprecation warnings on every affected panel.

### Prior PRs

PRs [#7813](https://github.com/jaegertracing/jaeger/pull/7813) and [#7871](https://github.com/jaegertracing/jaeger/pull/7871) attempted to migrate metric names from legacy `jaeger_*` to `otelcol_*`. That migration is already present in `main`. Neither PR addressed the Angular panel type deprecation. Both have been closed as superseded by this ADR.

### Current State of the SPM Runnable Example

`docker-compose/monitor/` is the runnable SPM (Service Performance Monitoring) example. It contains:
- `docker-compose.yml` — runs Jaeger + microsim + Prometheus. **No Grafana service.**
- `datasource.yml` — a Grafana datasource config pointing to Prometheus at `http://prometheus:9090`. This is a leftover artifact from when Grafana was previously part of the example; it is not mounted by any container in the current compose file.
- Alternative compose files for Elasticsearch and OpenSearch backends.

Grafana was previously part of this example but was removed at some point, leaving no automated or easy-to-run way to validate that dashboards actually load and display data correctly.

### Dashboard Toolchain: Jsonnet vs. Grafana Foundation SDK

Grafana's recommended "dashboards as code" approach is the [`grafana-foundation-sdk`](https://github.com/grafana/grafana-foundation-sdk) — typed builder libraries in Go, TypeScript, Python, PHP, and Java, generated from Grafana's internal panel and dashboard schemas via the [`cog`](https://github.com/grafana/cog) code generator.

`grafonnet` (the Jsonnet library currently used) is itself a generated downstream artifact of the same pipeline. Its README explicitly marks it as **"experimental, not meant for production use."** The foundation SDK is the recommended path going forward.

For Jaeger — a Go project — the Go SDK is a natural fit. It replaces the Jsonnet toolchain (`jb` + `jsonnet` + `grafana-builder` vendor directory) with a small `go run`-able program. No new external tools are needed; the only addition is `github.com/grafana/grafana-foundation-sdk/go` as a Go module dependency. The SDK also natively generates `timeseries` panels, so migrating to it resolves the Angular panel type issue (#5833) as an inherent consequence rather than a separate fix.

Since the migration scope is small — one dashboard, five rows, ten panels, 120 lines of Jsonnet — and the translation from Jsonnet builder calls to Go builder calls is mechanical, it makes sense to do the SDK migration as Step 2, before touching the dashboard content itself. This avoids doing any work on the Jsonnet toolchain that would immediately be thrown away.

### Problem Summary

There are three problems, addressed in the order below:

1. **No live validation**: Without Grafana in the SPM example, there is no easy way to confirm that the provisioned dashboard works against real Jaeger metrics.

2. **Outdated toolchain**: The Jsonnet-based authoring toolchain (`grafana-builder`) cannot produce modern panel types and is itself deprecated. Replacing it with the Go foundation SDK resolves both the toolchain problem and the panel type problem in one step.

3. **No regeneration pipeline**: There is no `make` target or CI check to regenerate or validate `dashboard-for-grafana.json`. The file is currently in sync with the Jsonnet source, but there is no automated guard to keep it that way as the source evolves.

---

## Decision

We will work incrementally, establishing live validation first and replacing the toolchain along with fixing the dashboard content:

1. ✅ **Restore Grafana to `docker-compose/monitor/`**, mounting `dashboard-for-grafana.json` directly from its canonical location. Grafana 11.x is used initially to tolerate the existing Angular panels. _(Merged: [#8215](https://github.com/jaegertracing/jaeger/pull/8215))_
2. **Migrate dashboard source to `grafana-foundation-sdk/go`**, done in two parts:
   - ✅ **2a** — Write the Go generator, produce `dashboard-for-grafana-v2.json`, and mount both dashboards in Grafana for manual side-by-side comparison against live data. _(Merged: [#8216](https://github.com/jaegertracing/jaeger/pull/8216))_
   - ✅ **2b** — After validation, delete the Jsonnet toolchain, promote the v2 file to `dashboard-for-grafana.json`, and upgrade Grafana to 12.x. _(Merged: [#8241](https://github.com/jaegertracing/jaeger/pull/8241))_
3. ✅ **Add CI validation** to keep `dashboard-for-grafana.json` in sync with the Go source. _([#8240](https://github.com/jaegertracing/jaeger/pull/8240))_

---

## Implementation Plan

### ✅ Step 1: Restore Grafana to the SPM docker-compose example _(Merged: [#8215](https://github.com/jaegertracing/jaeger/pull/8215))_

This step restores visibility and validation capability. The dashboard loaded at this point will still have Angular panels, so Grafana 11.x is used — the last series with Angular support enabled by default.

**Single source of truth for the dashboard JSON:** The compose service mounts `monitoring/jaeger-mixin/dashboard-for-grafana.json` directly via a relative path. There is no copy of the file under `docker-compose/monitor/`.

**Files to create/modify:**

1. **`docker-compose/monitor/grafana/provisioning/datasources/prometheus.yml`** — move/rename the existing orphaned `datasource.yml` to its correct provisioning location (same content):

   ```yaml
   apiVersion: 1
   datasources:
     - name: Prometheus
       type: prometheus
       url: http://prometheus:9090
       isDefault: true
       access: proxy
       editable: true
   # Intentionally no Jaeger datasource. Grafana is included here solely as a
   # metrics dashboard tool. Jaeger UI (port 16686) is the trace visualization
   # interface for this demo. Without a trace datasource configured, Grafana's
   # trace viewer is inert. Users who want to add one can do so manually.
   ```

2. **`docker-compose/monitor/grafana/provisioning/dashboards/default.yml`** — dashboard provider config:

   ```yaml
   apiVersion: 1
   providers:
     - name: jaeger-mixin
       type: file
       options:
         path: /etc/grafana/provisioning/dashboards
   ```

3. **`docker-compose/monitor/docker-compose.yml`** — add Grafana service, mounting the dashboard JSON directly from its canonical location:

   ```yaml
   grafana:
     networks:
       - backend
     image: grafana/grafana:11.6.0
     volumes:
       - "./grafana/provisioning:/etc/grafana/provisioning"
       - "../../monitoring/jaeger-mixin/dashboard-for-grafana.json:/etc/grafana/provisioning/dashboards/jaeger.json:ro"
     environment:
       - GF_AUTH_ANONYMOUS_ENABLED=true
       - GF_AUTH_ANONYMOUS_ORG_ROLE=Admin
     ports:
       - "3000:3000"
     depends_on:
       - prometheus
   ```

   `GF_AUTH_ANONYMOUS_ENABLED=true` with `Admin` role avoids a login prompt in the local development example. The `:ro` flag prevents Grafana from writing back to the source file.

4. **`docker-compose/monitor/README.md`** — add a section noting Grafana is available at `http://localhost:3000` with the Jaeger mixin dashboard pre-loaded.

**Validation:** Run `docker compose up` and confirm the Grafana dashboard loads and panels show data from microsim-generated traces. Angular deprecation warnings are expected at this stage.

### ✅ Step 2a: Introduce the Go SDK generator alongside the existing dashboard _(Merged: [#8216](https://github.com/jaegertracing/jaeger/pull/8216))_

Write the Go generator and mount its output in Grafana alongside the existing Jsonnet-generated dashboard for side-by-side comparison. The Jsonnet source and `dashboard-for-grafana.json` are left untouched at this stage.

**Note on the current state:** `dashboard-for-grafana.json` is in sync with the Jsonnet source — both faithfully produce `"type": "graph"` panels via `grafana-builder`. The problem is the toolchain itself, not drift. After this migration, `go run` is sufficient with no external tools.

The `generate/` subdirectory is a standalone Go module (`module github.com/jaegertracing/jaeger/monitoring/jaeger-mixin/generate`), separate from the main `go.mod`. This follows the same pattern as `internal/tools/go.mod` — a build-time tool that must not become a dependency of the Jaeger binary itself.

**Existing automation compatibility:**

- **Go version sync:** `scripts/lint/check-go-version.sh` discovers all `go.mod` files via `find` and enforces they match the root `go.mod`. The new module is covered automatically with no configuration changes.
- **Renovate:** `renovate.json` extends `config:best-practices`, which auto-discovers all `go.mod` files. `grafana-foundation-sdk` will receive standard monthly dependency update PRs. No changes needed.
- **Codecov:** `.codecov.yml`'s `ignore` list includes `internal/tools`. Add `monitoring/jaeger-mixin/generate` for the same reason — it is a build tool, not production code, and must not count against the 95% coverage target. No change to `after_n_builds` is needed since the ignored directory produces no coverage upload.

**Files to create:**

```
monitoring/jaeger-mixin/
  generate/
    main.go    ← dashboard definition in Go
    go.mod     ← requires grafana-foundation-sdk/go
    go.sum
  dashboard-for-grafana-v2.json  ← generated output, committed for comparison
```

**Build:**

```makefile
.PHONY: generate-dashboards
generate-dashboards:
    go run ./monitoring/jaeger-mixin/generate > monitoring/jaeger-mixin/dashboard-for-grafana-v2.json
```

**Example panel definition** using the Go SDK's fluent builder API:

```go
func spanIngestPanel() *timeseries.PanelBuilder {
    return timeseries.NewPanelBuilder().
        Title("Span Ingest Rate").
        WithTarget(prometheus.NewDataqueryBuilder().
            Expr(`sum(rate(otelcol_receiver_accepted_spans_total[1m]))`).
            LegendFormat("success")).
        WithTarget(prometheus.NewDataqueryBuilder().
            Expr(`sum(rate(otelcol_receiver_refused_spans_total[1m])) or vector(0)`).
            LegendFormat("error"))
}
```

All ten panels translate 1:1 from the Jsonnet `g.queryPanel()` calls. The translation is mechanical and well-suited to AI assistance. The Go SDK dashboard uses a distinct title (e.g. "Jaeger (v2)") and a different `uid` from the Jsonnet one so both can coexist in Grafana.

Add a second volume mount to `docker-compose.yml` so both dashboards load simultaneously:

```yaml
volumes:
  - "./grafana/provisioning:/etc/grafana/provisioning"
  - "../../monitoring/jaeger-mixin/dashboard-for-grafana.json:/etc/grafana/provisioning/dashboards/jaeger.json:ro"
  - "../../monitoring/jaeger-mixin/dashboard-for-grafana-v2.json:/etc/grafana/provisioning/dashboards/jaeger-v2.json:ro"
```

**Validation:** Run `docker compose up` and compare both dashboards in Grafana side-by-side against live microsim data. Confirm all panels in the v2 dashboard show data, use `timeseries` panel types, and match the content of the Jsonnet dashboard.

### ✅ Step 2b: Cutover — remove Jsonnet toolchain and promote v2 dashboard _(Merged: [#8241](https://github.com/jaegertracing/jaeger/pull/8241))_

Once side-by-side validation passes, complete the migration in a follow-up PR:

- Delete `dashboards.libsonnet`, `mixin.libsonnet`, `jsonnetfile.json`, `jsonnetfile.lock.json`, and `vendor/`
- Rename `dashboard-for-grafana-v2.json` to `dashboard-for-grafana.json`
- Update the `generate-dashboards` make target to write to `dashboard-for-grafana.json`
- Remove the transitional second volume mount from `docker-compose.yml`; the single mount now points at the Go SDK-generated file
- Bump the Grafana image to `grafana/grafana:12.0.0`
- Update `README.md` to reflect the new toolchain

**File layout after cutover:**

```
monitoring/jaeger-mixin/
  generate/
    main.go
    go.mod
    go.sum
  dashboard-for-grafana.json  ← Go SDK-generated (renamed from v2)
  prometheus_alerts.yml
  README.md
```

**Validation:** Run `docker compose up` with the single dashboard mount and Grafana 12.x. Confirm all panels render with no Angular deprecation warnings.

### ✅ Step 3: Add CI validation _([#8240](https://github.com/jaegertracing/jaeger/pull/8240))_

Add a `lint-monitoring` target to the top-level `Makefile` that regenerates the dashboard to a temp file, diffs against the committed JSON, and fails if they differ. The Go generator produces deterministic output, so no additional normalization is needed. Include the target in the top-level `lint` target and in the `generated-files-check` CI job.

---

## Consequences

### Positive

- **Incremental and validatable**: Grafana is restored first (Step 1), so every subsequent change can be confirmed against a running instance before it is committed.
- **No throw-away work**: The Jsonnet toolchain is replaced before any dashboard content changes, so nothing is done twice.
- **Angular panels fixed as a by-product**: Migrating to the Go SDK resolves #5833 inherently — no overlay hacks, no separate fix step.
- **No Jsonnet toolchain**: Contributors no longer need `jb` and `jsonnet` installed. `go run` is sufficient.
- **Type safety**: The SDK catches invalid panel configurations at compile time.
- **Single source of truth**: `dashboard-for-grafana.json` is the one canonical file. The compose example mounts it directly; there are no copies to keep in sync.
- **Live validation**: Running `docker compose up` in `docker-compose/monitor/` gives Grafana at `http://localhost:3000` with the mixin dashboard pre-loaded against real Jaeger metrics.
- **CI protection**: The sync check and dashboard-linter run on every PR.
- **Low barrier for operators**: `dashboard-for-grafana.json` remains a directly-usable artifact for operators who want to load the dashboard without any tooling.

### Negative / Trade-offs

- **Temporary Grafana version**: Step 1 uses Grafana 11.x; Step 2 upgrades to 12.x. Two small bumps across separate PRs.
- **Pre-built JSON is a derived artifact**: `dashboard-for-grafana.json` can drift if the CI check is removed or skipped. The lint step is the guard.
- **`grafana-foundation-sdk` is pre-1.0**: The SDK is in public preview (v0.0.x). API stability is not guaranteed, though Grafana Labs uses it in production.

### Out of Scope / Follow-up

- **Alert rule validation**: `prometheus_alerts.yml` is not validated in CI. Consider adding `promtool check rules` to the lint step as a follow-up.
- **Elasticsearch/OpenSearch compose variants**: The SPM example has alternative compose files for ES/OpenSearch backends. Grafana integration for those variants is deferred.
