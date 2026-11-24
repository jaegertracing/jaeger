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
