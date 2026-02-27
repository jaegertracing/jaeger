# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records (ADRs) for the Jaeger project. ADRs document important architectural decisions made during the development of Jaeger, including the context, decision, and consequences of each choice.

## What is an ADR?

An Architecture Decision Record (ADR) is a document that captures an important architectural decision made along with its context and consequences. ADRs help teams understand why certain decisions were made and provide historical context for future contributors.

## ADRs in This Repository

- [ADR-001: Cassandra FindTraceIDs Duration Query Behavior](001-cassandra-find-traces-duration.md) - Explains why duration queries in the Cassandra spanstore use a separate code path and cannot be efficiently combined with other query parameters.
- [ADR-002: MCP Server Extension](002-mcp-server.md) - Design for implementing Model Context Protocol server as a Jaeger extension for LLM integration.
- [ADR-003: Lazy Storage Factory Initialization](003-lazy-storage-factory-initialization.md) - Comparative analysis of approaches to defer storage backend initialization until actually needed.
- [ADR-004: Configuration Schema Generation](004-configuration-schema-generation.md) - Strategy and rationale for generating JSON schemas for Jaeger V2 configuration components using schemagen.

