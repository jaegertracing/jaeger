# Skills

A *skill* is a markdown playbook an AI agent reads before attempting a task —
how to detect an N+1 query pattern, how to walk a failed trace to its root
cause. Agents reach them through the `read_skill` MCP tool, starting at the root
`SKILL.md` catalog and following its links, so only the catalog is read up front
and an individual skill only when it looks relevant.

The `skills/` directory beside this file holds the skills compiled into the
Jaeger binary. Operators can add their own without rebuilding, via
`ai.skills_dir`.

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
SKILL.md                       # built-in root catalog
detect-n-plus-one/SKILL.md     # built-in skill
error-root-cause/SKILL.md      # built-in skill
custom/SKILL.md                # your catalog       ← <skills_dir>/SKILL.md
custom/your-skill/SKILL.md     # your skill         ← <skills_dir>/your-skill/SKILL.md
```

So `skills_dir` wants a root `SKILL.md` cataloguing your skills, plus one
directory per skill, each with its own `SKILL.md`.

Write catalog links relative to the **skills root**, not to the catalog's own
directory — an agent passes the link text straight back to `read_skill`:

```markdown
- [your-skill](custom/your-skill/SKILL.md) — one line on when to use it.
```

Each skill's `SKILL.md` opens with YAML frontmatter following the
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

Your root `custom/SKILL.md` is a hand-written catalog rather than a skill, so it
needs no frontmatter.

A `skills_dir` that cannot be opened or listed is broken configuration, and
Jaeger refuses to start rather than quietly serving an incomplete skill set.
