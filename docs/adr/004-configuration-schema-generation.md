# Architecture Decision Record: Configuration Schema Generation

## Status
Proposed

## Context
The goal for Jaeger V2 configuration is to provide **accurate, auto-generated, and functionally rich documentation** on [jaegertracing.io](https://www.jaegertracing.io/). 

Currently, our configuration is "Code-First": documentation is manually maintained, leading to drift, and we lack machine-readable definitions for validation or IDE support. While we briefly explored using `schemagen` (Go reflection to YAML), it was identified as an implementation detail that doesn't fully achieve the documentation goal—especially regarding "foreign references" to OpenTelemetry types.

This ADR proposes a **Schema-First** approach, aligning with [Issue 6186](https://github.com/jaegertracing/jaeger/issues/6186) and the long-term roadmap of the OpenTelemetry Collector.

### Bootstrapping (Phase 1)
We have already performed initial bootstrapping using `schemagen` (Go reflection to YAML) in PR #7947. This allowed us to quickly generate the initial inventory of schemas for internal extensions. Moving forward, these schemas will become the "source of truth," and we will pivot to the workflow described below.

## Decision

The goal is to generate documentation that accurately reflects the configuration schema expected by the **Jaeger binary**. To achieve this, we will move beyond Go-reflection-based generation and implement a **Schema-First** workflow where the schema is the primary Source of Truth for code, validation, and documentation.

### 1. The Workflow: Schema-First
We will adopt the following lifecycle for all Jaeger-specific configuration:

1.  **Source Schema**: Configuration is defined in JSON Schema (Draft 2020-12) or YAML schema files within the component directory.
2.  **Code & Validation Generation**: We use code generation (e.g., `go-jsonschema`) to produce Go structs **and** their associated validation logic (struct tags or `Validate()` methods). This ensures the Jaeger binary enforces exactly what is defined in the schema.
3.  **Documentation Generation**: The same schema is consumed by the documentation pipeline to produce Markdown for jaegertracing.io.

### 2. Handling Foreign References ($ref)
User-facing documentation must not contain "opaque" references to external repositories. To ensure the Jaeger binary's documentation is self-contained:
*   **Logical Pointers**: Source schemas use `$ref` to upstream OTel types (e.g., `confighttp.ServerConfig`) to ensure upstream compatibility.
*   **The Dereferencer**: Our build process will include a **Schema Dereferencer**. For the purpose of **Documentation and Binary Validation**, this tool will fetch and inline the fields of referenced OTel types.
*   **Complete Visibility**: This allows the documentation generator to show users the full set of options (e.g., `endpoint`, `tls`, `timeout`) on a single page, even if they are defined in the OTel Collector.

### 3. Functional Richness
Schemas must go beyond simple type/description to enable robust config authoring and runtime validation. We will utilize JSON Schema's full constraint set:
*   `pattern`: For regex-based string validation (e.g., hostnames, durations).
*   `minimum`/`maximum`: For numeric constraints.
*   `enum`: For restricted string options.
*   `default`: To provide the "sane defaults" that the Jaeger binary will use.

### 4. Shared Mechanics (Storage Backends)
For shared components like storage backends, we will maintain a library of "schema fragments." This ensures that the configuration for "Elasticsearch" is identical and documented consistently whether used in the Query service or the Storage extension.

### 5. Integration & Enforcement
*   **CI Check**: `make check-generate` will verify that Go structs and documentation are strictly derived from the source schemas.
*   **jaeger print-config**: This command will export the binary's expected schema, validated against the source-of-truth JSON schemas to ensure absolute parity.
