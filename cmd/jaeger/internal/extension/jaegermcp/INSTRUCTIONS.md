Jaeger is a distributed tracing backend. A trace is a tree of spans representing a request or workflow within layers or components of a system. Each span records a unit of work with timing, status, and metadata, using the OpenTelemetry data model.

## Skills

At the start of any session, call `resources/list` to discover available skills. Then read the `skill://skills-index` resource — it lists all available skills and when to use each one. When a task matches a skill description, read that skill's resource via `resources/read` and follow its instructions.

Example workflow:
1. `resources/list` → see all `skill://` resources
2. `resources/read skill://skills-index` → discover which skills exist and what they do
3. `resources/read skill://<name>` → load the full instructions for the matching skill

## Investigation Strategy

These tools support progressive disclosure to manage context density. While they can be called in any order based on available data, prefer starting with broad discovery (`get_services` or `search_traces`) or structural overviews (`get_trace_topology`) before requesting verbose OTLP details for specific spans.
