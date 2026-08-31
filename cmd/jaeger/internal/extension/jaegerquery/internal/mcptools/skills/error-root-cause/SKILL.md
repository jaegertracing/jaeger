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
   span with no errored children is the most likely root cause.
4. Use `get_span_details` on the candidate root-cause span(s) to inspect
   attributes, events, and status for the actual failure reason.
5. Report: root-cause span (service, operation, error message), the
   propagation chain, and a recommendation.

## Gotchas

- Timeouts at a parent may mask the real cause in a child that was cancelled —
  check children of timed-out spans even if they do not have error status.
- Multiple independent root causes can exist in a single trace.
