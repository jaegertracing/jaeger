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

Point `ai.skills_dir` at a directory on the query server's disk:

```yaml
extensions:
  jaeger_query:
    ai:
      enable_mcp: true          # required — skills are served over MCP
      skills_dir: /etc/jaeger/skills
```

The all-in-one config is not externally configurable, but the field can still be
set on it with `--set extensions.jaeger_query.ai.skills_dir={path}`.

Its contents are served under `custom/`, beside the built-in skills:

```
SKILL.md                       # built-in entry point
detect-n-plus-one/SKILL.md     # built-in skill
error-root-cause/SKILL.md      # built-in skill
custom/SKILL.md                # your entry point, mounts <skills_dir>/SKILL.md
custom/your-skill/SKILL.md     # your skill      , mounts <skills_dir>/your-skill/SKILL.md
```

`<skills_dir>/SKILL.md` is required: an agent reads it first, and without it
nothing below is reachable. It is an ordinary skill — if your instructions are
short it can be all you write. Anything longer belongs in a sub-skill, one
directory per skill, that the entry point links to and the agent opens only when
the description matches the task at hand.

Write those links relative to the **skills root**, not to the linking file's own
directory — an agent passes the link text straight back to `read_skill`:

```markdown
- [your-skill](custom/your-skill/SKILL.md) — one line on when to use it.
```

Every `SKILL.md` opens with YAML frontmatter following the
[agent skills specification](https://agentskills.io/specification):

```markdown
---
name: your-skill
description: >-
  When to use this skill. Agents read this to decide whether to open the
  skill at all, so say when it applies, not just what it does.
---

# Your Skill

## When this applies
...

## Procedure
1. ...
```

A `skills_dir` that cannot be opened, or whose `SKILL.md` cannot be read, is
broken configuration, and Jaeger refuses to start rather than quietly serving an
incomplete skill set.
