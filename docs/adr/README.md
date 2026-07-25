# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records (ADRs) for the Jaeger project. An ADR captures an architectural decision that has already been taken — the context that forced it, the decision itself, and its consequences — or documents the design of an implementation that already exists, so that later contributors can see why the system looks the way it does.

Proposals that have not been decided yet belong in [`docs/rfc/`](../rfc/) instead; that README carries the [ADR or RFC?](../rfc/README.md#adr-or-rfc) comparison.

## Lifecycle

An ADR is a record, not living documentation. It is written once — when the decision is taken, or when an existing design is being written down — and is not maintained afterwards as the code evolves. When a decision is revisited, write a new ADR stating the new decision and mark the old one superseded rather than editing the old record.

An ADR can also arrive by [graduation from an RFC](../rfc/README.md#graduating-into-an-adr): once an RFC's work is fully delivered and the resulting architecture is worth an enduring reference, the outcome is captured in a fresh ADR stating the resulting design, while the RFC stays as the record of how it was arrived at. The two point at each other through their `Status` fields. [ADR-012](012-unified-elasticsearch-client.md), graduated from [RFC 0006](../rfc/0006-unified-elasticsearch-client.md), is the example.

The same split can run in the other direction, for a document filed here that was really a proposal: the proposal content is extracted into a new RFC and the ADR is rewritten in place, keeping its number, as a record of the resulting implementation. [ADR-003](003-lazy-storage-factory-initialization.md) and [RFC 0009](../rfc/0009-lazy-storage-factory-initialization.md) came apart that way — see the historical note below.

## Conventions

- File name `NNN-short-slug.md` (three digits), next number in sequence.
- Header block: Status and Date, plus Related Issues where useful. Statuses in use: Proposed, Accepted, Implemented, Documented existing implementation.
- Sections: Context, Decision, Consequences — plus Alternatives Considered and References where relevant.
- Add an entry to the index below.

## Historical Note

ADRs 001–011 predate `docs/rfc/`. Before that directory existed, this one held both decision records and forward-looking proposals, so four of the earlier entries carried content that belongs in an [RFC](../rfc/README.md): [002](002-mcp-server.md), [003](003-lazy-storage-factory-initialization.md) and [007](007-grafana-dashboards-modernization.md) are proposals outright, and [010](010-trace-summary-api.md) paired a genuine API record with a milestone tracker.

Three have since been split, with the proposal moved to an RFC and the ADR rewritten as a record of the resulting implementation: [ADR-003](003-lazy-storage-factory-initialization.md) → [RFC 0009](../rfc/0009-lazy-storage-factory-initialization.md), [ADR-007](007-grafana-dashboards-modernization.md) → [RFC 0010](../rfc/0010-grafana-dashboards-modernization.md), and [ADR-010](010-trace-summary-api.md) → [RFC 0011](../rfc/0011-trace-summary-api.md). ADR-002 still carries a filing note flagging the mismatch, because the architecture it describes is still moving under [RFC 0008](../rfc/0008-ai-gateway-mcp-tool-routing.md); [#9115](https://github.com/jaegertracing/jaeger/issues/9115) tracks the remaining work.

Nothing here is renumbered or relocated to correct that. ADR paths and numbers are cited from code comments (ADR-009 in `static_handler.go`, ADR-010 in `summary.go`), from externally submitted [OpenSSF badge evidence](../security/openssf-gold-evidence.md) (ADR-004), and from issues and PRs outside this repo, so moving a file silently breaks references we do not control. RFC numbers also run chronologically, so a document from January 2026 cannot be given a number above RFC 0008 without the sequence lying about its order. Each document's own Status field says where it actually stands. New proposals go to [`docs/rfc/`](../rfc/).

## ADRs in This Repository

- [ADR-001: Cassandra FindTraceIDs Duration Query Behavior](001-cassandra-find-traces-duration.md) - Explains why duration queries in the Cassandra spanstore use a separate code path and cannot be efficiently combined with other query parameters.
- [ADR-002: MCP Server Extension](002-mcp-server.md) - Design for implementing Model Context Protocol server as a Jaeger extension for LLM integration.
- [ADR-003: Lazy Storage Factory Initialization](003-lazy-storage-factory-initialization.md) - Storage factories are built on first request and cached, with configuration validated at startup, so a declared but unused backend opens no connections and cannot fail startup. Graduated from RFC 0009.
- [ADR-004: Migrate Coverage Gating from Codecov to GitHub Actions](004-migrating-coverage-gating-to-github-actions.md) - Replaces Codecov PR gating with a local fan-in workflow that merges coverage profiles, gates on regression, and consolidates reporting with the existing metrics summary.
- [ADR-005: Badger Storage Record Layouts](005-badger-storage-record-layouts.md) - Documents the key and value formats used to store spans, secondary indexes, and sampling data in the Badger embedded key-value store backend.
- [ADR-006: Internal Tracing via OTel Collector TelemetryFactory](006-internal-tracing-via-otelcol-telemetry-factory.md) - Design for centralizing Jaeger's internal self-tracing through the Collector's TelemetryFactory hook, replacing per-extension manual tracer initialization and preventing recursive self-tracing loops in receivers.
- [ADR-007: Grafana Dashboard Modernization and SPM Example Validation](007-grafana-dashboards-modernization.md) - The monitoring mixin dashboard is generated from Go source via `grafana-foundation-sdk`, committed as `timeseries`-panel JSON, mounted into the SPM docker-compose example from that one location, and kept in sync by a lint check. Graduated from RFC 0010.
- [ADR-008: ClickHouse Storage Schema](008-clickhouse-storage-schema.md) - Schema design for the native ClickHouse storage backend.
- [ADR-009: UI Base-Path Auto-Detection](009-ui-base-path-auto-detection.md) - The UI self-detects its URL prefix from `window.location`, removing the need to configure `extensions.jaeger_query.base_path` in the backend for UI delivery and letting a single Jaeger pod serve under multiple prefixes.
- [ADR-010: Trace Summary API for Lightweight Search Results](010-trace-summary-api.md) - A `FindTraceSummaries` endpoint in the v3 API returning per-trace statistics instead of full span data, carried through `tracestore.Reader` with an embeddable fallback so unsupported backends still serve it. Graduated from RFC 0011.
- [ADR-011: Custom Distributions via Public Facade Packages](011-custom-distributions.md) - Double-dispatch facade pattern enabling users to build custom Jaeger distributions with ocb without splitting into a multi-module repository.
- [ADR-012: Unified Elasticsearch/OpenSearch Client](012-unified-elasticsearch-client.md) - The single Jaeger-owned `esclient` that carries all ES/OS traffic (searches, bulk writes, and index/rollover/ILM administration) over one transport and auth stack, replacing the `olivere/elastic` data-plane client. Graduated from RFC 0006.
