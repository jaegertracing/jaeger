---
name: analyze-critical-path
description: >-
  Triage latency bottlenecks by isolating execution time from scheduling or
  queue waits along the critical path. Use when a trace is slow and no spans
  show errors, or when the user asks what is slowing down a request.
license: Apache-2.0
metadata:
  author: jaegertracing
  version: "1.0"
allowed-tools: get_critical_path get_trace_topology get_span_details
---

# Analyze Critical Path

## When this applies

A trace is slower than expected without reporting errors, and the user wants to
know which operations account for the elapsed wall-clock duration.

## Procedure

1. Call `get_critical_path` with parameter `trace_id`. Identify the total trace
   duration (`total_duration_us`) and the critical path duration
   (`critical_path_duration_us`).
2. Rank the returned segments by descending `self_time_us`. Identify the dominant
   bottleneck segment (any segment exceeding 30% of `critical_path_duration_us`,
   or the top segment if duration is evenly distributed).
3. Call `get_trace_topology` with parameter `trace_id` to inspect the structural
   context of the bottleneck span:
   - Match the bottleneck `span_id` against the `path` fields in the topology list.
   - If no spans list this `span_id` in their ancestry path, classify it as a
     leaf execution bottleneck (such as a database query or CPU compute loop).
   - If other spans list this `span_id` as their parent in `path`, inspect the
     time offsets. If a large gap precedes or separates child calls, classify it
     as parent scheduling or un-instrumented processing wait.
4. Call `get_span_details` with parameters `trace_id` and `span_ids: ["<bottleneck_span_id>"]`
   to inspect attributes:
   - For database spans, inspect `db.statement` or `db.system`.
   - For RPC spans, inspect `rpc.service`, `rpc.method`, or `net.peer.name`.
   - For messaging spans, inspect `messaging.operation` and queue attributes.
5. Report:
   - The bottleneck span ID, service name, and operation name.
   - The self time in microseconds and its percentage of the critical path.
   - Classification: leaf execution bottleneck versus parent scheduling gap.
   - Concrete evidence from span attributes and recommended next diagnostic step.

## Gotchas

- Un-instrumented gaps: Gaps between sequential child calls are counted toward
  the parent span's self time. If a parent span has high self time and multiple
  children, verify whether a downstream dependency lacks tracing before blaming
  the parent logic.
- Parallel branches: Spans running concurrently off the critical path do not
  constrain total trace duration, even if their individual durations are high.
  Focus strictly on spans identified in the critical path segments.
- Asynchronous dispatch: Producer or consumer spans linked via follow-from
  references may record queue wait time that appears as latency; inspect
  messaging attributes to distinguish wait time from active processing.
