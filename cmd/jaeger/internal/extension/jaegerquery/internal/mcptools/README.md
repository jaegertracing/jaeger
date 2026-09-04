# mcptools

This package serves Jaeger's telemetry tools over MCP, `read_skill` among them.

A *skill* is a markdown playbook an AI agent reads before attempting a task —
how to detect an N+1 query pattern, how to walk a failed trace to its root
cause. Agents reach them through `read_skill`, starting at the root `SKILL.md`
and following its links, so a skill is only read when it looks relevant.

The `skills/` directory beside this file holds the skills compiled into the
Jaeger binary. Operators can add their own without rebuilding, via
`ai.skills_dir` — the rest of this file is for them.

## Adding installation-specific skills

Point `ai.mcp.skills_dir` at a directory on the query server's disk:

```yaml
extensions:
  jaeger_query:
    ai:
      mcp:                      # skills served over MCP
        skills_dir: /etc/jaeger/skills
```

The all-in-one config is not externally configurable, but the field can still be
set on it with `--set extensions.jaeger_query.ai.mcp.skills_dir={path}`.

Its contents are served under `custom/`, beside the built-in skills:

```
SKILL.md                       # built-in entry point
detect-n-plus-one/SKILL.md     # built-in skill
error-root-cause/SKILL.md      # built-in skill
custom/SKILL.md                # your entry point, mounts <skills_dir>/SKILL.md
custom/your-skill/SKILL.md     # your skill      , mounts <skills_dir>/your-skill/SKILL.md
```

`<skills_dir>/SKILL.md` is required: an agent reads it first, and without it
nothing below is reachable. It is also your index. An agent chooses what to read
next from the links it finds there and from nothing else, so the line you write
beside each link is what decides whether that skill is ever opened. If your
instructions are short, the entry point can be all you write. Anything longer
belongs in a file of its own that the index links to.

Write those links relative to the **skills root**, not to the linking file's own
directory — an agent passes the link text straight back to `read_skill`:

```markdown
- [your-skill](custom/your-skill/SKILL.md) — one line on when to use it.
```

A link may point at any file, under any name; `SKILL.md` is required only at the
root. The names above are a convention, not a rule.

A skill is markdown. Jaeger serves it as-is and parses nothing inside it:

```markdown
---
name: your-skill
description: One line on what this skill does.
---

# Your Skill

## When this applies
...

## Procedure
1. ...
```

Frontmatter like the block above is optional. It travels well — both the
[Open Knowledge Format](https://cloud.google.com/blog/products/data-analytics/how-the-open-knowledge-format-can-improve-data-sharing)
and the [agent skills specification](https://agentskills.io/specification) use
one, so a skill written for either stays readable here — but Jaeger reads no
field of it, and a `description` there is no substitute for the line beside the
link in your index.

A `skills_dir` that cannot be opened, or whose `SKILL.md` cannot be read, is
broken configuration, and Jaeger refuses to start rather than quietly serving an
incomplete skill set.

## Tuning tool response limits

The tools cap their own responses so a single call cannot flood the agent's
context window. The caps are the same ones the retired `jaeger_mcp` extension
exposed, and each can be overridden:

```yaml
extensions:
  jaeger_query:
    ai:
      mcp:
        max_span_details_per_request: 20     # spans per get_span_details / get_trace_errors / get_trace_topology
        max_search_results: 100              # ceiling on search_traces' search_depth
        max_read_file_size: 524288           # bytes, per read_skill response
```

Omitting a field, or setting it to `0`, keeps the built-in default shown above.
Negative values are rejected at startup — they would disable the tool they bound
rather than restrict it.

Lower `max_span_details_per_request` when running a model with a smaller context
window: it is the dominant lever on how much raw JSON a single call returns. Note
that it bounds three tools at once, so changing it moves all of them together.
