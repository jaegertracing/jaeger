# mcptools

This package serves Jaeger's telemetry tools over MCP, `read_skill` among them.
Operating the endpoint, and serving an installation's own skills through
`ai.mcp.skills_dir`, are documented on the website, starting at
[MCP Server](https://www.jaegertracing.io/docs/latest/ai/mcp-server/).

The `skills/` directory beside this file holds the skills compiled into the
Jaeger binary. A built-in skill follows the same guide as an operator's skill,
[Custom Skills](https://www.jaegertracing.io/docs/latest/ai/skills/) on the
website, with one difference: the root `SKILL.md` links to it without the
`custom/` prefix, for example `detect-n-plus-one/SKILL.md`.

The operator's directory is opened with `os.OpenRoot`, so `..` traversal and
symlinks that leave it are refused by the OS rather than by a path check. Jaeger
serving a skill and an agent choosing to read it are separate questions; see
[#9336](https://github.com/jaegertracing/jaeger/issues/9336) for what is known
about the second.

## Tuning the tool limits

Three tools bound how much one call may ask for. Each has a default, and each
can be raised under the same `ai.mcp` block:

```yaml
extensions:
  jaeger_query:
    ai:
      mcp:
        max_search_results: 500          # default 100, at most 1000
        max_span_details_per_request: 50 # default 20,  at most 100
        max_read_file_size: 1048576      # default 512 KiB, at most 10 MiB
```

| Setting | Bounds | Tools it limits |
| --- | --- | --- |
| `max_search_results` | returned spans | `search_traces` |
| `max_span_details_per_request` | spans per call | `get_span_details`, `get_trace_errors`, `get_trace_topology` |
| `max_read_file_size` | bytes served | `read_skill` |

`server_name` (default `jaeger`) sets the name the server advertises to clients.

Leave a setting out, or set it to `0`, to keep its default. A value outside the
range above is rejected at startup rather than being silently clamped, so a
limit an operator raised deliberately is the limit that takes effect.

The bounds are per call, not per session: they stop one request from allocating
without limit, and do not cap what a client can ask for across many requests.
