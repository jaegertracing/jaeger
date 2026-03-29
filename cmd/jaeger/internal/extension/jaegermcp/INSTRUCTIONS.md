Jaeger is a distributed tracing backend. A trace is a tree of spans
representing a single request flowing through multiple services. Each
span records one unit of work with timing, status, and attributes.

## Progressive Disclosure

These tools are designed for a drill-down workflow — start broad, then
narrow. Always call `get_services` before `search_traces` to discover
valid service names. Use `get_trace_topology` to understand trace
structure before calling `get_span_details` on specific spans.

## System Limits

- `search_traces` and `get_span_details` enforce server-configured
  limits on the number of results and span IDs per request.
- `get_trace_errors` reports the true total `error_count` even when
  the detailed spans list is truncated to the per-request limit.
