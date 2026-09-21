# Request for Comments (RFCs)

This directory contains Request for Comments (RFC) documents for the Jaeger project. An RFC describes a problem, surveys the solution space, and proposes a concrete approach — so that the approach can be discussed and revised before it is built. RFCs are the starting point for new design work in Jaeger; decisions that have already been taken are recorded as ADRs in [`docs/adr/`](../adr/).

## ADR or RFC?

Write an RFC when the work has not been decided yet. Write an ADR when you are recording an outcome — a decision already taken, or a design already in the code.

| | RFC (this directory) | [ADR](../adr/) |
| --- | --- | --- |
| **Purpose** | Propose an approach and invite comment before building it | Record a decision already taken, or document a design already in the code |
| **Written** | Before the work | At or after the decision |
| **Voice** | "We propose to…", alternatives weighed, one recommended | "We decided…" / "This is how it works", with consequences |
| **Typical sections** | Abstract, Motivation, Design, Alternatives, Implementation Plan | Context, Decision, Consequences |
| **Maintenance** | Prose frozen after merge; Status and milestone tracking kept current | Edited in place for a change affecting part of it; see [ADR lifecycle](../adr/README.md#lifecycle) |
| **End state** | Implemented (optionally graduating into an ADR), Superseded, or abandoned | Extended, or superseded by a later ADR |
| **File name** | `NNNN-slug.md` (four digits) | `NNN-slug.md` (three digits) |

## Lifecycle

An RFC starts as a proposal, open to comment and revision. Once its approach is adopted, the RFC doubles as the **plan of record for the work**: it decomposes the implementation into independently shippable milestones — in an `Implementation Plan` or `Roadmap` section, and for longer-running efforts a summary `Implementation status` table near the top — and that decomposition is where delivery is tracked.

At the same time, the RFC's narrative is a point-in-time snapshot of the system and the plan as of when it was written, and is read that way later. Those two roles pull in opposite directions, and the split is resolved by editing a merged RFC in one way only:

- **Keep the delivery tracking current.** As each milestone lands, mark it ✅ and link the PR that delivered it, and update the top-level `Status` field — done by the PR implementing the milestone, in that same PR. [RFC 0007](./0007-synchronous-elasticsearch-writes.md) annotates its milestones in place; [RFC 0008](./0008-ai-gateway-mcp-tool-routing.md) keeps a status table up front. Both shapes are fine.
- **Leave the prose alone.** Do not rewrite the abstract, design sections, or diagrams to track the evolving codebase. When the design changes materially, supersede the RFC with a new one instead of editing the old one into agreement with the code. [RFC 0002](./0002-ai-gateway-contextual-tools.md) is the example: its tool-routing choice is superseded by [RFC 0008](./0008-ai-gateway-mcp-tool-routing.md), and the document itself is retained unchanged, with a note pointing forward.

### Graduating into an ADR

Once an RFC's work is fully delivered, mark it Implemented. An implemented RFC is still a proposal document, though — it carries the whole trade-off analysis and the milestone-by-milestone history, which is not what a reader wanting to know how the system works today should have to wade through. So if the resulting architecture is worth an enduring reference, graduate it: capture the outcome in a fresh [ADR](../adr/) that states the resulting design, and leave the RFC as the record of how that design was arrived at. Do not mutate the RFC into documentation.

The two documents then point at each other through their `Status` fields — the RFC's says "graduated into ADR-NNN", the ADR's says "graduated from RFC NNNN" — so a reader landing on either one finds the other. [RFC 0006](./0006-unified-elasticsearch-client.md) → [ADR-012](../adr/012-unified-elasticsearch-client.md) is the worked example.

Graduation is optional and not the only end state. An RFC may also be superseded by a later RFC, or simply abandoned — in both cases it stays in this directory as a record of what was considered.

## Conventions

- File name `NNNN-short-slug.md`, next number in sequence; title `# RFC NNNN: Title`.
- Header block: Status, Author(s), Created, Last Updated, plus Issue / Related / Supersedes links where applicable. Statuses in use: Draft, Partially Implemented, Implemented, Superseded.
- Open with an Abstract, then numbered sections starting at `1. Motivation`.
- Add an entry to the index below, prefixed with the status marker that matches the header block (see the legend there). When an RFC's status changes, change the marker in the same PR.

## RFCs in This Repository

Each entry is marked with the RFC's status: ✅ Implemented · 🚧 Partially implemented · 📝 Draft · ♻️ Superseded. The RFC's own `Status` header is the source of truth; open the document for the milestone-level detail.

- 📝 [0001: GenAI Observability Data Layer](./0001-genai-data-layer.md) - Store and query GenAI evaluation results and benchmark datasets, correlated with traces, without adding an external SQL database
- ✅ [0002: AI Gateway — Frontend-Driven Contextual Tools](./0002-ai-gateway-contextual-tools.md) - Per-turn UI tool registration via ACP extension method; its §5 tool-routing decision was later superseded by [RFC 0008](./0008-ai-gateway-mcp-tool-routing.md)
- 🚧 [0003: Simplify Running Jaeger With the AI Sidecar](./0003-simplify-ai-sidecar-setup.md) - Reduce the setup steps needed for a first local run of Jaeger with an AI sidecar
- 🚧 [0004: Elasticsearch/OpenSearch Data Streams](./0004-elasticsearch-data-streams.md) - Data Streams as a new index management strategy for span storage
- 📝 [0005: Structured Query Filters for Trace Search](./0005-structured-query-filters.md) - A structured query-filter model for trace search: level-qualified attributes, built-in fields, and boolean composition
- ✅ [0006: Unified Elasticsearch/OpenSearch Client](./0006-unified-elasticsearch-client.md) - Collapse the data-plane and control-plane ES/OS clients into one Jaeger-owned client
- 🚧 [0007: Synchronous Elasticsearch/OpenSearch Writes](./0007-synchronous-elasticsearch-writes.md) - Per-batch synchronous bulk writes so failures propagate and Kafka offsets commit only after durable persistence
- 🚧 [0008: AI Gateway — Unified MCP Tool Routing](./0008-ai-gateway-mcp-tool-routing.md) - Consolidate telemetry and UI tool dispatch through one gateway-hosted MCP server; supersedes the tool-routing decision in RFC 0002
- ✅ [0009: Lazy Storage Factory Initialization](./0009-lazy-storage-factory-initialization.md) - Defer storage backend initialization until first use, weighing a two-phase factory framework against simply deferring factory creation; extracted from ADR-003, which records the outcome
- ✅ [0010: Grafana Dashboard Modernization and SPM Example Validation](./0010-grafana-dashboards-modernization.md) - Replace the Jsonnet mixin toolchain with the Go `grafana-foundation-sdk` to escape deprecated Angular panels, and restore Grafana to the SPM example for live validation; extracted from ADR-007, which records the outcome
- ✅ [0011: Trace Summary API for Lightweight Search Results](./0011-trace-summary-api.md) - A `FindTraceSummaries` endpoint returning per-trace statistics instead of full span data, with alternatives weighed and a five-milestone rollout; extracted from ADR-010, which records the outcome
- ✅ [0012: MCP Server Extension for Jaeger](./0012-mcp-server-extension.md) - Expose Jaeger's query capabilities to LLM agents as progressive-disclosure MCP tools; extracted from ADR-002, whose surviving decisions it links to, with routing since superseded by RFC 0008
- ✅ [0013: Optional Service Name in Trace Search](./0013-optional-service-name-in-search.md) - Let backends that can search without a service name declare it, so the UI can offer an "All Services" option where the storage supports it
- 📝 [0014: Search Result Pagination](./0014-search-result-pagination.md) - End-to-end keyset pagination for trace search via an opaque page token, with backend capability declaration for honest degradation; the ES/OS design replaces the approximate terms aggregation with collapse + `search_after`
- 📝 [0015: Typed Attribute Indexing for Elasticsearch/OpenSearch](./0015-typed-attribute-indexing-elasticsearch.md) - Index attribute values at their own type, not only as keywords, so the ordered and type-qualified predicates of RFC 0005 become answerable on ES/OS
- 📝 [0016: Span Search — Returning Matching Spans Instead of Traces](./0016-span-search.md) - A `FindSpans` endpoint that keeps the spans a search matched instead of aggregating them into trace identity, paginated by RFC 0014's cursor, with room reserved for the projection and grouping clauses RFC 0005 deferred
