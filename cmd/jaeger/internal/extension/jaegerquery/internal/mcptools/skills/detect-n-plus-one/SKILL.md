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
   name under each parent. Operation name alone is not enough to make a group:
   spans sharing a name may call different endpoints, and one unrelated call
   left in the group distorts every measurement in step 3. Confirm with
   `get_span_details` that a group targets one downstream operation, and split
   it where it does not.
3. A group is a candidate only if it is both repeated and serial. Take the
   group's elapsed window — earliest start to latest end — and compare it with
   the sum of the siblings' own durations. Roughly equal means each call waited
   for the one before it, which is the N+1. A sum larger than the window means
   the calls overlapped, and no number of them makes an N+1.
4. Apply the count last, to groups that survived step 3: more than 10 serial
   siblings is worth reporting.
5. Look at what varies between the siblings you kept. An N+1 repeats one call
   with a single changing parameter — a row id, a driver id — while everything
   else stays fixed. If the calls vary in more than that, they may be distinct
   work that happens to share a name.
6. Report: the parent span (service, operation), the repeated child operation,
   the count, the elapsed window and the summed duration that established the
   calls were serial.

## Gotchas

- Do not decide from duration similarity. A real N+1 containing one retry is
  less uniform than a fan-out of evenly-sized parallel calls, so the more
  uniform group is often the one that is *not* an N+1.
- Overlap is frequently partial: a worker pool runs a few calls at a time, so
  only some siblings overlap. Any excess of summed duration over the elapsed
  window means concurrency, and the group is a fan-out.
- Batch operations may share an operation name but carry different payloads —
  check span attributes to distinguish.
