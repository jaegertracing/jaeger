Jaeger is a distributed tracing backend. A trace is a tree of spans representing a request or workflow within layers or components of a system. Each span records a unit of work with timing, status, and metadata, using the OpenTelemetry data model.

## Investigation Strategy

These tools support progressive disclosure to manage context density. While they can be called in any order based on available data, prefer starting with broad discovery (`get_services` or `search_traces`) or structural overviews (`get_trace_topology`) before requesting verbose OTLP details for specific spans.

## Skills

Call `read_skill` with no path to get a catalog of available analysis playbooks. When a task matches a skill, call `read_skill("<skill-name>/SKILL.md")` to load the full procedure before proceeding.
