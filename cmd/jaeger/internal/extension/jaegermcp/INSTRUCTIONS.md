Jaeger is a distributed tracing backend. A trace is a tree of spans representing a request or workflow within layers or components of a system. Each span records a unit of work with timing, status, and metadata, using the OpenTelemetry data model.

## Investigation Strategy

These tools support progressive disclosure to manage context density.

Recommended workflow:
1. Use `get_services` and `get_span_names` to discover valid names before filtering.
2. Use `search_traces` to identify candidate traces.
3. Before requesting verbose OTLP payloads, use `get_trace_topology` or `get_critical_path` to narrow the investigation.
4. Call `get_span_details` only for specific suspicious span IDs discovered from topology or critical path output.
5. Use `get_trace_errors` when you specifically need error spans. Compare `total_error_count` with the number of returned spans because responses may be truncated by the server limit.

Avoid calling `get_span_details` for every span in a large trace. On wide or error-heavy traces, full span details can consume most of the context window before any reasoning begins.
