# Evaluating Jaeger's built-in skills

Methodology and results for the `detect-n-plus-one` and `error-root-cause` skills. Written so
the setup can be checked independently of the conclusions, and so a later evaluation harness
([#9135](https://github.com/jaegertracing/jaeger/issues/9135)) can consume the fixtures and
records rather than start over.

Fixtures and manifests:
`cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/handlers/testdata/skills/`.
Transcripts, run records, prompts and scorers are attached to the pull request.

---

## 1. Summary

Three claims, in decreasing order of confidence.

1. **The rewritten `detect-n-plus-one` step 3 works when the skill is read.** Two Claude
   models went from consistently wrong to consistently right on an adversarial fixture pair
   (30/30 with the skill, 0/15 without). The unaided failure was mechanistically identical
   across models: both inferred seriality from staggered start times instead of measuring
   overlap.

2. **It does not transfer to every model.** On `gpt-4o-mini` the same skill produced no
   correct answers. On `gemma-3-27b-it` the agent never emitted a real tool call, so nothing
   about the skill could be measured.

3. **Agents do not find the skill on their own.** Once prompts stopped naming the skill —
   the deployed condition — `read_skill` was called in **0 of 15 runs** across three arms.
   The agent went straight to telemetry tools. Where the skill's judgement was needed, it
   answered incorrectly 5/5. Naming the skill in the prompt restores it immediately: 5/5
   opened it and 5/5 returned the exact expected locus.

Claim 3 is the important one. Skill *content* is demonstrably fixable; skill *discovery* is
currently the binding constraint, and no amount of improving the procedure text changes an
outcome where the procedure is never read.

## 2. What is measured

**Q1 — does the skill change behaviour?** One model, one fixture, two arms differing only in
whether the skill is used.

**Q2 — does that hold across models?** The same comparison per model. Q1's answer is
model-dependent, so this is not a formality.

**Q3 — is the skill found when not named?** Prompts describe the task without naming the
skill, and the tool-call record shows whether the catalog was ever opened.

Nothing here measures whether the skills' advice is good in general. It measures behaviour on
four fixtures.

## 3. Fixtures

Each is built so **the naive answer and the correct answer diverge**. A trace on which any
reasonable procedure succeeds discriminates nothing.

| Fixture | Skill | Naive answer | Correct answer |
| --- | --- | --- | --- |
| `n_plus_one_positive` | `detect-n-plus-one` | — | is an N+1 |
| `n_plus_one_near_miss` | `detect-n-plus-one` | "N+1 — 11 repeated siblings" | not an N+1; the calls overlap |
| `error_timeout_masked` | `error-root-cause` | `frontend-proxy/POST`, the only errored span | `frontend/POST`, which carries no error status |
| `sibling_errors` | `error-root-cause` | ambiguous; two candidates tie | `payment/Charge`, the earlier failure |

Manifests record `label_derived_from`: `capture` (the label follows from measured timings or a
seeded fault) or `construction` (the trace was built to have the property; only
`sibling_errors`). Measured properties, verified against the fixture bytes by
`TestSkillFixtures_*`:

| Fixture | Siblings | Sum of durations | Elapsed window | Overlapping pairs |
| --- | --- | --- | --- | --- |
| `n_plus_one_positive` | 13 | 192.4 ms | 194.3 ms | 0 of 12 |
| `n_plus_one_near_miss` | 11 | 1815.7 ms | 1723.3 ms | 8 of 10 |

Sources: HotROD (image digest recorded per manifest) and the OpenTelemetry Demo with named
feature flags. `sibling_errors` is synthetic because two rounds of demo fault injection across
620 traces produced no parent with two errored children — the demo's services abort on first
failure.

## 4. Scoring

**One discrete label per run.** A boolean for `detect-n-plus-one`; a `service/operation` locus
for `error-root-cause`. Prose is not scored: a score over free text measures fluency and
drifts with every model release. The locus rather than the mechanism, because nothing in a
span says "CPU" or "garbage collection", so scoring the mechanism rewards a lucky guess.

**Extraction is mechanical and pre-registered.** Scorers are written and hashed before the
runs they score, and are not retuned afterwards on any pretext. Round two's prompts require a
terminal `VERDICT:` line, so extraction parses that line rather than pattern-matching prose.
Categories: a verdict, plus `ABSTAINED`, `UNPARSED`, `NO_ANSWER`, `PROVIDER_ERROR`. Absence of
a verdict is reported, never guessed.

**Groundedness is recorded but not gating** — whether the reply cites the deciding measurement
(sum versus window; the long non-errored child). This makes right-answer-wrong-reasoning
visible in the data instead of hiding it behind a correct label.

**Discovery is read from the tool-call record**, not from prose: which skill, if any, was
opened before the verdict.

**Human adjudication, where used, is reported separately and never merged into automated
totals.**

## 5. Harness

The gateway is bring-your-own-agent over [ACP](https://agentclientprotocol.com/), so the model
is a variable rather than a constant. Any agent presenting that surface is indistinguishable
to the gateway. Three were used: the in-repo Gemini sidecar; a thin ACP sidecar calling any
[OpenRouter](https://openrouter.ai/) model; and
[`@zed-industries/claude-code-acp`](https://www.npmjs.com/package/@zed-industries/claude-code-acp),
which speaks ACP over stdio and is bridged to the gateway's WebSocket.

Jaeger is built from the branch under test, run with in-memory storage and MCP enabled, and
the fixtures — already OTLP JSON — are replayed to the collector's OTLP/HTTP endpoint. Turns
are driven against `POST /api/ai/chat`; the AG-UI stream is kept verbatim as the transcript.

Two harness facts that affect any metric derived from these runs:

- **Tool-call counts are not comparable across agents.** The Claude adapter emits each
  `TOOL_CALL_START` twice; the Python sidecars emit once. All counts here are deduplicated by
  `toolCallId`. Claude runs also include `ToolSearch` calls belonging to that adapter's own
  tool-loading mechanism, not to Jaeger's MCP surface.
- **The serving agent is asserted, not assumed.** Before each suite, the harness reads the
  model identifier from the environment of the process owning the agent port. See §8.2 for why.

## 6. Results

### 6.1 With the skill read (prompts naming the skill)

n = 3 per arm per model, except Haiku at n = 12.

| Model | No skill, near-miss | Skill, near-miss | Skill, positive |
| --- | --- | --- | --- |
| `claude-haiku-4-5` | 0/12 | **12/12** | **12/12** |
| `claude-sonnet-4-5` | 0/3 | **3/3** | **3/3** |
| `openai/gpt-4o-mini` | 2/3 | 0/3 | 0/3 |
| `google/gemma-3-27b-it` | 0/3 | 0/3 | 0/3 — zero tool calls in all 9 runs |

The Claude models are perfect with the skill and never correct without it. The unaided
failures share one mechanism — inferring seriality from staggered start times:

> "the start-time staggering of these 10 `/route` spans (spread across ~128ms) … confirming
> that the one-customer-fetch-then-N-route-lookups fan-out is a serial N+1 pattern"

The fixture contradicts this: the calls overlap, summing to 1815.7 ms across a 1723.3 ms
window. Step 3 now replaces that inference with the measurement.

`gpt-4o-mini` inverts the pattern — correct twice unaided, never correct with the skill, and
three of nine skill-arm runs never called `read_skill` despite being asked. On this model the
skill is at best inert. `gemma-3-27b-it` emitted tool calls as literal text rather than
protocol calls; its runs carry no information about the skill either way.

### 6.2 Without naming the skill — the deployed condition

Prompts describe the task and do not mention skills. `claude-haiku-4-5`, n = 5 per arm.

| Arm | Fixture | Correct | Grounded | Opened the skill |
| --- | --- | --- | --- | --- |
| Implicit N+1 | `n_plus_one_near_miss` | **0/5** | 2/5 | **0/5** |
| Error locus | `error_timeout_masked` | 3/5 | 5/5 | **0/5** |
| Error locus | `sibling_errors` | 5/5 | 0/5 | **0/5** |
| Error locus, *skill named* | `error_timeout_masked` | **5/5** | 4/5 | **5/5** |

**`read_skill` was never called.** In every run the agent went directly to telemetry tools.

The consequences differ by fixture, which is itself informative. On the near-miss — where the
skill's judgement is the whole point — it answered incorrectly 5/5, the same failure as the
no-skill arm. On `sibling_errors` it was correct 5/5 using two tool calls, so that fixture
does not require the skill for this model. On the masked fixture it was correct 3/5, splitting
between the right locus and the naive `frontend-proxy/POST`.

The final row is the control: the same fixture with a prompt that names the skill. All five
runs opened `SKILL.md` then `error-root-cause/SKILL.md`, and all five returned exactly
`frontend/POST` — unanimous and matching the manifest. Without the skill the answers split
three ways and twice landed on `frontend-proxy/POST`, the naive answer the fixture encodes.
The difference is not that the model cannot reach the locus; it is that it does not reliably
do so, and does not reach for the procedure that would make it reliable.

**Leading hypothesis, not yet tested:** the agent is never told to start from the catalog. The
gateway supplies that guidance as MCP server instructions, and whether a given ACP agent
surfaces those to its model is agent-specific. The cheap next test is to check whether the
instruction text reaches the model at all, before concluding anything about the index's
wording.

## 7. What holds without any agent

The Go tests assert the tool layer deterministically, with no model: that `get_trace_topology`
exposes the repeated group with timings separating serial from overlapping, and that
`get_trace_errors` reaches the seeded failure. This establishes that the evidence a procedure
needs is reachable — not that an agent uses it, which §6.2 shows it often does not.

The agent half cannot run in CI: no model is available there, and adding a paid,
non-deterministic call to CI is not proposed. It is manual, with transcripts archived.

## 8. Threats to validity

### 8.1 The ablation is instructed, not enforced

The no-skill arms asked the agent not to read skills rather than making skills unavailable.
Compliance was 0 violations in 21 runs, but that is not the same as absence. Enforcing it
requires a build with the skill pruned from the embedded tree, because `read_skill` routes by
prefix — an operator tree can add skills but cannot shadow a built-in one. That build was
specified but not run.

### 8.2 An earlier batch of 36 runs was mislabelled

Four suites were launched intending four models. The agent-switching step matched processes by
command-line pattern, which silently failed, so one already-running agent served all of them;
each subsequent agent failed to bind the occupied port and exited, while the readiness check —
"is something listening" — passed against the wrong process.

Caught before publication, because four "different models" wrote in identical style and one
had implausibly started tool-calling. Those runs are retained with corrected labels and
counted as what they are. The harness now asserts the serving model from the process
environment before every suite. Two conclusions drawn from that batch were **withdrawn**: that
`gpt-4o-mini`'s earlier failure had not reproduced, and that `gemma`'s tool calling had begun
working.

### 8.3 Small and unbalanced n

n = 3 for three models, n = 12 for Haiku, n = 5 for the discovery arms. No confidence
intervals. Only Haiku's results have enough runs to be more than suggestive.

### 8.4 Scorer coverage

Round one's scorer returned `UNPARSED` for 14 of 63 runs, concentrated where answers are
discursive. It was deliberately not retuned after the fact; human readings are reported
separately. Round two's mandatory `VERDICT:` line removes most of that ambiguity.

### 8.5 Sampling parameters unrecorded

Temperature and top_p were provider defaults and not captured, so runs are not exactly
reproducible.

### 8.6 Narrow coverage

One fixture pair carries the N+1 comparison. The discovery result is one model. HotROD traces
are uniform — 40 spans, depth 3, six services — so these fixtures test pattern recognition,
not behaviour on deep or noisy traces.

## 9. The change this justified

`detect-n-plus-one` step 3 previously read:

> Flag any group with more than 10 near-identical siblings as a potential N+1 pattern. Check
> that children have similar durations (within 2x of the median).

Measured against the fixtures, 10 of 13 siblings (77%) fall within 2× of the median in the
genuine N+1, against 9 of 11 (82%) in the fan-out — a retry outlier makes the real N+1 less
uniform. On the criterion's own terms the negative scores higher than the positive, and the
rule never said how many siblings must comply, so it could not be applied deterministically.

Step 3 now compares the sum of sibling durations against the window they occupy, applying the
count afterwards. `error-root-cause` step 4 similarly now descends into a long non-errored
child before accepting a candidate, which is the case the masked fixture encodes.

## 10. What follows

- **Discovery is the priority.** Establish whether the catalog instruction reaches the model
  at all before revising index wording. A skill that is never opened cannot be evaluated on
  its content.
- **Enforced ablation** (§8.1) to confirm the instructed result.
- **Enforced ablation for `error-root-cause`** — the skill-named control (§6.2) shows the
  skill produces an exact, unanimous locus, but the comparison arm is discovery-condition
  rather than skill-removed.
- **Fixtures and records are the durable output.** They are labelled, provenance-stamped, and
  in the format an evaluation harness would consume.

## Appendix A — prompts

Verbatim. `$TRACE` is substituted with a fixture's trace ID; nothing else varies between arms
of the same class. Prompt files are hashed and the hash is recorded in every run record.

**Round one.** These name the skill, so they measure whether it helps when force-fed.

> *No skill, near-miss* — Do not read any skills and do not use the read_skill tool. Using
> only the telemetry tools, examine trace `$TRACE` and answer in three sentences: does it
> contain an N+1 query pattern, and what measurement decided it?

<!-- -->

> *With skill* — Read your skills catalog, then open the skill for detecting N+1 query
> patterns and follow its procedure on trace `$TRACE`. Then answer in three sentences: is this
> an N+1, and what measurement decided it?

**Round two.** These do not name the skill, so they measure the deployed condition, and they
require a terminal verdict line so extraction is mechanical.

> *Explicit, N+1* — Examine trace `$TRACE` and answer: does it contain an N+1 query pattern,
> and what measurement decided it? End your reply with a single line: `VERDICT: N+1` or
> `VERDICT: NOT_N+1`.

<!-- -->

> *Explicit, error locus* — Trace `$TRACE` failed. Which service and operation caused it? End
> your reply with a single line: `VERDICT: <service>/<operation>`.

<!-- -->

> *Implicit* — Trace `$TRACE` seems slow and makes a lot of repeated calls. What's going on?
> End with `VERDICT: N+1` or `VERDICT: NOT_N+1`.

<!-- -->

> *Contextual* — A user reported checkout being slow again this morning. I pulled trace
> `$TRACE` — can you take a look? End with `VERDICT: N+1` or `VERDICT: NOT_N+1`.

The change between rounds is deliberate and is why §6.1 and §6.2 are not directly comparable:
round one measures the skill's content, round two measures whether it is reached at all.

## Appendix B — models and dates

| Model | Provider / agent | Runs | Date |
| --- | --- | --- | --- |
| `claude-haiku-4-5-20251001` | Anthropic subscription, `claude-code-acp` + WebSocket bridge | 51 | 8 Aug 2026 |
| `claude-sonnet-4-5` | same | 9 | 8 Aug 2026 |
| `openai/gpt-4o-mini` | OpenRouter, thin ACP sidecar | 9 | 8 Aug 2026 |
| `google/gemma-3-27b-it` | OpenRouter, thin ACP sidecar | 9 | 8 Aug 2026 |
| `gemini-2.5-flash` | in-repo sidecar | 6 | 7 Aug 2026 |

Sampling parameters were provider defaults throughout and were not captured (§8.5).
