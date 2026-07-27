# Skills

A *skill* is a markdown playbook an AI agent reads before attempting a task —
how to detect an N+1 query pattern, how to walk a failed trace to its root
cause. Agents discover them through the `read_skill` MCP tool, starting at the
root `SKILL.md` catalog and following its links (progressive disclosure: only
the catalog is read up front, individual skills only when relevant).

This directory holds the skills compiled into the Jaeger binary. Operators can
add their own without rebuilding, via `ai.skills_dir`.

## Adding installation-specific skills

Point `ai.skills_dir` at a directory on the query server's disk:

```yaml
extensions:
  jaeger_query:
    ai:
      enable_mcp: true          # required — skills are served over MCP
      skills_dir: /etc/jaeger/skills
```

The all-in-one config is not externally configurable, but the field can still
be set on it with
`--set extensions.jaeger_query.ai.skills_dir={path}`.

Its contents are served under `custom/`, alongside the built-in skills:

```
SKILL.md                       # built-in root catalog
detect-n-plus-one/SKILL.md     # built-in skill
error-root-cause/SKILL.md      # built-in skill
custom/SKILL.md                # your catalog          ← <skills_dir>/SKILL.md
custom/your-skill/SKILL.md     # your skill            ← <skills_dir>/your-skill/SKILL.md
```

So `skills_dir` needs a root `SKILL.md` cataloguing your skills, plus one
directory per skill, each with its own `SKILL.md`. Skills nested deeper than
one level are served as plain files but are not treated as skills.

Write catalog links relative to the **skills root**, not to the catalog's own
directory — an agent passes the link text straight to `read_skill`:

```markdown
- [your-skill](custom/your-skill/SKILL.md) — one line on when to use it.
```

### Skill file format

Each skill's `SKILL.md` starts with YAML frontmatter following the
[agent skills specification](https://agentskills.io/specification):

```markdown
---
name: your-skill              # required; must equal the directory name.
                              # 1-64 chars, lowercase letters, digits, and
                              # single hyphens (no leading/trailing hyphen).
description: >-               # required, <=1024 chars. Agents read this to
  When to use this skill.     # decide whether to open the skill at all, so
                              # say when it applies, not just what it does.
license: Apache-2.0           # optional
metadata:                     # optional string -> string map
  author: your-org
compatibility: needs Jaeger v2  # optional, <=500 chars
allowed-tools: search_traces get_span_details   # optional; space-separated.
                              # Every name must be a tool this MCP server
                              # actually registers.
---

# Your Skill

## When this applies
...

## Procedure
1. ...
```

Unknown or misspelled keys are rejected. Your root `custom/SKILL.md` is a
hand-written catalog, not a skill, so it needs no frontmatter.

### If a skill is rejected

A skill whose frontmatter fails validation is skipped — Jaeger logs a warning
naming the file and the problem, serves every other skill normally, and starts
as usual. One typo cannot take down the query service. Check the startup logs
for `skipping invalid operator skill` if a skill you added is not showing up.

A `skills_dir` that cannot be opened or listed at all is different: that is
broken configuration, and Jaeger refuses to start rather than silently serving
an incomplete skill set.
