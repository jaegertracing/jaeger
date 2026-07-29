# RFC 0014: Change-Relevance-Aware CI

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-07-28
- **Last Updated:** 2026-07-29
- **Issue:** [#3113](https://github.com/jaegertracing/jaeger/issues/3113)
- **Related:** [#1476](https://github.com/jaegertracing/jaeger/issues/1476) · [#1784](https://github.com/jaegertracing/jaeger/issues/1784) · [RFC 0013 Reliable Coverage Gating and Real E2E Coverage](./0013-coverage-gating-and-e2e-coverage.md) (in review as [#9131](https://github.com/jaegertracing/jaeger/pull/9131)) · [ADR-004 Migrating Coverage Gating to GitHub Actions](../adr/004-migrating-coverage-gating-to-github-actions.md)

---

## Implementation status

| Milestone | Scope | Status |
| --- | --- | --- |
| M1 | Advisory relevance job and the backtest harness: compute, report, change nothing | ⬜ |
| M2 | Sound wins with no engine: docs-only skip, PR-event matrix narrowing | ⬜ |
| M3 | Make the required checks tolerate a partial run | ⬜ |
| M4 | Manifest tiers 0/1/3 plus the exhaustiveness guard | ⬜ |
| M5 | Tier 2: per-leg executed-package sets measured from `covdata` | ⬜ |
| M6 | Backtest, shadow, then enforce on PR events | ⬜ |
| M7 | Extend selection to image builds and static analysis | ⬜ |
| M8 | Record the outcome as an ADR | ⬜ |

---

## Abstract

Jaeger CI is all-or-nothing: a pull request editing one `README` runs the same 65 jobs and 311 machine-minutes as one rewriting the Cassandra span writer, and 68% of that is the 31 e2e storage cells, which also own the critical path. The fix everyone reaches for is a hand-written `paths:` filter per job, and it is the wrong fix — such a filter is an unverifiable *claim* about which code a job exercises, and when it is wrong CI reports a green check for a job that never ran (§6, option B).

This RFC proposes to derive that claim instead of declaring it. Relevance comes from the Go build graph including `//go:embed` assets, from the set of packages each CI job is *observed to execute* — measured on `main` from the coverage output CI already produces — and from a fail-closed rule sending any change to the build or CI machinery to a full run, leaving ~68 runtime-only inputs (`cmd/jaeger/config-*.yaml`, `docker-compose/**`, `scripts/e2e/*.sh`) as the one declared tier, held exhaustive by a guard that fails CI on any file no rule classifies. Measurement rather than static analysis is essential: `cmd/jaeger`'s dependency closure spans 170 of the module's 242 packages, because one binary links every storage backend, so import-graph reachability cannot tell the Cassandra leg from the ClickHouse one. Reliability then rests not on the predictor being right but on four properties — selection applies to `pull_request` events only so the merge queue and `main` always run everything, a stale map disables selection rather than corrupting it, the decision is backtested against the recorded outcomes of merged pull requests before it may skip anything, and every run prints why each job ran. Two changes need no engine at all and come first, because they are sound by construction and recover 42% of the run on their own: skipping the expensive suite for documentation-only changes, and narrowing the Cassandra/Elasticsearch/OpenSearch version matrices on pull-request events while the merge queue keeps the full matrix.

---

## 1. Motivation

### 1.1 What every pull request costs

Measured on run [`30386676327`](https://github.com/jaegertracing/jaeger/actions/runs/30386676327), a representative `pull_request` run on the parallel path:

| Group | Jobs | Machine-minutes | Share |
| --- | --- | --- | --- |
| e2e legs | 31 | 212 | 68% |
| build-binaries | 8 | 30 | 10% |
| docker images | 4 | 23 | 7% |
| unit tests (Go, sidecar, CI scripts) | 3 | 17 | 5% |
| stage 1 lint checks | 8 | 13 | 4% |
| static analysis (CodeQL, FOSSA, dependency review) | 4 | 8 | 3% |
| ocb-build | 1 | 4 | 1% |
| orchestration | 6 | 0 | — |
| **total** | **65** | **311** | |

Wall-clock is 13–15 minutes on the parallel path and ~31 minutes on the sequential path external contributors get. On the parallel path the critical path is an e2e cell — the four `cassandra … e2e` cells run 10 minutes each, longer than any other job once `docker-build` narrows itself to `linux/amd64` for pull requests ([`ci-docker-build.yml:41-44`](../../.github/workflows/ci-docker-build.yml)). So e2e is not merely the bulk of the cost, it is what the author waits for.

Within e2e, cost concentrates in the version matrices:

| Leg | Cells | Machine-minutes |
| --- | --- | --- |
| cassandra | 6 | 53 |
| elasticsearch | 5 | 39 |
| opensearch | 5 | 39 |
| spm | 4 | 21 |
| clickhouse | 2 | 11 |
| badger | 2 | 9 |
| grpc | 2 | 9 |
| kafka, memory, query, tailsampling | 1 each | 6 each |
| ui-reverse-proxy | 1 | 3 |

Cassandra, Elasticsearch and OpenSearch are 16 cells and 131 machine-minutes — 42% of the entire run — and the cells within each leg differ only by backend version and schema-creation mode.

There is a second multiplier: over the last 100 orchestrator runs, 87 were `pull_request` and 13 were `push` to `main`, roughly seven pull-request runs per merge. Pull-request CI, not merge CI, is where the time goes.

### 1.2 The existing adaptations address a different question

[`ci-orchestrator.yml`](../../.github/workflows/ci-orchestrator.yml) already adapts on two axes, and neither concerns the content of the change. The three-stage sequential path exists to fail fast on cheap checks before spending on expensive ones; the parallel path exists to shorten feedback for trusted actors. Both decide **when** work runs and **who** gets it early, never whether a job is inapplicable, so a linter-only change runs every Cassandra cell either way. The one content-based narrowing in the repository is a two-entry `paths:` filter on [`ci-unit-tests-go-tip.yml:14-18`](../../.github/workflows/ci-unit-tests-go-tip.yml), guarding a non-required workflow; there is no `paths-ignore:`, no `dorny/paths-filter`, and no merge-base diff anywhere under `.github/`.

---

## 2. Proposal

### 2.1 Two principles

**Derive the claim, never declare it.** A statement that a job is unnecessary must trace to something a script recomputed from the repository. Any input that cannot be recomputed fails closed — the job runs.

**Selection is a pull-request latency optimization, never a merge gate.** It applies to `pull_request` events only; `merge_group` and `push` to `main` continue to run the full suite. This restriction is what makes the rest defensible: an imperfect predictor becomes a bounded latency risk, because the worst outcome of a bad selection is a merge-queue rejection rather than a defect on `main`.

A corollary keeps the scope honest: the cheap checks stay unconditional. Selection targets the 31 e2e cells and the image builds. Stage 1 lint (13 minutes across 8 jobs), `make test` (8 minutes, whole-module and the backbone of the coverage report), the CI scripts Jest suite, and the DCO and label checks are cheap and global, and gating them buys minutes while adding a class of mistake.

### 2.2 Why this has to be measured, not inferred

The mechanical starting point is the build graph. `go list -deps` gives each package's transitive imports, and inverting it yields every package a change could affect. `go list -json` additionally reports `EmbedFiles`, which attaches non-Go assets to the graph for free: `internal/storage/v1/cassandra/schema` reports `v004-go-tmpl.cql.tmpl`, so the Cassandra schema template is attributed to a package without anyone writing a rule, as are `internal/storage/elasticsearch/esclient/index_templates/*.json` and the 13 ClickHouse DDL files under `internal/storage/v2/clickhouse/sql`.

Where it breaks is the unified binary. The e2e harness spawns `./cmd/jaeger/jaeger` ([`e2e_integration.go:105`](../../cmd/jaeger/internal/integration/e2e_integration.go)), and that binary registers every storage backend:

```
$ go list -deps ./cmd/jaeger | grep -c jaegertracing/jaeger
170          # out of 242 packages in the module
```

`internal/storage/v2/clickhouse/sql` is in the closure of the binary the *Cassandra* leg runs, because that binary links ClickHouse whether or not it ever calls it. Reachability therefore marks 70% of the module relevant to all 12 e2e legs at once, and a purely static scheme skips almost nothing for almost any Go change. The graph answers "could this change alter the bytes of the binary this leg runs" — a sound question, and the wrong one.

The question that discriminates is "which packages does this leg actually run", and it is answerable by measurement. The 11 `direct` legs already run storage code in-process under `-coverpkg=./...` and produce real profiles today ([RFC 0013 §2.1](./0013-coverage-gating-and-e2e-coverage.md)), so their executed-package sets are already in artifacts the run uploads; the spawned-binary legs are exactly what [RFC 0013](./0013-coverage-gating-and-e2e-coverage.md) M3–M5 fixes with `go build -cover` and `GOCOVERDIR`. This is the mechanism behind Azure DevOps' Test Impact Analysis, and it is preferable to the model-trained alternative (Develocity Predictive Test Selection) because a measured map is inspectable and its errors are attributable.

One independence is worth stating, because it decouples this RFC from RFC 0013's decision gate: RFC 0013 M4 gates rollout on whether binary coverage adds *material* coverage, whereas a relevance map needs only the **set of packages with a non-zero counter**. A leg whose new coverage is 0.2 percentage points still names precisely the packages it executed. If M4 finds the coverage immaterial for gating, the instrumentation remains worth keeping for this purpose.

### 2.3 The relevance manifest: four tiers

Every tracked path resolves through exactly one tier, tried in order.

| Tier | Covers | Source | Maintenance |
| --- | --- | --- | --- |
| 0 | `.github/**`, `Makefile`, `scripts/makefiles/**`, `go.mod`, `go.sum`, `Dockerfile*`, `.codecov.yml`, submodule pointers | rule: disable selection, run everything | none |
| 1 | `*.go` and `//go:embed` assets | `go list -deps -json`, inverted | none |
| 2 | which leg executes which package | measured on `main` from `coverage-*` artifacts | none |
| 3 | 24 `cmd/jaeger/config-*.y*ml`, 36 files under `docker-compose/`, 8 `scripts/e2e/*.sh` | declared, generated by reference scan | 68 files, guarded |

Tier 0 is deliberately broad: these inputs can change the meaning of every job, including the relevance engine itself, so analyzing them is not worth attempting and "the CI change ran under the old CI rules" is made impossible.

Tier 2 is a checked-in JSON, so a change in what a leg covers appears as a reviewable diff rather than as invisible cache state. Two legs have no in-process profile to derive it from: `spm` (4 cells, 21 minutes) drives `docker-compose/monitor` with `curl` and `jq` from [`scripts/e2e/spm.sh`](../../scripts/e2e/spm.sh), and `ui-reverse-proxy` does the same. Both run the same instrumented binary inside `docker-compose`, so the measurement is available in principle by mounting a `GOCOVERDIR` volume and stopping the stack gracefully; until that exists they are classified by tier 3 alone, which for `spm` means it runs on any change to the metrics pipeline, the four metricstore backends, or its compose stack — conservative, and still skipping `spm` for a pure Cassandra or Badger change.

Tier 3 is the only declared tier, and two properties keep it from becoming the hand-written path list §6 rejects. It is **generated where it can be**: the config files a leg uses are named in that leg's test source or shell script, so a reference scan proposes the mapping and the guard verifies that every `cmd/jaeger/config-*.y*ml` and every `docker-compose/**` file is claimed by at least one leg that references it. And it is **exhaustive by enforcement**, per §2.4.

### 2.4 The guards

The tiers decide; the guards are what make the decision trustworthy. Each converts a class of silent error into a loud one.

**G1 — exhaustiveness.** A stage 1 check fails if any tracked file is classified by no tier, naming the file. Adding `docker-compose/foundationdb/docker-compose.yml` fails CI until someone assigns it. The converse holds too: a tier-3 rule matching no existing file fails, so a refactor cannot leave a dead glob behind. This is the guard a plain `paths:` filter lacks, and the reason a declared tier is tolerable at all.

**G2 — staleness disables selection rather than corrupting it.** On `main`, the engine compares each leg's freshly measured package set against the recorded one. A set that *grew* means the map under-claims — the unsafe direction — so the run opens or updates a bot pull request with the regenerated manifest and marks selection disabled until it lands. Pull requests fall back to full CI in the meantime. The map cannot be quietly wrong, only temporarily unused.

**G3 — backtest before enforcement.** For every merged pull request, the diff and the pass/fail result of each job are recorded and queryable. Replaying the engine over the last several hundred answers the question that matters — how many times would this selection have skipped a job that actually failed — before it is allowed to skip anything. A non-zero miss rate is a defect to diagnose, not a statistic to accept, and the backtest is re-runnable whenever the engine or manifest changes.

**G4 — shadow mode.** Before enforcement the `relevance` job publishes its selection while everything still runs, so disagreements are observable on real traffic. The same instrumentation remains afterward for auditing a specific pull request.

**G5 — the merge queue is the sound gate,** per §2.1. The precondition is that pull requests actually merge through the queue; the administrator bypasses forced by the coverage freeze that [RFC 0013](./0013-coverage-gating-and-e2e-coverage.md) M1 has now fixed were an anomaly, and if bypassing became routine the backstop would weaken to post-merge detection on `main`.

**G6 — explainability and override.** The `relevance` job writes a step-summary table — every job key, ran or skipped, and the rule that decided it — and a `ci:full` label forces a full run, mirroring the existing `ci:parallel`.

### 2.5 How it plugs into the orchestrator

A new `relevance` job in [`ci-orchestrator.yml`](../../.github/workflows/ci-orchestrator.yml), alongside `setup`, emits a JSON array of enabled job keys. Stage 2 and stage 3 gain a `workflow_call` input carrying it, and each call site in [`ci-orchestrator-stage3.yml`](../../.github/workflows/ci-orchestrator-stage3.yml) and [`ci-e2e-all.yml`](../../.github/workflows/ci-e2e-all.yml) gains `if: contains(fromJSON(inputs.selection), '<key>')`. Keys are per leg and per matrix dimension — `e2e-cassandra`, `e2e-cassandra:5.x`, `docker-all-in-one` — so one mechanism serves both leg selection and matrix narrowing. Nothing else changes: the trust-based sequential/parallel split, the stage boundaries, `ci-success` as the single required check, and the workflows' internal structure all stay as they are.

The engine is a Node module in [`.github/scripts/`](../../.github/scripts/), the established home for tested CI logic — it has a `package.json`, a Jest suite, a dedicated fast `ci-scripts` job, and the precedent of [`ci-summary-report-publish.js`](../../.github/scripts/ci-summary-report-publish.js) being consumed by a workflow. Logic that gates the entire suite should itself be unit tested, and this is the only place in the repository where that is already a convention. Its inputs are the merge-base diff, `go list -deps -json ./...` run in the job (seconds with a warm module cache, and accurate for the pull request's own imports rather than a snapshot of `main`'s), and the manifest.

---

## 3. Constraints and impact

### 3.1 Three required checks depend on which jobs ran

This is the sharp edge, and it means selection cannot be switched on before the work in M3.

| Check | Why a partial run breaks it | Resolution |
| --- | --- | --- |
| `codecov/patch`, `codecov/project` | withheld until `after_n_builds: 12` uploads arrive, and 11 of the 12 come from cells inside the `ci-e2e-*` workflows | `codecov.notify.manual_trigger: true` plus `codecovcli send-notifications` from the fan-in |
| `Coverage Gate` | compares the merged total against a `main` baseline; a partial run lowers the total by construction | `carryforward: true` per flag, and diff-level gating ([RFC 0013](./0013-coverage-gating-and-e2e-coverage.md) M6) |
| `Metrics Comparison` | consumes `.metrics/metrics_snapshot_<storage>.txt` files that e2e legs produce | report absent snapshots as deselected, not as regressions |

Deselecting any e2e leg today drops the upload count below the threshold and Codecov posts nothing — reproducing the five-day merge freeze [RFC 0013 §1.1](./0013-coverage-gating-and-e2e-coverage.md) documents. Replacing `after_n_builds` with an explicit completion signal from [`ci-summary-report.yml`](../../.github/workflows/ci-summary-report.yml) — the only job that sees the whole run — removes the hand-maintained mirror of the job count that RFC 0013 identifies as the freeze's root cause, so it is worth doing on its own merits. `carryforward` is Codecov's mechanism for exactly this situation: a flag receiving no upload carries its coverage forward from the base, keeping the merged total comparable.

### 3.2 Structural facts the design has to accommodate

**The e2e workflows cannot use `paths:` at all.** All 12 `ci-e2e-*.y*ml` files are `on: workflow_call` only, fanned out by [`ci-e2e-all.yml`](../../.github/workflows/ci-e2e-all.yml). Event-level path filtering is unavailable by construction, so the decision must be a job output — which is the desired shape anyway, since it puts the logic in one place instead of 12.

**The single required check already tolerates skips.** Branch protection requires six contexts — `All CI Checks Passed`, `Coverage Gate`, `Metrics Comparison`, `codecov/patch`, `codecov/project`, `check-label` — and the first is [`ci-success`](../../.github/workflows/ci-orchestrator.yml), a gatekeeper with `if: always()` that inspects `needs.*.result`. GitHub's well-known trap, where a required check attached to a path-filtered workflow stays `Expected — Waiting for status to be reported` forever, does not apply, because no required context hangs off a skippable workflow. Teaching `ci-success` to accept `skipped` for deliberately deselected jobs modifies logic that already exists.

### 3.3 Storage and quota impact

Essentially none, because the map is derived from artifacts CI already uploads: the fan-in job already downloads every `coverage-*` artifact to compute `Coverage Gate`, and extracting the set of packages with a non-zero counter is a read of data it is already holding. The only new persisted object is the tier-2 JSON, whose hard upper bound is small — the module's 242 import paths total 16 KB of text, so even the absurd case of all 31 cells executing all 242 packages is ~490 KB, and the realistic figure is a file well under 200 KB that changes only when code moves between packages.

For scale, a full run currently stores 54 artifacts totalling 1 MB with 7-day retention (coverage profiles are the bulk, 80–171 KB each). The repository is public, so Actions minutes are free and unmetered and artifact storage is not separately charged; were the manifest kept in an Actions cache instead (§5), the relevant limit is 10 GB per repository, against which 200 KB alongside the existing `coverage-baseline_` entry is noise. The net direction is downward: fewer jobs per pull-request run means fewer coverage profiles and metrics snapshots uploaded. The one thing that does grow artifact volume is not this RFC — [RFC 0013](./0013-coverage-gating-and-e2e-coverage.md) M5 replaces 15 near-empty profiles with real ones, plausibly taking a run from ~1 MB to ~2 MB.

---

## 4. Implementation plan

### M1 — Advisory relevance job ⬜

Add the `relevance` job and the Node engine with its Jest suite, computing and publishing a selection that nothing consumes (G4), plus the backtest harness (G3) replaying the engine over merged pull requests using their diffs and recorded job outcomes. The deliverable is evidence, not behavior change, and the miss rate it produces is the input to every later decision here.

### M2 — Sound wins with no engine ⬜

Two changes that need no manifest and no map, both sound by construction and independently revertible:

- **Documentation-only changes.** A pull request touching only `**/*.md`, `docs/**` (excluding embedded assets), or `.github/ISSUE_TEMPLATE/**` cannot affect a compiled artifact. Today it costs 311 machine-minutes.
- **Version-matrix narrowing on `pull_request`.** Run the newest Cassandra, Elasticsearch and OpenSearch on pull requests and the full matrix in the merge queue and on `main` — the identical trade `ci-docker-build.yml` already makes for platforms, applied to the 16 cells and 131 machine-minutes §1.1 identifies as the largest concentration of cost.

Together these recover ~42% of the run. Requires `ci-success` to accept `skipped`, and the M3 items for the documentation-only case, since it deselects coverage-uploading jobs.

### M3 — Make the required checks tolerate a partial run ⬜

The three resolutions in §3.1. Ordered before any selection that touches a coverage-uploading job. Coordinate with [RFC 0013](./0013-coverage-gating-and-e2e-coverage.md) M2, whose upload invariant is stated in terms of `after_n_builds` and will need restating in terms of the explicit completion signal.

### M4 — Manifest tiers 0/1/3 and the exhaustiveness guard ⬜

The manifest file, the substrate rule, build-graph inversion including `EmbedFiles`, the generated-then-verified tier-3 rules, and G1 as a stage 1 check. At the end of this milestone the engine classifies every file in the repository, and selection is still advisory.

### M5 — Tier 2: measured executed-package sets ⬜

Extract per-leg package sets from the `coverage-*` artifacts of `main` runs, starting with the 11 `direct` legs whose profiles are already real, extending to the spawned-binary legs once [RFC 0013](./0013-coverage-gating-and-e2e-coverage.md) M3–M5 lands and noting §2.2's independence from its materiality verdict. Implement G2 so a grown set opens a refresh pull request and disables selection rather than silently under-claiming.

### M6 — Enforce ⬜

Consume the selection at the `ci-e2e-all.yml` and `ci-orchestrator-stage3.yml` call sites for `pull_request` events only, gated on a clean backtest (G3) and a shadow period (G4) with no disagreement that would have hidden a failure. Ship G6 in the same milestone — the override has to exist before the first contributor needs it.

### M7 — Extend to image builds and static analysis ⬜

Bring `docker-build`, `docker-all-in-one`, `docker-hotrod`, `ocb-build`, `build-binaries`, CodeQL and FOSSA under the same selection, which is the request in [#3113](https://github.com/jaegertracing/jaeger/issues/3113). Last because these are 65 machine-minutes against e2e's 212, and because image builds feed the `spm` and `ui-reverse-proxy` legs, entangling their selection with the tier-2 gap in §2.3.

### M8 — Record the outcome ⬜

Capture the resulting arrangement — the tiers, the guards, where the decision is computed, and what stays unconditional — as an ADR, and mark this RFC Implemented with a pointer to it.

---

## 5. Open questions

- **M2 may be most of the win.** If matrix narrowing and the documentation-only skip recover 42% and M1's backtest shows a low ceiling beyond that, the honest conclusion is to stop after M4. The backtest should estimate that ceiling before M5's cost is committed.
- **Whether `spm` justifies closing the tier-2 gap.** Instrumenting a binary inside `docker-compose` and shutting the stack down gracefully is real work for 21 machine-minutes; leaving `spm` permanently on tier-3 rules may be the better trade.
- **Where the tier-2 manifest lives.** A checked-in JSON is reviewable but produces bot pull requests; an Actions cache keyed like the existing `coverage-baseline_` entry produces none but is invisible. This RFC recommends the checked-in file for reviewability, and the choice is reversible.
- **Whether selection should reach the sequential path first.** External contributors get both the slowest path and the full suite. Applying selection there first helps where the 31-minute wall clock hurts most, and is also where the trust argument is weakest — a policy call rather than a technical one.

---

## 6. Alternatives considered

| Criteria | A. Status quo | B. Declared path filters | C. Static Go graph | D. Measured execution map | E. Defer expensive jobs to merge queue | F. Bazel / Pants |
| --- | --- | --- | --- | --- | --- | --- |
| Author sees relevant failures before review | 🟢 | 🔴 <sup>1</sup> | 🟢 | 🟢 | 🔴 <sup>4</sup> | 🟢 |
| Cannot weaken the merge gate | 🟢 | 🟡 <sup>2</sup> | 🟡 <sup>2</sup> | 🟢 <sup>3</sup> | 🟢 | 🟢 |
| Detects its own drift | 🟢 | 🔴 | 🟢 | 🟢 | 🟢 | 🟢 |
| Separates legs sharing one binary | — | 🟡 <sup>5</sup> | 🔴 <sup>6</sup> | 🟢 | — | 🟢 <sup>7</sup> |
| Covers non-Go inputs | — | 🟢 | 🟡 <sup>8</sup> | 🟡 <sup>9</sup> | 🟢 | 🟢 |
| PR wall-clock reduction | 🔴 | 🟢 | 🔴 <sup>6</sup> | 🟢 | 🟢 | 🟢 |
| Machine-minute reduction | 🔴 | 🟢 | 🔴 <sup>6</sup> | 🟢 | 🟡 <sup>10</sup> | 🟢 |
| No build-system change | 🟢 | 🟢 | 🟢 | 🟡 <sup>11</sup> | 🟢 | 🔴 |
| Ongoing maintenance | 🟢 | 🔴 | 🟢 | 🟡 <sup>12</sup> | 🟢 | 🔴 |
| Decision explainable to a contributor | 🟢 | 🟢 | 🟢 | 🟢 <sup>13</sup> | 🟢 | 🟡 |

🟢 good · 🟡 partial or caveated · 🔴 poor · — not applicable

<sup>1</sup> §6.1. <sup>2</sup> Nothing prevents the same filters applying in the merge queue, and the temptation to apply them there is what makes the unsoundness reach `main`. <sup>3</sup> By construction: selection is restricted to `pull_request` events (§2.1). <sup>4</sup> Storage failures surface after review; for outside contributors, a rejected queue entry instead of a red check. <sup>5</sup> Achievable by hand, but with no basis beyond someone's belief. <sup>6</sup> §2.2: `cmd/jaeger`'s closure is 170 of 242 packages. <sup>7</sup> Requires per-backend test targets and hermetic backend stacks, which is most of the migration cost. <sup>8</sup> `//go:embed` assets only. <sup>9</sup> Tier 3 remains declared, held exhaustive by G1. <sup>10</sup> The merge queue still runs everything, and a rejection re-runs it. <sup>11</sup> Depends on [RFC 0013](./0013-coverage-gating-and-e2e-coverage.md)'s `-cover` instrumentation. <sup>12</sup> Tiers 0–2 are derived; tier 3 is 68 guarded files. <sup>13</sup> Via G6's step-summary table.

**D, restricted to `pull_request` events, keeping E's guarantee as the backstop.** D is the only option that both discriminates between legs linking the same binary and reports when its own map has gone stale. E is not adopted as a strategy — deferring all storage validation past review is a real cost to contributors and reviewers — but its guarantee is retained wholesale.

### 6.1 Why declared path filters are not acceptable (option B)

The standard GitHub Actions answer is a `paths:` filter or `dorny/paths-filter` feeding job-level `if:`, which here means a hand-written path list per e2e leg. The objection is not that such lists are hard to write but that **nothing ever checks them, and their failure mode is a green check on a job that did not run**. Three ways they rot:

- **Import creep.** A rule enumerating `internal/storage/v1/cassandra/**` is correct until the Cassandra writer starts calling a new shared helper in `internal/jptrace`. That helper is now load-bearing for the Cassandra leg and appears in no rule. Nothing fails; the leg stops running for changes that break it.
- **Silent moves.** Refactoring `internal/storage/v2/clickhouse/sql` elsewhere leaves the old glob matching nothing, and a glob matching nothing is indistinguishable in CI output from a glob matching nothing *relevant*.
- **Runtime coupling no glob expresses.** Which code an e2e run touches depends on `cmd/jaeger/config-cassandra.yaml`, on `STORAGE`, and on OTel collector component registration resolved at startup — none of it visible in an import graph, let alone a path list.

G1 (§2.4) is what separates tier 3 from this option: a declared rule set that must classify every tracked file, and fails CI when it does not, cannot rot silently in any of these three ways.

### 6.2 Why not a build system that does this natively (option F)

Bazel or Pants would make change analysis correct by construction. The cost is a build-system migration that would dominate every other engineering effort in the project, and it would still require per-backend test targets and hermetic backend stacks to distinguish the e2e legs. D reaches most of the same outcome using coverage instrumentation already being built for a different reason.

---

## 7. References

- [#3113](https://github.com/jaegertracing/jaeger/issues/3113) — Rethink our CI to avoid building all the docker images every time
- [#1476](https://github.com/jaegertracing/jaeger/issues/1476) — Optimize CI
- [#1784](https://github.com/jaegertracing/jaeger/issues/1784) — Elasticsearch integration tests started taking too long
- [RFC 0013](./0013-coverage-gating-and-e2e-coverage.md) — Reliable Coverage Gating and Real E2E Coverage
- [ADR-004](../adr/004-migrating-coverage-gating-to-github-actions.md) — Migrating Coverage Gating to GitHub Actions
- [`.github/workflows/README.md`](../../.github/workflows/README.md) — the staged orchestrator architecture
- [Azure DevOps Test Impact Analysis](https://learn.microsoft.com/en-us/azure/devops/pipelines/test/test-impact-analysis?view=azure-devops) — selection from a dynamic dependency map built during test execution
- [Develocity Predictive Test Selection](https://docs.gradle.com/develocity/2026.1/using-develocity/predictive-test-selection/) — the model-trained alternative to a measured map
- [Codecov `manual_trigger` and `send-notifications`](https://docs.codecov.com/docs/notifications) — explicit report completion instead of counting uploads
- [GitHub Actions billing](https://docs.github.com/en/billing/concepts/product-billing/github-actions) — free minutes and artifact storage for public repositories
- [Troubleshooting required status checks](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/collaborating-on-repositories-with-code-quality-features/troubleshooting-required-status-checks) — why a path-filtered required check hangs, and why `ci-success` avoids it
