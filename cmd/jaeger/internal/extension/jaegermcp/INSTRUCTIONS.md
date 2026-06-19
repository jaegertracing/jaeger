Jaeger is a distributed tracing backend. A trace is a tree of spans representing a request or workflow within layers or components of a system. Each span records a unit of work with timing, status, and metadata, using the OpenTelemetry data model.

## Skills

At the start of any session, read the skills index to discover available capabilities:

    fs_read_text_file  path=/skills/skills-index/SKILL.md

The index lists all available skills and when to use each one. When a task matches a
skill description, read that skill's file:

    fs/read_text_file  path=/skills/<name>/SKILL.md

Example workflow:
1. Read `/skills/skills-index/SKILL.md` → discover which skills exist
2. Read `/skills/greet-user/SKILL.md` → load the full instructions for a matching skill

## Investigation Strategy

These tools support progressive disclosure to manage context density. While they can be called in any order based on available data, prefer starting with broad discovery (`get_services` or `search_traces`) or structural overviews (`get_trace_topology`) before requesting verbose OTLP details for specific spans.
