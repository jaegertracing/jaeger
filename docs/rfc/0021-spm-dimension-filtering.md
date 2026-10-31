# RFC 0021: SPM Dimension Filtering

- **Status:** Draft
- **Author:** [ChaitanyaD48](https://github.com/ChaitanyaD48)
- **Created:** 2026-05-15
- **Last Updated:** 2026-11-01
- **Issue:** [#7433: Filter metrics on page by tag](https://github.com/jaegertracing/jaeger/issues/7433)

## Abstract

We propose backend API support and operator-declared dimension filtering
for Prometheus-backed Service Performance Monitoring (SPM). Operators declare
filterable dimensions in Jaeger's configuration, clients retrieve those
declarations through a new endpoint, and selected values become exact-match
PromQL label selectors. This partially addresses #7433. UI integration and
Elasticsearch/OpenSearch filtering remain separate work.

## 1. Motivation

The Service Performance Monitoring (SPM) page in the Jaeger UI surfaces
call-rate, error-rate, and latency metrics aggregated by service. Today the
only filterable axis (beyond service name and span kind) is the time range.
Operators who emit metrics for multiple environments — `prod`, `staging`,
`dev` — into a single Prometheus backend cannot scope the SPM view to one
environment. The community workaround has been to run a duplicate Jaeger
deployment per environment, which is operationally expensive and defeats the
purpose of a centralised observability stack.

[#7433](https://github.com/jaegertracing/jaeger/issues/7433) requests SPM
filtering with API changes and support for Prometheus and
Elasticsearch/OpenSearch. This RFC covers the backend API and Prometheus
portion: filtering metrics by configured dimensions such as deployment
environment. It does not complete the issue. UI integration,
Elasticsearch/OpenSearch filtering, and dynamic discovery of dimensions or
their values are outside this proposal's implementation scope.

### Why PromQL Constrains the Design

PromQL is not SQL. A query like

```
sum(rate(calls{service_name="cart"}[5m])) by (service_name)
```

can only filter on labels that exist on the stored time series. The
spanmetrics connector aggregates spans using its default dimensions and any
additional attributes listed in its
[`dimensions:`](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/connector/spanmetricsconnector#configurations)
configuration. An attribute discarded during aggregation cannot be
recovered by a later metrics query. Additional filterable attributes must
therefore be preserved as dimensions when generating span metrics.

Prometheus supports discovering existing label names and values through its
[HTTP API](https://prometheus.io/docs/prometheus/latest/querying/api/#querying-metadata).
Dynamic discovery is possible for retained labels; it cannot recover
discarded attributes. This proposal chooses operator-declared dimensions
to provide a server-side allowlist and optional bounded value lists without
adding discovery queries.

### Backend Asymmetry

Elasticsearch/OpenSearch and ClickHouse can query stored span attributes,
but wiring these filters into their metric queries is separate work. This
proposal adds shared API types while implementing filtering only in the
Prometheus reader. Other readers return an empty dimension list. The API
contract is that the backend advertises which dimensions are filterable,
and clients send filters that match the advertised set.

## 2. Proposed Design

We propose a **pre-configured** dimension-filter capability for Prometheus
metrics, including the environment-filtering use case described in #7433.

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

   A separate UI integration can call this on Monitor-page load and render
   one dropdown per dimension. The backend supplies the dimension list, so
   clients do not need a separate UI-config-JSON declaration. UI changes
   are not delivered by the backend implementation.

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
   - Unexpected failures retrieving dimensions → 500.
   - Disabled metrics backend → 501.

### Naming

The Go field is `Filters` (not `Tags` or `Dimensions`) and the HTTP param is
`?filter=k:v`. This deliberately:

- Avoids the existing `?tag=k:v` parameter used by `/api/traces`, which has
  unrelated semantics for trace search.
- Reads naturally in code: `params.Filters["deployment.environment"]`.

The OTel-ecosystem word "dimensions" is reserved for the **declaration** of
the filterable set (matching the spanmetrics connector vocabulary), not for
the runtime values passed in by callers.

## 3. Consequences

### Positive

- Clients can query metrics for one environment through a shared Jaeger
  deployment. Exposing this in the Monitor page requires the separate UI
  integration.
- The new API surface (`Filters`, `?filter=`, `/api/metrics/dimensions`) is
  additive and backwards-compatible. Existing clients that don't pass
  filters are unaffected; existing readers that don't implement filter
  semantics return an empty dimension list, allowing a future UI integration
  to hide the dropdowns.
- The design makes the requirement to retain attributes during aggregation
  explicit while leaving discovery of existing labels as possible future
  work.
- Operators can configure a closed set of values for bounded dropdowns
  without querying Prometheus for label values on every page load.

### Negative

- Operators must keep `extra_dimensions:` in the Jaeger config in sync with
  `dimensions:` in their spanmetrics connector config. There is no
  cross-validation across the two configs (they live in different process
  spaces). A declared dimension whose label is absent from the stored
  metrics can produce no matching series.
- Dynamic discovery of dimensions and values is not included.
- Asymmetric backend support: today only the Prometheus metricstore honours
  `Filters`. The Elasticsearch and ClickHouse metricstores accept the field
  (it's additive on the shared `BaseQueryParameters`) and return an empty
  dimension list from `GetDimensions`. The proposed UI integration can hide
  the dropdowns for those backends. Elasticsearch/OpenSearch support
  requested in #7433 remains unfinished.
- The disabled metricstore returns `ErrDisabled` from `GetDimensions` as
  well as the three existing query methods, which maps to HTTP 501. Clients
  must handle this just as they handle 501 from `/api/metrics/calls` today.

## 4. Alternatives Considered

### A. Dynamic discovery through the Prometheus HTTP API

Discover label names and values through Prometheus's `/api/v1/labels` and
`/api/v1/label/<label_name>/values` endpoints, scoped to the relevant span
metrics, and let users select filters from those results.

**Pros**: Avoids duplicating available label names and values in Jaeger's
configuration and reflects labels already present on stored metrics.

**Cons**: Requires discovery queries and decisions about which labels to
expose, value cardinality, caching, and dropdown behavior. It still cannot
recover attributes discarded during span-to-metrics aggregation. Deferred
in favor of an explicit operator-controlled allowlist for this proposal;
it remains a feasible extension for existing labels.

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

## 5. Implementation Plan

The backend implementation is proposed in
[#8579](https://github.com/jaegertracing/jaeger/pull/8579), which is under
review:

1. Add the shared `Dimension`, `Filters`, and `GetDimensions` API types and
   methods, with empty declarations for unsupported metric backends.
2. Add and validate Prometheus `extra_dimensions` configuration, and
   generate exact-match label selectors for selected filters.
3. Expose the dimensions endpoint and parse and validate filters on metrics
   requests.
4. Validate the backend with the test plan below and document configuration
   in the SPM sample.

UI integration is separate, proposed in
[jaeger-ui#3900](https://github.com/jaegertracing/jaeger-ui/pull/3900).
Elasticsearch/OpenSearch filtering remains follow-up work under #7433;
it is not delivered by the backend PR above.

## 6. Test Plan

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
  - Unexpected `GetDimensions` failure on a filtered metrics request → 500.
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

## 7. Future Improvements

- **Elasticsearch/OpenSearch filtering.** Implement dimension declarations
  and apply `Filters` in metric queries to cover the remaining backend
  support requested in #7433.
- **ClickHouse filtering.** Apply filters in ClickHouse metric queries and
  advertise supported dimensions in a separate backend change.
- **Dynamic label discovery.** Explore discovering existing labels and
  values, with cardinality limits, caching, and UI behavior designed
  explicitly. Attributes must still be retained during aggregation.

## 8. References

- [Issue #7433](https://github.com/jaegertracing/jaeger/issues/7433) — the
  SPM filtering request partially addressed by this proposal.
- [OpenTelemetry Collector Contrib — spanmetrics connector
  configuration](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/connector/spanmetricsconnector#configurations).
- [OpenTelemetry → Prometheus translator — label name normalisation](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/pkg/translator/prometheus#metric-name).
- `internal/storage/v1/api/metricstore/interface.go` — Reader interface, including the new `GetDimensions` and `Filters`.
- `internal/config/promcfg/config.go` — `ExtraDimensions` configuration.
- `internal/storage/metricstore/prometheus/metricstore/reader.go` — PromQL injection.
- `cmd/jaeger/internal/extension/jaegerquery/internal/http_handler.go` — request validation and the new `/api/metrics/dimensions` endpoint.
