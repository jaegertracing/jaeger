# How to Author Custom AI Skills for Jaeger

This guide explains how to write, validate, and deploy your own **Jaeger AI
Skills** — declarative configuration files that teach the Jaeger AI assistant
new debugging workflows without recompiling the binary.

---

## What Is a Skill?

A **Skill** is a YAML file that defines:

| Field | Purpose |
|---|---|
| `name` | Unique identifier (kebab-case, e.g. `analyze-critical-path`) |
| `description` | One-sentence summary shown in the UI skill picker |
| `system_prompt` | Instructions injected before the user's query |
| `allowed_tools` | The MCP tools the agent may call for this skill |
| `output_format` | Expected output shape (`markdown`, `json`, `text`) |
| `constraints` | Additional restrictions on agent behavior |
| `examples` | Few-shot demonstrations (recommended) |

**Key design principle:** Skills *compose and constrain* existing MCP tools.
They do NOT register new tool behavior at runtime. New tool capabilities must
be implemented in the Jaeger MCP extension code and reviewed by maintainers
before skills can reference them.

---

## Skill File Format

```yaml
name: my-skill-name          # required; unique, kebab-case
version: "1.0.0"             # optional; semver
description: >               # required
  One sentence describing what this skill does.

system_prompt: |             # required
  You are an expert in ...
  
  ## Steps
  1. Call `some_tool` with ...
  2. Then call `another_tool` to ...
  
  ## Output
  Format your answer as Markdown.

allowed_tools:               # required; min 1 entry
  - search_traces
  - get_trace_topology
  - get_span_details

output_format: markdown      # optional; default: text

constraints:                 # optional; appended to system_prompt
  - Do not speculate beyond the trace data.

examples:                    # optional but strongly recommended
  - user_query: "Why is this trace slow?"
    expected_tool_sequence:
      - get_trace_topology
      - get_span_details
    annotated_reasoning: >
      Call get_trace_topology first to see the span tree, then get_span_details
      on the slowest span to confirm attributes.
```

---

## Available MCP Tools

The following tools are available for use in `allowed_tools`. Only reference
tools listed here; unknown tool names are rejected at load time.

| Tool | Description |
|---|---|
| `search_traces` | Search for traces by service, operation, tags, and time range |
| `get_trace_topology` | Get the parent/child span tree structure of a trace |
| `get_critical_path` | Get the critical path segments and their latency contribution |
| `get_span_details` | Get full attributes, events, and timing for a specific span |
| `get_trace_errors` | Get all error spans and their status codes / messages |
| `get_services` | List all services currently registered in Jaeger |
| `list_skills` | List available AI skills (meta-tool; available to all skills) |

---

## Writing Effective System Prompts

### Structure your prompt with sections

```
## Your Role
Brief description of the agent's persona/expertise.

## Analysis Steps
Numbered steps referencing specific tool names.

## Output Requirements
Describe expected format, level of detail, and mandatory fields.

## Constraints
What the agent must NOT do.
```

### Reference tools by exact name

Always use backtick-quoted tool names in your prompt (e.g. `` `get_critical_path` ``).
This reduces hallucination and makes the expected tool call sequence explicit.

### Preserve structured trace data

The MCP tools return structured JSON. Your prompt should instruct the agent
to reason over this structure — *not* to summarize it into plain text before
analyzing. For example:

```
# Good
"Call `get_trace_topology` and examine the parent/child relationships
 to find spans with more than 10 children."

# Avoid  
"Summarize the trace and then analyze it."
```

### Use examples for uncommon patterns

The `examples` field is injected into the system prompt as few-shot
demonstrations. Include at least one example for skills targeting non-obvious
patterns (e.g. circuit breaker trips, fan-out amplification).

---

## Deploying Skills

1. **Create your skill file** (e.g. `my-analysis.yaml`) following the format above.

2. **Place it in the skills directory.** By default, Jaeger looks for skills in:
   ```
   ./skills/
   ```
   Configure a custom path in `jaeger-config.yaml`:
   ```yaml
   extensions:
     jaegermcp:
       skills_dir: /etc/jaeger/skills
   ```

3. **Restart Jaeger.** The skills loader runs at startup. Invalid files are
   logged as errors but do not prevent Jaeger from starting.

4. **Verify loading** via the `list_skills` MCP tool or the Jaeger UI skill picker.

---

## Validation Errors

Common errors you may see in Jaeger logs:

| Error | Cause | Fix |
|---|---|---|
| `skill.name must not be empty` | Missing `name` field | Add a kebab-case `name` |
| `system_prompt must not be empty` | Missing `system_prompt` | Add a non-empty prompt |
| `allowed_tools references unknown MCP tool "X"` | Typo or tool not yet implemented | Check the tool name in Available MCP Tools above |
| `skill name "X" is defined more than once` | Two files with the same `name` | Rename one skill |

---

## Example Skills

Two built-in example skills are included with Jaeger:

- **`analyze-critical-path`** — Identifies the critical path and top latency contributors.
- **`detect-n-plus-one`** — Detects the N+1 query anti-pattern.

These are located in `cmd/jaeger/internal/extension/jaegermcp/internal/skills/`
and can be used as templates.

---

## Contributing a Skill to Jaeger

If you author a generally useful skill, consider contributing it upstream:

1. Add your YAML file to `cmd/jaeger/internal/extension/jaegermcp/internal/skills/`.
2. Add an entry to `docs/skills-authoring-guide.md` (this file) under "Built-in Skills."
3. Open a PR referencing issue [#8440](https://github.com/jaegertracing/jaeger/issues/8440).

Skills submitted upstream are reviewed for correctness of tool references and
clarity of prompt instructions.
