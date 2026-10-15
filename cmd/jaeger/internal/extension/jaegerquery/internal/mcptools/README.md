# mcptools

This package serves Jaeger's telemetry tools over MCP, `read_skill` among them.

A *skill* is a markdown playbook an AI agent reads before attempting a task —
how to detect an N+1 query pattern, how to walk a failed trace to its root
cause. Agents reach them through `read_skill`, starting at the root `SKILL.md`
and following its links, so a skill is only read when it looks relevant.

The `skills/` directory beside this file holds the skills compiled into the
Jaeger binary. Operators can add their own without rebuilding, via
`ai.mcp.skills_dir` — the rest of this file is for them. For writing the skills
themselves, see [AUTHORING.md](./AUTHORING.md).

## Turning the MCP endpoint on

The `ai.mcp` block enables the endpoint; an empty block is enough:

```yaml
extensions:
  jaeger_query:
    ai:
      mcp: {}
```

It serves at `<basePath>/api/ai/mcp/` on the query port, carrying the telemetry
tools and `read_skill` with the built-in skills. Point an MCP client there, or
leave it to an AI chat sidecar configured with `ai.agent_url`.

## Running behind a reverse proxy or sidecar

The MCP SDK that serves this endpoint carries a DNS-rebinding guard for MCP servers on a
developer's machine: a request that arrives on a loopback address with a non-loopback `Host`
header is rejected with `403 Forbidden: invalid Host header`. A reverse proxy or service-mesh
sidecar that dials the query port over `127.0.0.1` and preserves the client's `Host` (an Envoy
sidecar without host rewriting, for example) produces that shape for every request, so behind one
the endpoint rejects every caller with that body. The guard never fires when the proxy rewrites
`Host` to the upstream address or dials a non-loopback address, so change the proxy where that is
possible. Where it is not, turn the guard off:

```yaml
extensions:
  jaeger_query:
    ai:
      mcp:
        streamable_http:
          disable_localhost_protection: true
```

The UI and the REST API on the same port have no such guard, so this setting changes nothing
about them. Keep the default when clients reach the query port directly, by a loopback address or
a routable host name; only a loopback connection carrying some other name is affected.

## Running several replicas behind a load balancer

By default the endpoint keeps one MCP session per client in the memory of the `jaeger-query`
process that handled `initialize`, and the SDK looks that session up by the `Mcp-Session-Id`
header on every later request. When a load balancer without session affinity spreads a client's
requests over several replicas, a request that reaches a replica other than the one that holds the
session is answered with `404 session not found`; and when the client's `notifications/initialized`
is the request that went astray, the replica that holds the session rejects the next `tools/list`
with `method "tools/list" is invalid during session initialization`. Set `streamable_http.stateless` to make every
request stand on its own:

```yaml
extensions:
  jaeger_query:
    ai:
      mcp:
        streamable_http:
          stateless: true
```

Jaeger's tools keep no state between requests, so they behave the same. A stateless endpoint
answers 405 to the GET that a client sends to open a stream for server-initiated messages. Jaeger
never sends any, so in the default mode that stream only sits idle until the session times out;
the MCP specification allows a server to offer no stream and requires the client to continue
without it.

This covers external MCP clients on the shared `/api/ai/mcp/` endpoint. The chat gateway's
turn-scoped endpoint, `/api/ai/mcp/<id>/`, is different: the turn it serves is held in the memory
of the process that is streaming that chat to the browser, so a request for it that lands on
another replica is answered 404 before the MCP handler is reached, stateless or not. An agent that
is not co-located with `jaeger-query` must reach the replica that owns the turn, which means
`ai.mcp.base_url` has to name that process (its pod address, not a load-balanced service name) or
the load balancer has to route by the `<id>` path segment.

## Answering with plain JSON instead of an event stream

By default the endpoint answers every POST with a `text/event-stream` body, even when that body
holds a single result, because the stream form lets a server send progress or its own requests
before the result. Jaeger's tools send none, so the stream carries the one result and nothing
else. Set `streamable_http.json_response` to answer a single-message POST with a plain
`application/json` body, which is easier for proxies, logging middleware and command-line clients
to read:

```yaml
extensions:
  jaeger_query:
    ai:
      mcp:
        streamable_http:
          json_response: true
```

MCP clients must accept both forms, so the setting changes nothing for them. A JSON-RPC batch is
still answered with a stream.

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

`custom/` is a prefix on the path an agent asks for, not a directory you create:
inside `skills_dir`, your entry point is plain `SKILL.md`.

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

## Editing skills on a running server

`skills_dir` is read live. Editing a skill's text, or adding a file the index
links to, takes effect on the next `read_skill` call — no restart, no reload
signal. Startup validation is the exception: it runs once, so a `skills_dir`
that breaks after the server started is reported when an agent reads it rather
than by refusing to serve.

Skills are static text, and Jaeger never executes them. What reads them is an
agent, which then decides what to do.

## What Jaeger checks, and what it limits

A `skills_dir` that cannot be opened, or whose `SKILL.md` cannot be read, is
broken configuration, and Jaeger refuses to start rather than quietly serving an
incomplete skill set:

| Startup error | Cause |
| --- | --- |
| `cannot open skills_dir "…"` | Path is missing, or is not a directory |
| `cannot read SKILL.md in skills_dir "…"` | Directory opens, but has no readable entry point |

At read time a bad path is reported back to the agent as
`cannot read "<path>": …` — which is also what every `custom/…` path returns
when no `skills_dir` is configured.

Two limits apply to what is served:

- **Path containment.** The directory is opened with `os.OpenRoot`, so `..`
  traversal and symlinks pointing outside `skills_dir` are refused by the OS
  rather than by a path check that could be tricked.
- **File size.** A served file is capped at 512 KiB; beyond that the reply is cut
  and ends with `file content truncated after 524288 bytes`. The cap is fixed, not
  a config field — a skill anywhere near it is far too long to be useful to an
  agent.

## Who can write to skills_dir

A skill is instructions an agent follows, with your telemetry tools already in
hand. Anyone who can write to `skills_dir` can steer that agent's behaviour
without touching Jaeger's binary or configuration. Treat the directory as part
of the trusted configuration surface: own it by root or the Jaeger service
account, keep it out of paths writable by application deployments, and review
changes to it as you would a config change.

## Checking it works

With an MCP client attached to the endpoint, ask for your entry point directly:

```
read_skill(path="custom/SKILL.md")
```

Your text coming back means the directory is mounted and reachable.
`cannot read "custom/SKILL.md"` means either no `skills_dir` is configured on
this server, or the file is not there under that name.

Note that Jaeger serving a skill and an agent choosing to read it are separate
questions — see [AUTHORING.md](./AUTHORING.md), and
[#9336](https://github.com/jaegertracing/jaeger/issues/9336) for what is known
about the second.
