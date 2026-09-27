# Writing a skill

A *skill* is a markdown playbook an AI agent reads before attempting a trace
analysis task. It carries the judgement a telemetry tool cannot: which of two
similar-looking traces is actually an N+1, which errored span is the cause and
which is a bystander. The agent supplies the reasoning; the skill supplies the
procedure worth reasoning about.

This page is about writing one. For serving skills from an installation —
`ai.mcp.skills_dir`, layout, limits — see [README.md](./README.md).

The two skills in the binary are worked examples:
[detect-n-plus-one](./skills/detect-n-plus-one/SKILL.md) and
[error-root-cause](./skills/error-root-cause/SKILL.md).

## When a skill is the right answer

Write one when a competent analyst would reach a different conclusion than the
obvious reading of the data — and can say why, as a procedure. That is the whole
test. If the tools already answer the question, a skill adds a read for nothing.

Skills are a poor fit for anything that is really a missing tool, a missing span
attribute, or a policy your team enforces elsewhere. They are text an agent
chooses to read, not a rule it must obey.

## The trigger is what gets it read

Jaeger parses nothing inside a skill. An agent chooses what to open from the
root index and from nothing else, so **the one line you write beside a link is
what decides whether that skill is ever read.** Everything else in the file is
addressed to a reader who has already arrived.

The rule for that line is narrower than it looks: **it has to be checkable
before the skill runs.** An agent matches it against what it knows going in —
the user's question, and whatever it has cheaply observed — not against what it
will know afterwards.

The same skill, written three ways:

- ❌ **Says what it does.** "Analyses span timing distributions to classify
  sibling groups." Nothing here describes a situation, so there is nothing to
  match against.
- ❌ **Circular.** "Use when a trace shows many repeated near-identical child
  spans." This reads as: *if a trace makes many repeated calls, use the skill
  that will tell you it is making many repeated calls.* You would already have
  had to do the analysis to know the trigger applied.
- ✅ **Checkable.** "Use when a request is slow and the cause is not yet known,
  or when the user asks about repeated queries or chatty database access." Both
  halves are known before any analysis: one from the trace's duration, the other
  from what the user said.

The test is mechanical: **could you tell that your trigger applies without
running the skill?** If not, it names the finding rather than the situation, and
it will only match once it is too late to be useful.

Two sources are always legitimate, and between them usually enough:

- **What the user said**, in their words. "The user asks why a request failed."
- **What is cheap to observe** before any real analysis. "A trace is slow."
  "Some span in the trace carries an error status."

The built-in skills are a matched pair on this. `error-root-cause` triggers on
"a trace has errors and the user asks why it failed" — both checkable up front.
`detect-n-plus-one` triggers partly on the trace "showing repeated downstream
calls", which is the finding, not the situation.

Two mechanical rules for the link itself:

- **Write it relative to the skills root**, not to the linking file's own
  directory. The agent passes the link text straight back to `read_skill`, so a
  path that resolves in a markdown preview can still be a broken link. Your entry
  point is `<skills_dir>/SKILL.md` on disk, but an agent asks for it as
  `custom/SKILL.md` — so a skill in `<skills_dir>/slow-checkout-triage/` is
  linked as `custom/slow-checkout-triage/SKILL.md`, **not** as
  `slow-checkout-triage/SKILL.md`.
- **`SKILL.md` is required only at the root.** Below it a link may point at any
  file under any name; one directory per skill is a convention, not a rule.

```markdown
- [slow-checkout-triage](custom/slow-checkout-triage/SKILL.md) — Triage a slow
  checkout by separating queue wait from service time. Use when a checkout
  request is slow and nothing in the trace errored.
```

## Whether it gets read

An agent will not always open a skill, even when your trigger matches, and it is
often right not to. If the answer is something a capable model already knows,
reading the skill only spends context. The skills worth writing are the ones
that change the answer, not the ones that restate it.

