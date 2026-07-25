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
| **Maintenance** | Prose frozen after merge; Status and milestone tracking kept current | Frozen after merge |
| **End state** | Implemented (optionally graduating into an ADR), Superseded, or abandoned | Superseded by a later ADR |
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
- Add an entry to the index below.

## RFCs in This Repository

- [0001: GenAI Observability Data Layer](./0001-genai-data-layer.md) - Store and query GenAI evaluation results and benchmark datasets, correlated with traces, without adding an external SQL database
- [0002: AI Gateway — Frontend-Driven Contextual Tools](./0002-ai-gateway-contextual-tools.md) - Per-turn UI tool registration via ACP extension method
- [0003: Simplify Running Jaeger With the AI Sidecar](./0003-simplify-ai-sidecar-setup.md) - Reduce the setup steps needed for a first local run of Jaeger with an AI sidecar
- [0004: Elasticsearch/OpenSearch Data Streams](./0004-elasticsearch-data-streams.md) - Data Streams as a new index management strategy for span storage
- [0005: Qualified Attribute Queries](./0005-qualified-attribute-queries.md) - Allow scoping tag/attribute queries to specific OTLP levels
- [0006: Unified Elasticsearch/OpenSearch Client](./0006-unified-elasticsearch-client.md) - Collapse the data-plane and control-plane ES/OS clients into one Jaeger-owned client
- [0007: Synchronous Elasticsearch/OpenSearch Writes](./0007-synchronous-elasticsearch-writes.md) - Per-batch synchronous bulk writes so failures propagate and Kafka offsets commit only after durable persistence
- [0008: AI Gateway — Unified MCP Tool Routing](./0008-ai-gateway-mcp-tool-routing.md) - Consolidate telemetry and UI tool dispatch through one gateway-hosted MCP server; supersedes the tool-routing decision in RFC 0002
- [0009: Lazy Storage Factory Initialization](./0009-lazy-storage-factory-initialization.md) - Defer storage backend initialization until first use, weighing a two-phase factory framework against simply deferring factory creation; extracted from ADR-003, which records the outcome
