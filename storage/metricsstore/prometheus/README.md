# Overview

Contains the "abstract" Prometheus reader (accompanied by test execution helpers)
and defines the interface that each Prometheus-compliant metrics storage
implementation (e.g. Prometheus, M3, etc.) is required to implement. 

These implementations should be under: `./plugin/metrics/{db_name}/{store_type}/...`.

# Motivation

Most (if not all) of the known Prometheus-compliant metrics storage solutions follow the
[Prometheus API specification](https://prometheus.io/docs/prometheus/latest/querying/api/)
supporting the `/api/v1/query_range` endpoint, which returns metrics over a given time-range.
For example:
- [Cortex](https://cortexmetrics.io/docs/api/#range-query)
- [M3](https://m3db.io/docs/reference/m3query/api/query/)
- [Thanos](https://thanos.io/tip/components/query.md/#metric-query-flow-overview)
- [VictoriaMetrics](https://github.com/VictoriaMetrics/VictoriaMetrics#prometheus-querying-api-usage)

This is the only Prometheus endpoint Jaeger uses.

Given the broad support for the Prometheus API from well-known metrics backends,
duplication can be minimized by implementing the bulk of the logic within an "abstract"
Prometheus layer, allowing for a simple extension of this layer when adding a new
Prometheus-compliant metrics storage backend.