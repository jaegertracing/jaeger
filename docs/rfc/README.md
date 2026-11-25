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
| **Maintenance** | Prose rewritten while the work is in flight and frozen once Implemented; Status and milestone tracking kept current | Edited in place for a change affecting part of it; see [ADR lifecycle](../adr/README.md#lifecycle) |
| **End state** | Implemented (optionally graduating into an ADR), Superseded, or abandoned | Extended, or superseded by a later ADR |
| **File name** | `NNNN-slug.md` (four digits) | `NNN-slug.md` (three digits) |

## Lifecycle

An RFC starts as a proposal, open to comment and revision. Once its approach is adopted, the RFC doubles as the **plan of record for the work**: it decomposes the implementation into independently shippable milestones — in an `Implementation Plan` or `Roadmap` section, and for longer-running efforts a summary `Implementation status` table near the top — and that decomposition is where delivery is tracked.

The delivery tracking of an RFC is always kept current. How far its prose is kept in agreement with the work depends on whether the work is finished:

- **Keep the delivery tracking current.** The PR that implements a milestone updates the RFC itself: it marks the milestone ✅ and links itself on that milestone. The top-level `Status` field names no PRs. It gives the status, at most the milestones that are done ("Partially Implemented, M1–M4 done"), and the pointer to an ADR or to a superseding RFC where there is one. [RFC 0007](./0007-synchronous-elasticsearch-writes.md) annotates its milestones in place; [RFC 0008](./0008-ai-gateway-mcp-tool-routing.md) keeps a status table up front. Both shapes are fine. Put the ✅ at the start of the bullet, right after the list marker and before the bold label (`- ✅ **M4 — Elasticsearch/OpenSearch.** …`), because a checkmark buried mid-sentence cannot be scanned down the left margin; the "Delivered in #NNNN, which …" note stays inside the text and carries no emoji of its own. If other delivered entries in the document use a different placement, normalize them in the same change rather than adding another variant.
- **An RFC still being implemented is a live document.** While its status is Draft or its milestones are in flight, implementation turns up findings and reverses decisions, and the RFC is expected to be rewritten to match — that is the point of writing one down. Update the prose that a decision moved past, and say what is true now rather than narrating the document's own history. When implementation rules out an approach that the RFC proposed, move that approach to the alternatives considered and give the reason it was discarded. State the reason as a property of the approach, not as an account of what the RFC used to say.
- **An RFC whose work is finished is a snapshot.** Once its status is Implemented, do not rewrite its prose, abstract, or diagrams to track the evolving codebase: from then on the narrative is a record of the state and plan at the time, and the code has moved on for reasons the RFC never claimed to cover. When the design changes materially after that, supersede the RFC with a new one instead of editing the old one into agreement with the code. [RFC 0002](./0002-ai-gateway-contextual-tools.md) is the example: its tool-routing choice is superseded by [RFC 0008](./0008-ai-gateway-mcp-tool-routing.md), and the document itself is retained unchanged, with a note pointing forward.

### Graduating into an ADR

Once an RFC's work is fully delivered, mark it Implemented. An implemented RFC is still a proposal document, though — it carries the whole trade-off analysis and the milestone-by-milestone history, which is not what a reader wanting to know how the system works today should have to wade through. So if the resulting architecture is worth an enduring reference, graduate it: capture the outcome in a fresh [ADR](../adr/) that states the resulting design, and leave the RFC as the record of how that design was arrived at. Do not mutate the RFC into documentation.

The two documents then point at each other through their `Status` fields — the RFC's says "graduated into ADR-NNN", the ADR's says "graduated from RFC NNNN" — so a reader landing on either one finds the other. [RFC 0006](./0006-unified-elasticsearch-client.md) → [ADR-012](../adr/012-unified-elasticsearch-client.md) is the worked example.

Graduation is optional and not the only end state. An RFC may also be superseded by a later RFC, or simply abandoned — in both cases it stays in this directory as a record of what was considered.

## Evidence for claims about other systems

A design often depends on how a database or another external system behaves: what a statement does on a cluster, what happens when a node restarts, which error a misconfiguration produces. The author tests such a claim and records the test in the RFC, so that a reviewer checks that the test is adequate instead of repeating it or taking the claim on trust.

- **Run the test before stating the claim.** Documentation describes what a system is meant to do, and a test shows what the tested version does. Where the two differ, the RFC says so.
- **Script the test.** The files that run it live in a directory next to the RFC, named after the RFC file without its extension: `NNNN-short-slug/`. One script starts the environment, starts from a clean state, runs every test, and prints each result under an identifier. A reader can run it again, and nobody has to in order to review the RFC.
- **Commit the output.** The output of the run is committed in the same directory, and it names the version of the system that was tested.
- **Cite the test from the claim.** An appendix of the RFC lists the tests with their results, and each claim in the body names the test that backs it.
- **List what was not tested.** A claim that rests on documentation alone, or that cannot be tested, appears in a "Not tested" list with the reason.
- **Review the test as evidence, not as product code.** The files in `NNNN-short-slug/` are not built, shipped or maintained after the RFC merges. A review checks that each test is adequate for the claim that cites it and that the committed output shows what the RFC says. The standards for production code, such as naming, structure, error handling and test coverage, do not apply to them.

[RFC 0022](./0022-clickhouse-schema-migrations.md) is the example: its Appendix A cites the tests in [`0022-clickhouse-schema-migrations/`](./0022-clickhouse-schema-migrations/).

## Conventions

- File name `NNNN-short-slug.md`, next number in sequence; title `# RFC NNNN: Title`.
- Files that accompany an RFC, such as test scripts, their recorded output and images, go in a directory `NNNN-short-slug/` next to it.
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
- ✅ [0007: Synchronous Elasticsearch/OpenSearch Writes](./0007-synchronous-elasticsearch-writes.md) - Per-batch synchronous bulk writes so failures propagate and Kafka offsets commit only after durable persistence
- 🚧 [0008: AI Gateway — Unified MCP Tool Routing](./0008-ai-gateway-mcp-tool-routing.md) - Consolidate telemetry and UI tool dispatch through one gateway-hosted MCP server; supersedes the tool-routing decision in RFC 0002
- ✅ [0009: Lazy Storage Factory Initialization](./0009-lazy-storage-factory-initialization.md) - Defer storage backend initialization until first use, weighing a two-phase factory framework against simply deferring factory creation; extracted from ADR-003, which records the outcome
- ✅ [0010: Grafana Dashboard Modernization and SPM Example Validation](./0010-grafana-dashboards-modernization.md) - Replace the Jsonnet mixin toolchain with the Go `grafana-foundation-sdk` to escape deprecated Angular panels, and restore Grafana to the SPM example for live validation; extracted from ADR-007, which records the outcome
- ✅ [0011: Trace Summary API for Lightweight Search Results](./0011-trace-summary-api.md) - A `FindTraceSummaries` endpoint returning per-trace statistics instead of full span data, with alternatives weighed and a five-milestone rollout; extracted from ADR-010, which records the outcome
- ✅ [0012: MCP Server Extension for Jaeger](./0012-mcp-server-extension.md) - Expose Jaeger's query capabilities to LLM agents as progressive-disclosure MCP tools; extracted from ADR-002, whose surviving decisions it links to, with routing since superseded by RFC 0008
- ✅ [0013: Optional Service Name in Trace Search](./0013-optional-service-name-in-search.md) - Let backends that can search without a service name declare it, so the UI can offer an "All Services" option where the storage supports it
- 📝 [0014: Search Result Pagination](./0014-search-result-pagination.md) - End-to-end keyset pagination for trace search via an opaque page token, with backend capability declaration for honest degradation; the ES/OS design replaces the approximate terms aggregation with collapse + `search_after`
- 📝 [0015: Typed Attribute Indexing for Elasticsearch/OpenSearch](./0015-typed-attribute-indexing-elasticsearch.md) - Index attribute values at their own type, not only as keywords, so the ordered and type-qualified predicates of RFC 0005 become answerable on ES/OS
- 📝 [0016: Span Search — Returning Matching Spans Instead of Traces](./0016-span-search.md) - A `FindSpans` endpoint that keeps the spans a search matched instead of aggregating them into trace identity, paginated by RFC 0014's cursor, with room reserved for the projection and grouping clauses RFC 0005 deferred
- 🚧 [0017: Evolving the ClickHouse Schema](./0017-clickhouse-schema-vs-clickstack.md) - Changes to Jaeger's ClickHouse schema, several influenced by ClickStack: codecs, `LowCardinality` and table settings, configurable attribute hoisting, a `key=value` text index, and typed `Map` columns for attributes, opt-in until a retest gates the default
- 📝 [0018: Streaming Responses in the API v3 HTTP Gateway](./0018-streaming-http-api.md) - Stream server-streaming API v3 RPCs over HTTP as newline-delimited JSON behind an `Accept`-header opt-in, with in-band errors, per-message-flushed gzip, and the buffered response unchanged for existing clients
- 📝 [0019: SQL as an Input Query Language for Trace Search](./0019-sql-query-surface.md) - Accept a read-only SQL `SELECT` over a virtual `spans`/`events`/`links`/`traces` schema, compile it in the query service to the RFC 0005 expression tree and the RFC 0016 result-shaping fields, keep joins and the structural tier out of scope, and parse it with an existing pure-Go library rather than a new grammar
- 📝 [0020: Badger Span Kind Index](./0020-badger-spankind-index.md) - Extend Badger's operation index and cache to support returning and filtering operations by span kind in `GetOperations`
- 📝 [0021: SPM Dimension Filtering](./0021-spm-dimension-filtering.md) - Add backend API support and operator-declared dimension filtering for Prometheus-backed SPM; UI integration and Elasticsearch/OpenSearch filtering remain separate work
- 📝 [0022: ClickHouse Schema Migrations](./0022-clickhouse-schema-migrations.md) - Amend the ClickHouse schema after it reached users by keeping the per-object templates as the living schema and adding hand-written per-object deltas per schema version, each applied whole by the binary or by the operator and checked against the templates in CI; weighed against golang-migrate and in-house versioned deltas, the Cassandra full-copy layout, generated deltas and reconciliation
