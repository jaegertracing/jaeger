---
name: jaeger-skills
description: Root catalog of Jaeger trace analysis skills.
---

# Jaeger Analysis Skills

Skills are organized using progressive disclosure.
Read a sub-skill's SKILL.md before applying it.

## Available Skills

- [detect-n-plus-one](detect-n-plus-one/SKILL.md) — Detect N+1 query patterns where one
  parent operation triggers many near-identical child spans. Use when traces show repeated
  downstream calls or the user asks about chatty DB access.

- [error-root-cause](error-root-cause/SKILL.md) — Walk a failed trace to the first
  originating error span. Use when a request failed and the user wants to know where.

- [jaeger-sql](jaeger-sql/SKILL.md) — Write the sql input of search_traces. Use when the
  user writes the search as SQL, or when the structured inputs cannot say it: OR, NOT, a
  comparison on an attribute, a text pattern, or a condition on a span's events or links.

Installation-specific skills, if any, are listed under
[custom/SKILL.md](custom/SKILL.md). If that file does not exist, this Jaeger
instance has none.
