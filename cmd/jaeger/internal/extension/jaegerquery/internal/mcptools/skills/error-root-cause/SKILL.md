---
name: error-root-cause
description: >-
  Walk a failed trace to the first originating error span and distinguish root
  cause from cascading failures. Use when a trace has errors and the user asks
  why it failed, what caused the errors, or which service is the root cause.
license: Apache-2.0
metadata:
  author: jaegertracing
  version: "1.0"
allowed-tools: get_trace_errors get_trace_topology get_span_details
---

# Error Root Cause Analysis

## When this applies

A trace contains one or more error spans and the user wants to know the
originating failure, not just the symptoms.

## Procedure

1. Use `get_trace_errors` to list all error spans with their status messages.
2. Use `get_trace_topology` to see the full span tree and identify
   parent-child relationships among the error spans.
3. Walk from each error span toward the leaves of the tree. The deepest error
   span with no errored children is the *candidate* root cause.
4. Before accepting that candidate, look at its children even though none of
   them errored. If one ran for nearly the whole of the candidate's duration,
   the candidate did not fail — it gave up waiting, and the cause lies in that
   child. Descend into it and repeat, until no child accounts for the time.
5. Use `get_span_details` on the span you settled on to inspect attributes,
   events, and status for the actual failure reason.
6. Report: the span the failure originates from (service, operation, and error
   message if it has one), the propagation chain, and a recommendation.

## Gotchas

- A timed-out call often has nothing below it. The work it was waiting on never
  returned, so no spans were recorded for it. Name the last span you can see and
  say the cause lies beneath it, rather than blaming the timeout itself.
- The span a trace blames and the span that failed are frequently in different
  services. Report where the failure originates, not where it surfaced.
- Multiple independent root causes can exist in a single trace.
