# Jaeger MCP Server — Instructions for LLM Clients

You have access to a Jaeger distributed tracing backend via MCP tools.
Jaeger stores traces from microservice architectures — each trace is a
tree of spans representing a single request flowing through multiple services.

## Investigation workflow

Follow this drill-down sequence. Do NOT skip to `get_span_details` early —
full span payloads are ~6x larger than topology spans and will exhaust your
context window on large traces.

1. **Discover services** — call `get_services` to learn what services exist.
2. **Search traces** — call `search_traces` with a service name and time range
   to find traces matching your criteria. Results are lightweight summaries
   (trace ID, duration, span count, error flag) — not full trace data.
3. **Map the trace** — call `get_trace_topology` on an interesting trace ID.
   This returns the structural skeleton: parent-child relationships, timing,
   and error status for every span, without attributes or logs. Use the `path`
   field (slash-delimited span IDs like `rootID/parentID/spanID`) to understand
   the call tree.
4. **Find errors** — if the trace has errors, call `get_trace_errors` to get
   full details for error spans only. `error_count` reflects the true total
   even when the response is truncated.
5. **Analyze latency** — call `get_critical_path` to identify the sequence of
   spans that form the blocking execution path. These are the spans whose
   duration directly contributed to end-to-end latency.
6. **Inspect specific spans** — only now call `get_span_details` with the
   specific span IDs you identified from steps 3-5. Request only the spans
   you actually need to examine.

## Tool reference

| Tool | Purpose | When to use |
|------|---------|-------------|
| `health` | Check server status | Before starting an investigation |
| `get_services` | List service names | First step — discover what's available |
| `get_span_names` | List span/operation names for a service | When filtering by operation |
| `search_traces` | Find traces by service, time, duration, attributes | Finding candidate traces to investigate |
| `get_trace_topology` | Structural overview of a trace (no attributes) | Understanding call flow before drilling in |
| `get_trace_errors` | Error spans with full details | Diagnosing failures |
| `get_critical_path` | Latency-critical span sequence | Diagnosing slowness |
| `get_span_details` | Full OTLP span data for specific span IDs | Final step — inspecting specific spans |

## Common investigation patterns

**"Why is this service slow?"**
→ `get_services` → `search_traces` (with `duration_min`) →
`get_trace_topology` → `get_critical_path` → `get_span_details` on
critical path spans

**"What errors are happening?"**
→ `get_services` → `search_traces` (with `with_errors: true`) →
`get_trace_errors` → `get_span_details` on error spans if more context needed

**"Show me the architecture / service dependencies"**
→ `get_services` → `search_traces` → `get_trace_topology` on a
representative trace

## Important constraints

- `search_traces` requires `service_name`. Call `get_services` first if you
  do not know the service name.
- Time parameters accept RFC 3339 (`2024-01-15T10:30:00Z`) or relative
  formats (`-1h`, `-30m`, `now`). Default lookback is 1 hour.
- `get_span_details` has a per-request span limit. Request only the span IDs
  you need — do not request all spans in a trace.
- `get_trace_topology` supports a `depth` parameter to limit tree depth.
  Use `depth: 2` or `depth: 3` on very large traces to get a high-level
  view before going deeper.
- `get_trace_errors` returns `error_count` reflecting all errors in the
  trace, even when the detailed spans list is truncated to the configured
  limit. Use `error_count` to gauge severity.
