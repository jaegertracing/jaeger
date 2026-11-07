Jaeger is a distributed tracing backend. A trace is a tree of spans representing a request or workflow within layers or components of a system. Each span records a unit of work with timing, status, and metadata, using the OpenTelemetry data model.

## Investigation Strategy

These tools support progressive disclosure to manage context density. While they can be called in any order based on available data, prefer starting with `read_skill` (path `SKILL.md`) to choose an analysis approach, then broad discovery (`get_services`, `find_traces` or `find_spans`) or structural overviews (`get_trace_topology`) before requesting verbose OTLP details for specific spans.

`find_traces` takes either its structured inputs or one SQL statement in `sql`, and `find_spans` takes only a SQL statement; read the `jaeger-sql/SKILL.md` skill before writing one. `find_spans` returns full span data for every match, so bound it with `LIMIT` or `max_results`.

## Skills

Skills are organized using progressive disclosure. Call `read_skill` with path `SKILL.md` to get the root skill which will guide you to more specific sub-skills.
