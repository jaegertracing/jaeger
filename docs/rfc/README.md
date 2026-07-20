# Request for Comments (RFCs)

This directory contains Request for Comments (RFC) documents for the Jaeger project. RFCs explore potential architectural changes, new features, or significant renovations that are under consideration but have not yet been decided upon. They serve as a record of the problem space, trade-off analysis, and proposed approaches to inform future decisions.

## What is an RFC?

An RFC is a document that describes a problem, surveys the solution space, and proposes a concrete approach — without committing to it. Unlike ADRs, which record decisions already made, RFCs are living documents open to feedback and revision. An RFC may eventually be adopted (at which point it may graduate into an ADR), modified, superseded, or simply archived as a record of considered-but-rejected ideas.

## RFCs in This Repository

- [0001: GenAI Observability Data Layer](./0001-genai-data-layer.md) - GenAI Observability Data Layer
- [0002: AI Gateway — Frontend-Driven Contextual Tools](./0002-ai-gateway-contextual-tools.md) - Per-turn UI tool registration via ACP extension method
- [0003: Simplify Running Jaeger With the AI Sidecar](./0003-simplify-ai-sidecar-setup.md) - Simplify AI sidecar setup and configuration
- [0004: Elasticsearch/OpenSearch Data Streams](./0004-elasticsearch-data-streams.md) - Data Streams as a new index management strategy for span storage
- [0005: Qualified Attribute Queries](./0005-qualified-attribute-queries.md) - Allow scoping tag/attribute queries to specific OTLP levels
- [0006: Unified Elasticsearch/OpenSearch Client](./0006-unified-elasticsearch-client.md) - Collapse the data-plane and control-plane ES/OS clients into one Jaeger-owned client
- [0009: Search Result Pagination](./0009-search-pagination.md) - Cursor-based "load more" pagination for the trace search API
