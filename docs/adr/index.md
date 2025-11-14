# Architecture Decision Records (ADRs)

This folder contains Architecture Decision Records (ADRs) for decisions that affect the design and maintenance of the Jaeger codebase.

An ADR documents:
- the context that led to a decision,
- the decision itself,
- and its consequences (for implementers and users).

The intent is to provide a durable record so future contributors understand why the code looks the way it does.

Current ADRs
- [0001 - Cassandra: duration queries for FindTraceIDs are handled as a separate path](cassandra-find-traces-duration.md) - explains why the Cassandra spanstore handles duration queries via the duration_index and treats them differently from other indices (tags, generic inverted indices).
