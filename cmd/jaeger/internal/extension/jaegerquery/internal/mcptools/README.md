# mcptools

This package serves Jaeger's telemetry tools over MCP, `read_skill` among them.

A *skill* is a markdown playbook an AI agent reads before attempting a task —
how to detect an N+1 query pattern, how to walk a failed trace to its root
cause. Agents reach them through `read_skill`, starting at the root `SKILL.md`
and following its links, so a skill is only read when it looks relevant.

The `skills/` directory beside this file holds the skills compiled into the
Jaeger binary. For writing a skill, see
[Custom Skills](https://www.jaegertracing.io/docs/latest/ai/skills/) on the
website.

Operating the endpoint is documented on the website, in
[MCP Server](https://www.jaegertracing.io/docs/latest/ai/mcp-server/): enabling
and disabling it, the tool catalog, connecting a client, exposure and
authentication, tenants and sessions, the Streamable HTTP transport options, and
serving installation-specific skills from `ai.mcp.skills_dir`. This file keeps
the developer-level detail of the skills mount: how the operator's directory is
mounted, the exact startup errors, and how the limits are enforced.

## How `skills_dir` is mounted

The directory named by `ai.mcp.skills_dir` is served under the `custom/` prefix
beside the built-in skills:

```
SKILL.md                       # built-in entry point
detect-n-plus-one/SKILL.md     # built-in skill
error-root-cause/SKILL.md      # built-in skill
custom/SKILL.md                # operator entry point, mounts <skills_dir>/SKILL.md
custom/your-skill/SKILL.md     # operator skill      , mounts <skills_dir>/your-skill/SKILL.md
```

`custom/` is a prefix on the path an agent asks for, not a directory the
operator creates. The built-in root `SKILL.md` links to `custom/SKILL.md`, so an
agent that follows the index reaches the operator's skills, and a `read_skill`
for any `custom/…` path answers `cannot read "<path>": …` when no `skills_dir`
is configured.

The directory is read live, so an edit takes effect on the next `read_skill`
call. Startup validation is the exception: it runs once, so a directory that
breaks after the server started is reported when an agent reads it rather than
by refusing to serve.

## What Jaeger checks, and what it limits

A `skills_dir` that cannot be opened, or whose `SKILL.md` cannot be read, is
broken configuration, and Jaeger refuses to start rather than quietly serving an
incomplete skill set:

| Startup error | Cause |
| --- | --- |
| `cannot open skills_dir "…"` | Path is missing, or is not a directory |
| `cannot read SKILL.md in skills_dir "…"` | Directory opens, but has no readable entry point |

Two limits apply to what is served:

- **Path containment.** The directory is opened with `os.OpenRoot`, so `..`
  traversal and symlinks pointing outside `skills_dir` are refused by the OS
  rather than by a path check that could be tricked.
- **File size.** A served file is capped at 512 KiB; beyond that the reply is cut
  and ends with `file content truncated after 524288 bytes`. The cap is fixed, not
  a config field — a skill anywhere near it is far too long to be useful to an
  agent.

Jaeger serving a skill and an agent choosing to read it are separate questions;
see [Custom Skills](https://www.jaegertracing.io/docs/latest/ai/skills/), and
[#9336](https://github.com/jaegertracing/jaeger/issues/9336) for what is known
about the second.
