# Skill fixtures

Trace fixtures for testing what the built-in skills' procedures actually conclude, rather
than only that the skill files are servable.

Each fixture is OTLP JSON — the format Jaeger's v3 API returns and `ptrace.JSONUnmarshaler`
consumes — paired with a `.manifest.json` recording where it came from, whether the pattern
is present, and the expected answer. The manifest is what makes these usable as benchmark
samples for [#9135](https://github.com/jaegertracing/jaeger/issues/9135) rather than opaque
bytes.

| Fixture | Skill | Pattern | Expected answer |
| --- | --- | --- | --- |
| `n_plus_one_positive.json` | `detect-n-plus-one` | present | 13 serial `GetDriver` siblings under `be56eec6f3678130` |
| `n_plus_one_near_miss.json` | `detect-n-plus-one` | absent | 11 *overlapping* `HTTP GET` siblings — a fan-out, not an N+1 |
| `error_timeout_masked.json` | `error-root-cause` | present | locus is `frontend/POST`, which carries no error status |
| `sibling_errors.json` | `error-root-cause` | present | locus is `payment/Charge`, the earlier of two failed siblings |
| `n_plus_one_full_concurrent.json` | `detect-n-plus-one` | absent | whole 40-span trace; the route group overlaps |
| `n_plus_one_full_serial.json` | `detect-n-plus-one` | present | the same trace at `-W 1`; the route group is serial |

## How a fixture is built

Each fixture pairs with a `.manifest.json` recording where the trace came from, whether the
pattern is present, and the answer a skill should reach. Two rules govern the set.

**Every fixture is adversarial: the naive answer and the correct answer must diverge.** A
trace where any reasonable procedure succeeds measures nothing. So the N+1 pair is two
repeated sibling groups that differ only in timing, and both error fixtures are cases the
"deepest errored span with no errored children" rule gets wrong on its own — one because the
failing span carries no error status, the other because two candidates tie.

**Every expected answer is derived mechanically, never judged.** A manifest records how:

| `label_derived_from` | Meaning |
| --- | --- |
| `capture` | The label follows from the captured trace — measured timings, or the fault the feature flag seeded |
| `construction` | The trace was built to have the property, so the label is true by construction |

Manifests also carry `skill_rule_reaches_expected_answer`, which is `false` where the skill's
own procedure is insufficient and the fixture exists to prove it.

## Trimmed and untrimmed fixtures

Four fixtures are trimmed to the span set that carries the pattern; two are whole traces.
Both exist on purpose.

Trimming is not only about keeping a diff readable. A whole HotROD trace contains *both*
patterns at once — a genuine N+1 in the driver service and a concurrent fan-out in the
frontend — so a whole-trace negative is only meaningful once it names the group it is about.
The trimmed near-miss makes "not an N+1" true of the entire file.

The untrimmed pair takes the other approach and scopes the claim with
`expected.parent_span_id`. It is a matched pair captured from HotROD's `-W` flag, which sets
the route-service worker pool: at `-W 3` the eleven route calls overlap, at `-W 1` the same
eleven run one after another. Same services, same call graph, same span count — only the
timing differs, so the ground truth is a property of the flag the capture ran under rather
than a judgement about the trace. That makes them the fixtures for asking whether an agent
can find the pattern amid unrelated spans, which the trimmed ones cannot test.

## What is scored, and what is not

Only a **discrete label** is scored: a boolean for `detect-n-plus-one`, and for
`error-root-cause` a **locus** — the service and operation the failure originates from.
Explanations, recommendations, and severity judgements are not scored, because a score over
free prose measures the model's fluency rather than the procedure.

The locus is deliberately not the *mechanism*. Nothing in a span says "CPU" or "garbage
collection", so scoring the mechanism rewards a guess that happens to land. Where a trace
cannot support a mechanism at all, the manifest says so via
`mechanism_determinable_from_trace: false`.

## What these tests verify, and what they cannot

The Go tests here assert the **tool layer**: that `get_trace_topology` and `get_trace_errors`
expose enough for a skill's procedure to reach the labelled answer — the repeated group
survives the call, its timings distinguish serial from overlapping, and the error list
reaches the seeded failure. This runs in CI and involves no model.

Whether an agent *following the prose* arrives at that answer cannot run in CI: there is no
model available there, and adding a paid, non-deterministic call to CI is not proposed. That
half is a manual run against a real ACP agent, captured as evidence on the pull request. To
attribute a result to the skill rather than the model, run the same fixture twice with the
same agent — once with the skill available and once without — and compare; a difference
between models on the same skill is a property of the model, not of the playbook.

## Regenerating

Fixtures are captured from HotROD; each HotROD manifest records the exact image digest it
was captured from. The OpenTelemetry Demo fixture records the demo commit and the feature
flags instead, and the synthetic fixture records neither.

```bash
# 1. bring up HotROD and Jaeger (Compose v1 syntax; use `docker compose` if you have v2)
cd examples/hotrod && docker-compose up -d

# 2. drive load — one dispatch produces both patterns, but more traces give the
#    trimmer a choice of clean ones
for i in $(seq 1 40); do for c in 123 392 731 567; do
  echo "http://localhost:8080/dispatch?customer=$c&nonse=$i$RANDOM"
done; done | xargs -P 6 -n 1 curl -s -o /dev/null

# 3. pull the traces as OTLP JSON
MIN=$(date -u -d '2 hours ago' +%Y-%m-%dT%H:%M:%S.000000Z)
MAX=$(date -u -d '+5 minutes' +%Y-%m-%dT%H:%M:%S.000000Z)
curl -s "http://localhost:16686/api/v3/traces?query.service_name=frontend\
&query.start_time_min=$MIN&query.start_time_max=$MAX&query.num_traces=200" -o traces.json

# 4. trim to fixtures
python3 regenerate.py traces.json .
```

Both time bounds are **required** by the v3 API, and `start_time_max` must be slightly in the
future — passing exactly *now* returns `404 No traces found`.

## What these fixtures do and do not cover

HotROD emits the same *shape* on every dispatch: the same services, the same call graph,
maximum depth 3. The span count varies a little, because failed Redis calls are retried and
each retry adds a span — captures here ranged from 39 to 40 spans. That makes it a good
source of *pattern* fixtures, since the N+1 and the fan-out are textbook and reproducible,
and a poor source of production variety. These fixtures test pattern recognition. They do
not test behaviour on deep, noisy, or heterogeneous traces.

HotROD's Redis client fails on a fixed schedule rather than randomly or once per request:
`errorSimulator.checkError` decrements a counter shared across the whole process and fails
every fifth `GetDriver` call. With roughly thirteen calls per dispatch that is two or three
failures per trace, which is why the positive fixture carries `STATUS_CODE_ERROR` on some of
its siblings. They are kept: a real N+1 has retries in it.