Where a skill does change the answer, naming it in the prompt is what reliably
gets it read today. [#9336](https://github.com/jaegertracing/jaeger/issues/9336)
tracks what has been measured about reaching them without that.

## Anatomy

A skill is plain markdown, optionally opening with YAML frontmatter:

```markdown
---
name: slow-checkout-triage
description: >-
  Triage a slow checkout request by separating queue wait from service time.
  Use when a checkout or payment trace is slow but no span shows an error.
allowed-tools: get_trace_topology get_critical_path get_span_details
---

# Slow Checkout Triage

## When this applies
...

## Procedure
1. ...

## Gotchas
- ...
```

Jaeger reads no field of that block — it serves the file verbatim, frontmatter
included, and the agent sees the lot. So `allowed-tools` documents which tools
the procedure expects; it is not a sandbox, and nothing stops an agent calling
something else.

Frontmatter is still worth writing, for two reasons. It travels: both the
[agent skills specification](https://agentskills.io/specification) and the
[Open Knowledge Format](https://cloud.google.com/blog/products/data-analytics/how-the-open-knowledge-format-can-improve-data-sharing)
use one, so a skill written for either stays readable here. And keeping
`description` identical to the skill's index line means the two cannot drift —
worth doing, since proposals to build the catalog from frontmatter instead of by
hand are live (see [#9336](https://github.com/jaegertracing/jaeger/issues/9336)).

Keep `name` matching the directory name for the same reason.

The three body headings are convention, and they earn their place: **When this
applies** lets an agent bail out of a skill it opened speculatively, **Procedure**
is the part it follows, and **Gotchas** is where the failure modes go.

## Keep it small

Progressive disclosure is the point: the agent reads the index, then one skill,
not the tree. So a skill is worth splitting as soon as it covers two situations
a reader would arrive at separately — each with its own entry and its own trigger
line in the index.

As a working target, keep a single skill under roughly 500 lines (~5,000
tokens). That is a design target chosen to keep one skill a small fraction of a
working context; it is not a measured threshold, and nothing enforces it. The
only hard limit is the 512 KiB served-file cap, which is orders of magnitude
larger and no guide at all. In practice, when a skill approaches 500 lines the
reason to split it is that it has stopped being about one situation.

## Write a procedure that decides

The failure mode is not a procedure that is wrong, it is one that cannot be
applied. Two readers following it must reach the same answer.

`detect-n-plus-one` originally qualified a group on count plus "similar
durations (within 2x of the median)". Measured against captured traces, a
*parallel fan-out* scored 82% on that criterion and a genuine N+1 scored 77% — a
retry in the real N+1 made it less uniform than the fan-out, so on the rule's own
terms the negative looked more like an N+1 than the positive. The rule also never
said how many siblings had to comply. It read like a criterion and decided
nothing.

The fix ([#9263](https://github.com/jaegertracing/jaeger/pull/9263)) names a
measurement and a comparison instead: sum the siblings' durations, compare with
the elapsed window from earliest start to latest end, and read roughly-equal as
serial. Same intent, now with an answer.

So:

- **Name the measurement**, not the impression. "Sum of durations versus elapsed
  window", not "similar durations".
- **Say what to do with each outcome**, including the negative one. A step that
  only describes the passing case leaves the agent to invent the rest.
- **Order steps by what eliminates most.** Cheap disqualifying checks first,
  expensive per-span lookups last.
- **End with what to report**, naming the evidence. Both built-in skills close by
  listing the fields the answer must carry — that is what stops a confident
  summary with nothing behind it.

## Gotchas are where the value hides

**Gotchas** is not a footnote section. It is where the cases go in which the
obvious reading is wrong — and those cases are usually why the skill exists:

> A timed-out call often has nothing below it. The work it was waiting on never
> returned, so no spans were recorded for it. Name the last span you can see and
> say the cause lies beneath it, rather than blaming the timeout itself.

Every gotcha should be something you have watched mislead someone. Write the
trap, then the correct reading.

## Test it against a real trace

A skill you have only read is untested. The built-in skills are being checked
against captured trace fixtures in
[#9263](https://github.com/jaegertracing/jaeger/pull/9263); two rules from that
work are worth borrowing whatever tooling you use.

- **Make the fixture adversarial.** A trace on which any reasonable procedure
  succeeds proves nothing. Pick one where the naive answer and the correct answer
  differ, so it can tell you your procedure is not pulling its weight.
- **Derive the expected answer mechanically.** Have it follow from measured
  timings or a deliberately injected fault, so the label is a fact about the
  capture rather than a judgement you can quietly revise to match whatever your
  skill happens to say.

This is not a formality — it is what turned up the `detect-n-plus-one` defect
above. Both built-in skills changed as a result.

## Checklist

- [ ] Linked from the index, relative to the skills root.
- [ ] The index line is checkable before the skill runs — you could tell it
      applies without doing the analysis.
- [ ] `name` matches the directory; frontmatter `description` matches the index
      line.
- [ ] Every step names a measurement and says what to do with each outcome.
- [ ] The final step says what to report and what evidence to carry.
- [ ] Gotchas cover the cases where the obvious reading is wrong.
- [ ] Checked against at least one trace where the naive answer is wrong.
