# ADR-010: SPM Dimension Filtering

* **Status**: Proposed
* **Date**: 2026-05-15

## Context

The Service Performance Monitoring (SPM) page in the Jaeger UI surfaces
call-rate, error-rate, and latency metrics aggregated by service. Today the
only filterable axis (beyond service name and span kind) is the time range.
Operators who emit metrics for multiple environments — `prod`, `staging`,
`dev` — into a single Prometheus backend cannot scope the SPM view to one
environment. The community workaround has been to run a duplicate Jaeger
deployment per environment, which is operationally expensive and defeats the
purpose of a centralised observability stack.

This is tracked by two upstream issues:

- [#5438](https://github.com/jaegertracing/jaeger/issues/5438) (May 2024) —
  the narrow ask: "filter SPM by environment tag" so a single Jaeger pod can
  serve multiple environments.
- [#7433](https://github.com/jaegertracing/jaeger/issues/7433) (Aug 2025) —
  the broader proposal: arbitrary tag filtering across SPM, with both
  Prometheus and Elasticsearch metric backends.

This ADR addresses **only #5438**. The arbitrary-tag question (#7433) raises
separate product/UX problems — how does a user discover available tags? How
should the dropdown behave? How is value cardinality handled? — and should
be designed independently. Conflating the two led to a design that PromQL
cannot satisfy.

### Why PromQL Constrains the Design

PromQL is not SQL. A query like

```
sum(rate(calls{service_name="cart"}[5m])) by (service_name)
```

can only filter on labels that already exist on the time series. There is no
way to "filter by a tag that may exist." The spanmetrics connector that
produces these time series only emits labels listed in its
[`dimensions:`](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/connector/spanmetricsconnector#configurations)
configuration; everything else is collapsed at ingest time and unrecoverable
at query time. So the operator must pre-declare every label they want to
filter on. There is no engineering around this — it is a property of the
Prometheus data model itself.

This is the central design constraint: arbitrary-tag filtering against a
Prometheus-backed SPM is fundamentally not possible. A small,
operator-declared set is.

### Backend Asymmetry

Other metric backends — Elasticsearch, ClickHouse — store the raw span
attributes and could in principle support more flexible filtering. That
asymmetry is acceptable for this ADR: the SPM API surface is defined here in
terms of the dimensions the Prometheus backend can support, and other
backends opt in (or not) to richer behaviour in their own time. The API
contract is "the backend advertises which dimensions are filterable;
clients send filters that match the advertised set."

## Decision

Introduce a small, **pre-configured** dimension-filter capability scoped to
the environment-segregation use case in #5438.

### Mechanism

1. **Operator declares the filterable dimensions in the Jaeger backend
   YAML**, under the Prometheus metric backend:

   ```yaml
   metric_backends:
     some_metrics_storage:
       prometheus:
         endpoint: http://prometheus:9090
         extra_dimensions:
           - name: deployment.environment
             display_name: Environment
             values: [prod, staging, dev]
   ```

   The same names must already appear in the spanmetrics connector's
   `dimensions:` list — this is the operator's responsibility and is
   identical in spirit to the existing requirement that span-kind and
   service-name labels exist on the metrics.

2. **The Jaeger Query backend exposes the declared dimensions** via a new
   endpoint:

   ```
   GET /api/metrics/dimensions
   → [
       { "name": "deployment.environment",
         "displayName": "Environment",
         "values": ["prod", "staging", "dev"] }
     ]
   ```

   The Jaeger UI calls this on Monitor-page load and renders one dropdown
   per dimension. No UI-config-JSON changes are required: the dimension list
   is dynamic and backend-driven.

3. **Selected filter values are sent on the metrics endpoints** as repeated
   `filter=key:value` query params:

   ```
   GET /api/metrics/calls?service=cart
     &filter=deployment.environment:prod
   ```

   The Go API surface gains a `Filters map[string]string` on
   `metricstore.BaseQueryParameters`. The HTTP parser reuses the existing
   `key:value` convention from trace search's `?tag=` parameter.

4. **The Prometheus reader injects the filters as PromQL label selectors**,
   in the same position and pattern as the existing `span_kind` selector:

   ```
   sum(rate(calls{service_name=~"cart",
                  span_kind=~"SPAN_KIND_SERVER",
                  deployment_environment="prod"}[10m]))
     by (service_name)
   ```

   Dot-separated OTel attribute names are converted to underscore form to
   match the OTel→Prometheus exporter's label name translation. Values are
   emitted with Go's `%q` (which produces PromQL-compatible string literals)
   and the matcher is `=` (exact), not `=~` (regex), to keep the attack
   surface and cardinality predictable.

5. **The HTTP handler validates each request's filters** against
   `Reader.GetDimensions(ctx)` before invoking the reader:

   - Unknown filter keys → 400.
   - Values not in the declared `Values:` set (when configured) → 400.
   - Values containing control characters, double quotes, or backslashes → 400
     (length capped at 256).

### Naming

The Go field is `Filters` (not `Tags` or `Dimensions`) and the HTTP param is
`?filter=k:v`. This deliberately:

- Avoids the existing `?tag=k:v` parameter used by `/api/traces`, which has
  unrelated semantics for trace search.
- Avoids collision with the **separate, future** free-form arbitrary-tag
  feature in #7433; that one is free to keep "tags" terminology when its
  own design is settled.
- Reads naturally in code: `params.Filters["deployment.environment"]`.

The OTel-ecosystem word "dimensions" is reserved for the **declaration** of
the filterable set (matching the spanmetrics connector vocabulary), not for
the runtime values passed in by callers.

## Consequences

### Positive

- A single Jaeger pod can serve metrics for multiple environments without
  duplicating the deployment — the original #5438 ask.
- The new API surface (`Filters`, `?filter=`, `/api/metrics/dimensions`) is
  additive and backwards-compatible. Existing clients that don't pass
  filters are unaffected; existing readers that don't implement filter
  semantics return an empty dimension list and the UI hides the dropdowns.
- The PromQL constraint is acknowledged explicitly rather than papered over,
  so future contributors don't waste cycles trying to add "discover tags
  dynamically from Prometheus" features.
- An operator-declared closed set of values means the UI dropdowns are
  bounded, predictable, and don't require an expensive `label_values()`
  round-trip on every page load.

### Negative

- Operators must keep `extra_dimensions:` in the Jaeger config in sync with
  `dimensions:` in their spanmetrics connector config. There is no
  cross-validation across the two configs (they live in different process
  spaces). A typo silently produces an unfiltered query.
- Free-form arbitrary-tag filtering remains a separate problem for #7433.
- Asymmetric backend support: today only the Prometheus metricstore honours
  `Filters`. The Elasticsearch and ClickHouse metricstores accept the field
  (it's additive on the shared `BaseQueryParameters`) and return an empty
  dimension list from `GetDimensions`. The UI hides the dropdowns for those
  backends.
- The disabled metricstore returns `ErrDisabled` from `GetDimensions` as
  well as the three existing query methods, which maps to HTTP 501. Clients
  must handle this just as they handle 501 from `/api/metrics/calls` today.

## Alternatives Considered

### A. Free-form tag filtering on top of PromQL `label_values()`

Discover labels on the fly via Prometheus's `label_values()` and let users
filter on anything that appears.

**Pros**: Zero configuration; supports arbitrary tags.

**Cons**: `label_values()` only returns labels that already exist on the
time series — i.e. labels the operator already declared in the spanmetrics
connector. Discovering them is the operator's job and not solved by this.
Furthermore, a fully dynamic dropdown raises UX questions (cardinality,
free-text vs. select, hierarchical labels) that belong in a separate design
discussion. Rejected because it conflates two problems (#5438 and #7433)
and the additional flexibility doesn't actually unblock the
environment-segregation use case.

### B. Operator declares dropdowns in the UI-config JSON only

Put the dimension list in `config-ui.json`, the same file that defines the
menu and other UI options. The backend accepts whatever the UI sends without
validating against an allowlist.

**Pros**: Zero new backend config; one declaration site.

**Cons**: The backend trusts the client. Anyone with HTTP access to the
metrics API can pass arbitrary label selectors into PromQL, which is both a
security concern (label-cardinality DoS, query-shape probing) and a
maintenance hazard (no server-side validation means typos succeed silently
and produce subtly wrong results). Rejected.

### C. Declare in both backend YAML and UI-config JSON

Declare names in backend YAML (for validation) and dropdown definitions
(display names, allowed values) in UI-config JSON.

**Pros**: Cleanest separation of concerns between API-surface and UI
rendering.

**Cons**: Two configs the operator must keep in lockstep — three when you
count the spanmetrics connector. Doubles the surface area for
silent-misconfiguration regressions. Rejected.

### D. Inject into `JAEGER_STORAGE_CAPABILITIES`

The static-asset handler already injects a `JAEGER_STORAGE_CAPABILITIES`
JSON object into `index.html`. We could extend that with a
`spmDimensions` field.

**Pros**: Symmetric with the existing capabilities-injection mechanism;
no extra round-trip on page load.

**Cons**: The capabilities object today carries booleans
(`archiveStorage`, `dependenciesV2`, …) — short, static, page-load-cached
values. Dimension lists are richer, may change at runtime if the operator
hot-reloads config, and are only relevant to the Monitor page (not the
trace-search or dependency pages). A dedicated endpoint is a better fit
for the access pattern, and keeps the HTML payload lean. Rejected.

## Test Plan

### Unit tests (automated, in CI)

- `internal/storage/metricstore/prometheus/metricstore/reader_test.go`
  - Single filter selector emitted in PromQL for latencies, calls, and
    errors (both numerator and denominator).
  - Multiple filters emitted in deterministic alphabetical order.
  - Dot → underscore conversion in label names.
  - Values containing characters that need PromQL string escaping (e.g.
    `"`) emitted via `%q` without breaking the query.
  - Empty `Filters` produces PromQL byte-identical to today (no regression).
  - `GetDimensions` returns the configured slice (defensive copy).
- `internal/config/promcfg/config_test.go`
  - Valid `extra_dimensions:` config validates.
  - Invalid name (regex violation) → error.
  - Duplicate names → error.
- `cmd/jaeger/internal/extension/jaegerquery/internal/query_parser_test.go`
  - Single `?filter=k:v` parsed into `Filters` map.
  - Repeated `?filter=k:v` parsed into the map.
  - Malformed (missing colon, empty value) → 400.
- `cmd/jaeger/internal/extension/jaegerquery/internal/http_handler_test.go`
  - `GET /api/metrics/dimensions` happy path.
  - `ErrDisabled` → 501.
  - Unknown filter key → 400 (mock advertises a different dimension set).
  - Value not in `Values:` → 400.
  - Free-text value containing a control character → 400.
  - Valid filter ⇒ underlying mock receives `Filters` map populated.

### Integration / manual verification

1. Build `make build-jaeger`.
2. In `cmd/jaeger/config-spm.yaml`, set the spanmetrics connector
   `dimensions:` to include `deployment.environment`, and enable the
   `extra_dimensions:` block on the Prometheus backend.
3. Send synthetic traces from two services tagged with
   `deployment.environment=prod` and `staging`.
4. `curl /api/metrics/dimensions` returns the declared dimension.
5. `curl /api/metrics/calls?service=svc&filter=deployment.environment:prod`
   produces a PromQL query containing `deployment_environment="prod"`.
6. Each negative case (unknown key, out-of-set value, control character)
   returns 400.
7. Point `metric_backends` at `disabled` → `/api/metrics/dimensions`
   returns 501.

## Future Improvements

- **Elasticsearch / ClickHouse symmetry.** Once this lands, opening per-
  backend issues to wire `Filters` into the ES bool query and ClickHouse
  `WHERE` clauses is cheap — the API surface is already in place.
- **Free-form / arbitrary tag filtering (#7433).** A separate ADR, with its
  own product/UX discussion. It will need to address tag discovery,
  cardinality bounds, multi-value semantics, and dropdown behaviour. The
  `Filters` field is named generically enough that #7433's solution can
  coexist with this one without breaking the API.

## References

- [Issue #5438](https://github.com/jaegertracing/jaeger/issues/5438) — the
  environment-filter use case driving this ADR.
- [Issue #7433](https://github.com/jaegertracing/jaeger/issues/7433) — the
  broader arbitrary-tag proposal (intentionally out of scope here).
- [OpenTelemetry Collector Contrib — spanmetrics connector
  configuration](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/connector/spanmetricsconnector#configurations).
- [OpenTelemetry → Prometheus translator — label name normalisation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/translator/prometheus#metric-name).
- `internal/storage/v1/api/metricstore/interface.go` — Reader interface, including the new `GetDimensions` and `Filters`.
- `internal/config/promcfg/config.go` — `ExtraDimensions` configuration.
- `internal/storage/metricstore/prometheus/metricstore/reader.go` — PromQL injection.
- `cmd/jaeger/internal/extension/jaegerquery/internal/http_handler.go` — request validation and the new `/api/metrics/dimensions` endpoint.
