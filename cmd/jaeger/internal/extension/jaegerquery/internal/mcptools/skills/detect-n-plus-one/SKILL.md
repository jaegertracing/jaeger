---
name: detect-n-plus-one
description: >-
  Detect N+1 query patterns in a trace, where one parent operation triggers many
  near-identical child spans (often database calls). Use when a trace is slow
  and shows repeated downstream calls, or when the user asks about N+1,
  repeated queries, or chatty DB access.
license: Apache-2.0
metadata:
  author: jaegertracing
  version: "1.0"
allowed-tools: search_traces get_trace_topology get_span_details
---

# Detect N+1 Query Patterns

## When this applies

A parent span has many child spans with the same operation name and similar
duration, typically database queries or RPC calls.

## Procedure

1. Find candidate traces with `search_traces`.
2. Pull the span tree with `get_trace_topology`; group child spans by operation
   name under each parent.
3. Flag any group with more than 10 near-identical siblings as a potential N+1
   pattern. Check that children have similar durations (within 2x of the median).
4. Inspect repeated siblings with `get_span_details` to confirm they target the
   same downstream service and carry similar attributes.
5. Report: the parent span (service, operation), the repeated child operation,
   the count, total wall-clock time consumed, and whether children run
   sequentially or in parallel.

## Gotchas

- Parallel fan-out can look like N+1 but is intentional — check whether
  children overlap in time before flagging.
- Batch operations may share an operation name but carry different payloads —
  check span attributes to distinguish.
