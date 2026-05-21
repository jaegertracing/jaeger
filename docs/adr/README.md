# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records (ADRs) for the Jaeger project. ADRs document important architectural decisions made during the development of Jaeger, including the context, decision, and consequences of each choice.

## What is an ADR?

An Architecture Decision Record (ADR) is a document that captures an important architectural decision made along with its context and consequences. ADRs help teams understand why certain decisions were made and provide historical context for future contributors.

## ADRs in This Repository

- [ADR-001: Cassandra FindTraceIDs Duration Query Behavior](001-cassandra-find-traces-duration.md) - Explains why duration queries in the Cassandra spanstore use a separate code path and cannot be efficiently combined with other query parameters.
- [ADR-002: MCP Server Extension](002-mcp-server.md) - Design for implementing Model Context Protocol server as a Jaeger extension for LLM integration.
- [ADR-003: Lazy Storage Factory Initialization](003-lazy-storage-factory-initialization.md) - Comparative analysis of approaches to defer storage backend initialization until actually needed.
- [ADR-004: Migrate Coverage Gating from Codecov to GitHub Actions](004-migrating-coverage-gating-to-github-actions.md) - Design for replacing Codecov PR gating with a local fan-in workflow that merges coverage profiles, gates on regression, and consolidates reporting with the existing metrics summary.
- [ADR-005: Badger Storage Record Layouts](005-badger-storage-record-layouts.md) - Documents the key and value formats used to store spans, secondary indexes, and sampling data in the Badger embedded key-value store backend.
- [ADR-006: Internal Tracing via OTel Collector TelemetryFactory](006-internal-tracing-via-otelcol-telemetry-factory.md) - Design for centralizing Jaeger's internal self-tracing through the Collector's TelemetryFactory hook, replacing per-extension manual tracer initialization and preventing recursive self-tracing loops in receivers.
- [ADR-007: Grafana Dashboard Modernization and SPM Example Validation](007-grafana-dashboards-modernization.md) - Plan to migrate monitoring mixin dashboards from deprecated Angular "Graph" panels to `timeseries` panels, add CI validation, and restore Grafana to the SPM docker-compose example for live dashboard validation.
- [ADR-008: ClickHouse Storage Schema](008-clickhouse-storage-schema.md) - Schema design for the native ClickHouse storage backend.
- [ADR-009: UI Base-Path Auto-Detection](009-ui-base-path-auto-detection.md) - Proposal to make the UI self-detect its URL prefix from `window.location`, removing the need to configure `extensions.jaeger_query.base_path` in the backend for UI delivery and enabling a single Jaeger pod to serve under multiple prefixes.
- [ADR-010: Trace Summary API for Lightweight Search Results](010-trace-summary-api.md) - Proposal for a new `FindTraceSummaries` endpoint in the v3 API that returns per-trace statistics instead of full span data, reducing search response size and enabling storage-native aggregation with a fallback for backends that do not implement it.
