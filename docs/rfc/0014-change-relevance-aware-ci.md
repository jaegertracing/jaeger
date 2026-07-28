# RFC 0014: Change-Relevance-Aware CI

- **Status:** Draft
- **Author:** Yuri Shkuro
- **Created:** 2026-07-28
- **Last Updated:** 2026-07-28
- **Issue:** [#3113](https://github.com/jaegertracing/jaeger/issues/3113)
- **Related:** [#1476](https://github.com/jaegertracing/jaeger/issues/1476) · [#1784](https://github.com/jaegertracing/jaeger/issues/1784) · [RFC 0013 Reliable Coverage Gating and Real E2E Coverage](./0013-coverage-gating-and-e2e-coverage.md) (in review as [#9131](https://github.com/jaegertracing/jaeger/pull/9131)) · [ADR-004 Migrating Coverage Gating to GitHub Actions](../adr/004-migrating-coverage-gating-to-github-actions.md)

---

## Implementation status

| Milestone | Scope | Status |
| --- | --- | --- |
| M1 | Advisory relevance job: compute and report a selection, change nothing | ⬜ |
| M2 | Sound wins with no relevance engine: docs-only skip, PR-event matrix narrowing | ⬜ |
| M3 | Make the required checks tolerate a partial run | ⬜ |
| M4 | Relevance manifest tiers 0/1/3 plus the exhaustiveness guard | ⬜ |
| M5 | Tier 2: per-leg executed-package sets measured from `covdata` | ⬜ |
| M6 | Backtest against merged history, then enforce selection on PR events | ⬜ |
| M7 | Extend selection to image builds and static analysis | ⬜ |
| M8 | Record the outcome as an ADR | ⬜ |

---

## Abstract

Jaeger CI is all-or-nothing. A pull request that edits only `ci-lint-checks.yaml`, or only a `README`, runs the same 65 jobs and 311 machine-minutes as one that rewrites the Cassandra span writer, and its author waits the same 15 minutes (31 minutes on the sequential path) for a verdict. 68% of that cost is the 31 e2e matrix cells, which also own the critical path, so the waiting is not incidental — it is the e2e suite validating storage backends the change cannot reach.

The obvious fix is to attach `paths:` filters to jobs. That fix is unreliable in a specific and unacceptable way: a hand-written path list is a claim about which code a job exercises, nothing checks the claim, and when it is wrong CI reports success for a job it never ran. The failure is silent, and it degrades in exactly the direction that matters — a rule written when a package had one caller keeps passing after the package acquires five.

This RFC proposes to keep the claim but stop writing it by hand. Relevance is computed from three mechanically derived inputs — the Go build graph including `//go:embed` assets, the set of packages each CI job is *observed to execute*, and a fail-closed substrate rule that routes any change to the build or CI machinery to a full run — with the residue of runtime-only inputs (storage `docker-compose` files, `cmd/jaeger/config-*.yaml`, `scripts/e2e/*.sh`) as the one declared tier, held honest by a guard that fails CI on any tracked file no rule classifies. The Go build graph alone is not sufficient and the RFC shows why with a measurement: `cmd/jaeger`'s dependency closure contains 170 of the repository's 242 packages, because the single binary links every storage backend, so static analysis cannot tell the Cassandra leg from the ClickHouse one. Runtime observation can, and [RFC 0013](./0013-coverage-gating-and-e2e-coverage.md)'s `-cover` + `GOCOVERDIR` work produces exactly the per-leg package sets required — needing only which packages executed, not whether their coverage was material.

Reliability comes from four properties rather than from trusting the predictor: selection applies to `pull_request` events only, so the merge queue and `main` always run the full suite and no skip can gate a merge; the map's staleness disables selection instead of corrupting it; the decision is backtested against the recorded outcomes of merged pull requests before it is allowed to skip anything; and every run prints why each job ran, with a `ci:full` label to override. The recommended sequencing puts the changes that need no relevance engine at all — skipping the expensive suite on documentation-only changes, and narrowing the Cassandra/Elasticsearch/OpenSearch version matrices on pull-request events while the merge queue keeps the full matrix — ahead of the engine, because they are sound by construction and recover 42% of the run on their own.

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

Wall-clock is 13–15 minutes on the parallel path and ~31 minutes on the sequential path that external contributors get. On the parallel path the critical path is an e2e cell: the four `cassandra … e2e` cells run 10 minutes each, longer than any other job once `docker-build` narrows itself to `linux/amd64` for pull requests ([`ci-docker-build.yml:41-44`](../../.github/workflows/ci-docker-build.yml)). So e2e is not merely the bulk of the cost, it is also what the author is waiting for.

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

There is a second multiplier. Over the last 100 orchestrator runs, 87 were `pull_request` and 13 were `push` to `main`: roughly seven pull-request runs per merge, since every push to a branch under review re-runs everything. Pull-request CI, not merge CI, is where the machine time goes.

### 1.2 The staged DAG addresses a different problem

[`ci-orchestrator.yml`](../../.github/workflows/ci-orchestrator.yml) already has two axes of adaptation, and neither is about the content of the change. The three-stage sequential path exists to fail fast on *cheap* checks before spending on expensive ones, and the parallel path exists to shorten feedback for *trusted actors*. Both axes decide **when** work runs and **who** gets it early. Neither ever decides that a job is inapplicable. A linter-only change traverses all three stages, or all three at once, and either way runs every Cassandra cell.

### 1.3 Why the obvious fix is not acceptable

The standard GitHub Actions answer is a `paths:` filter per workflow, or `dorny/paths-filter` feeding job-level `if:` conditions. Applied here it would mean, for each of the 12 e2e legs, a hand-written list of the paths that leg cares about.

Such a list is an assertion — "the Cassandra leg exercises only this code" — and the reason to reject it is not that it is hard to write but that **nothing ever checks it, and its failure mode is a green check on a job that did not run**. Three specific ways it rots:

- **Import creep.** A rule enumerating `internal/storage/v1/cassandra/**` is correct until the Cassandra writer starts calling a new shared helper in `internal/jptrace`. The helper's package is now load-bearing for the Cassandra leg and appears in no rule. Nothing fails; the leg simply stops running for changes that break it.
- **Silent moves.** Refactoring `internal/storage/v2/clickhouse/sql` into a new location leaves the old glob matching nothing. A glob that matches nothing is indistinguishable, in CI output, from a glob that matches nothing *relevant*.
- **Runtime coupling that no glob expresses.** The e2e harness spawns a compiled binary against a YAML config ([`e2e_integration.go`](../../cmd/jaeger/internal/integration/e2e_integration.go)); which code the run touches depends on `cmd/jaeger/config-cassandra.yaml`, on the `STORAGE` environment variable, and on OTel collector component registration resolved at startup. None of that is visible in an import graph, let alone in a path list.

The requirement this RFC takes from that analysis: **a relevance claim must be derived from something the repository can recompute, and any claim that cannot be recomputed must fail closed.**

---

## 2. Current state

### 2.1 What CI already narrows, and on what basis

Three precedents exist, and they establish that narrowing per event type is already accepted practice:

| Mechanism | Where | Basis |
| --- | --- | --- |
| `linux/amd64` only, no debug images | [`ci-docker-build.yml:41-48`](../../.github/workflows/ci-docker-build.yml) | event type: `pull_request` / `merge_group` vs. push to `main` |
| Sequential vs. parallel stages | [`ci-orchestrator.yml:25-122`](../../.github/workflows/ci-orchestrator.yml) | actor trust |
| Whole-workflow `paths:` filter | [`ci-unit-tests-go-tip.yml:14-18`](../../.github/workflows/ci-unit-tests-go-tip.yml) | changed paths |

The third is the only path-based conditional in the repository, and it guards a non-required workflow with a two-entry list naming the workflow itself. There is no `paths-ignore:`, no `dorny/paths-filter`, no `tj-actions/changed-files`, and no merge-base diff anywhere under `.github/`. The two `git diff` invocations in [`ci-lint-checks.yaml:100-103`](../../.github/workflows/ci-lint-checks.yaml) are generated-file drift checks, not relevance.

### 2.2 Structural constraints on any selection scheme

Four properties of the existing arrangement constrain the design, and one of them is a gift.

**The gift: a single required check that already tolerates skips.** Branch protection on `main` requires six contexts — `All CI Checks Passed`, `Coverage Gate`, `Metrics Comparison`, `codecov/patch`, `codecov/project`, `check-label`. The first is [`ci-success`](../../.github/workflows/ci-orchestrator.yml), a gatekeeper job with `if: always()` that inspects `needs.*.result` and already branches on which path ran. GitHub's well-known trap — a required check attached to a workflow that path filters skipped stays `Expected — Waiting for status to be reported` forever — does not apply, because no required context is attached to a skippable workflow. Extending `ci-success` to treat `skipped` as acceptable for deliberately deselected jobs is a change to logic that exists rather than a new pattern.

**Constraint 1: the e2e workflows cannot use `paths:` at all.** All 12 `ci-e2e-*.y*ml` files are `on: workflow_call` only; they are fanned out by [`ci-e2e-all.yml`](../../.github/workflows/ci-e2e-all.yml) from [`ci-orchestrator-stage3.yml`](../../.github/workflows/ci-orchestrator-stage3.yml). Event-level path filtering is unavailable by construction, so the selection must be a job output consumed as `if:` at the call sites — which is the desired shape anyway, since it puts the decision in one place instead of 12.

**Constraint 2: three of the six required checks depend on which jobs ran.** This is the sharp edge.

- `codecov/patch` and `codecov/project` are withheld until `after_n_builds: 12` uploads arrive ([`.codecov.yml`](../../.codecov.yml)). Eleven of those 12 uploads come from e2e legs. Deselecting any e2e leg drops the count below the threshold and Codecov posts nothing — reproducing exactly the five-day merge freeze that [RFC 0013 §1.1](./0013-coverage-gating-and-e2e-coverage.md) documents.
- `Coverage Gate` compares the merged total against a baseline cached from `main` ([`ci-summary-report.yml:89-131`](../../.github/workflows/ci-summary-report.yml)). A partial run produces a lower total by construction, so the no-regression comparison fails on every selective run.
- `Metrics Comparison` consumes `.metrics/metrics_snapshot_<storage>.txt` files that the e2e legs produce via [`verify-metrics-snapshot`](../../.github/actions/verify-metrics-snapshot/action.yaml). Skipped legs contribute no snapshot.

None of these is fatal, and §4.5 resolves each, but they mean selection cannot be switched on ahead of that work.

**Constraint 3: two legs run no Go test.** `spm` (4 cells, 21 minutes) drives `docker-compose/monitor` with `curl` and `jq` assertions from [`scripts/e2e/spm.sh`](../../scripts/e2e/spm.sh); `ui-reverse-proxy` does the same via [`scripts/e2e/ui-reverse-proxy.sh`](../../scripts/e2e/ui-reverse-proxy.sh). Any scheme that derives relevance from Go test execution has nothing to say about them without extra work (§4.2, tier 2b).

---

## 3. What "relevance" has to mean

### 3.1 The asymmetry that sets the standard

A selection can be wrong in two directions with wildly different costs. Running a job that the change could not have affected wastes minutes. *Not* running a job that the change breaks converts CI from a gate into a source of false confidence — and the failure surfaces later, in someone else's pull request, with the cause several merges back.

So the target is not accuracy, it is **conservatism with an audit trail**: every uncertainty resolves toward running the job, and every claim that a job is unnecessary traces to something a script recomputed. Two consequences follow immediately.

First, **selection is a latency optimization for pull requests and never a merge gate.** It applies to `pull_request` events only. `merge_group` and `push` to `main` continue to run the full suite, so the code that lands on `main` has always been validated completely, and the worst outcome of a bad selection is a merge-queue rejection rather than a defect on `main`. This single restriction converts an unsound predictor into a bounded latency risk, and it is the reason the rest of the design is defensible.

Second, **the cheap checks stay unconditional.** Selection targets the 31 e2e cells and the image builds. The stage 1 lint checks (13 minutes across 8 jobs), Go unit tests (`make test`, 8 minutes, whole-module and the backbone of the coverage report), the CI scripts Jest suite, and DCO/label checks are not worth reasoning about: they are cheap, they are global, and gating them buys minutes while adding a class of mistake.

### 3.2 The Go build graph is necessary and not sufficient

The mechanical starting point is the build graph. `go list -deps` gives each package's transitive imports; inverting it gives, for a set of changed packages, every package whose build could be affected. `go list -json` additionally reports `EmbedFiles`, which is how non-Go assets attach to the graph for free: `internal/storage/v1/cassandra/schema` reports `v004-go-tmpl.cql.tmpl`, so the Cassandra schema template is attributed to a package without anyone writing a rule. The same holds for `internal/storage/elasticsearch/esclient/index_templates/*.json` and the 13 ClickHouse DDL files under `internal/storage/v2/clickhouse/sql`.

Where it breaks is the unified binary. The e2e harness spawns `./cmd/jaeger/jaeger` ([`e2e_integration.go:105`](../../cmd/jaeger/internal/integration/e2e_integration.go)), and that binary registers every storage backend:

```
$ go list -deps ./cmd/jaeger | grep -c jaegertracing/jaeger
170          # out of 242 packages in the module
```

`internal/storage/v2/clickhouse/sql` is in the closure of the binary that the *Cassandra* leg runs, because the Cassandra leg's binary links ClickHouse whether or not it ever calls it. Build-graph reachability therefore marks 70% of the module as relevant to all 12 e2e legs simultaneously, and a purely static scheme would skip almost nothing for almost any Go change. The graph answers "could this change alter the bytes of the binary this leg runs" — a sound question, and the wrong one.

### 3.3 Execution is the discriminating signal

The question that discriminates is "which packages does this leg actually run", and it is answerable by measurement rather than inference. This is the mechanism behind Azure DevOps' Test Impact Analysis, which builds a dynamic dependency map from code exercised during test execution and selects on it; the alternative industrial approach, Develocity's Predictive Test Selection, trains a model on historical change/outcome pairs. The measurement approach is the right one here because the map it produces is inspectable and its errors are attributable, whereas a model's are not.

Jaeger is unusually well positioned to measure it. The 11 `direct` legs already run storage code in-process under `-coverpkg=./...` and produce real profiles today ([RFC 0013 §2.1](./0013-coverage-gating-and-e2e-coverage.md)), so their executed-package sets are available from artifacts the run already uploads. The spawned-binary legs produce nothing usable today, and [RFC 0013](./0013-coverage-gating-and-e2e-coverage.md) M3–M5 is exactly the work that fixes that: `go build -cover`, a `GOCOVERDIR`, and `go tool covdata` after a graceful shutdown.

One point of independence is worth stating plainly, because it decouples this RFC from RFC 0013's decision gate. RFC 0013 M4 gates its rollout on whether binary coverage adds *material* coverage. A relevance map needs only the **set of packages with a non-zero counter**, not the magnitude. A leg whose new coverage is 0.2 percentage points still names precisely the packages it executed, which is the whole input required here. If RFC 0013 M4 concludes the coverage is immaterial for gating, the instrumentation remains worth keeping for this purpose, and this RFC's M5 should say so rather than inheriting the negative verdict.

### 3.4 The residue

What no coverage tool sees, and what therefore has to be handled another way:

- **Runtime configuration.** `cmd/jaeger/config-*.yaml` (one or more per leg), consumed by the spawned binary as a command-line argument.
- **Backend stacks.** `docker-compose/{cassandra,elasticsearch,opensearch,clickhouse,kafka}/**` and `docker-compose/monitor/**`.
- **The harness scripts.** `scripts/e2e/*.sh`, plus `scripts/makefiles/IntegrationTests.mk`.
- **The two Go-test-free legs** from §2.2.

This residue is 68 files: 24 `cmd/jaeger/config-*.y*ml`, 36 under `docker-compose/`, and 8 `scripts/e2e/*.sh`. It is small, it changes rarely, and — unlike the Go graph — it has no mechanical owner. It is the one part of the design that is declared, and §4.3's guards exist mostly to keep it from becoming the hand-written path list that §1.3 rejects.

---

## 4. Design

### 4.1 Shape

A new `relevance` job in [`ci-orchestrator.yml`](../../.github/workflows/ci-orchestrator.yml), alongside the existing `setup`, computes a JSON array of enabled job keys and exposes it as an output. `stage2`/`stage3` gain a `workflow_call` input carrying that array, and each call site in [`ci-orchestrator-stage3.yml`](../../.github/workflows/ci-orchestrator-stage3.yml) and [`ci-e2e-all.yml`](../../.github/workflows/ci-e2e-all.yml) gains `if: contains(fromJSON(inputs.selection), '<key>')`. Job keys are per leg and per matrix dimension — `e2e-cassandra`, `e2e-cassandra:5.x`, `docker-all-in-one` — so the same mechanism serves both leg selection and matrix narrowing.

The engine is a Node module in [`.github/scripts/`](../../.github/scripts/), which is the established home for tested CI logic: it has a `package.json`, a Jest suite, a dedicated fast `ci-scripts` job, and the precedent of [`ci-summary-report-publish.js`](../../.github/scripts/ci-summary-report-publish.js) being consumed by a workflow. Decision logic that gates the entire suite must itself be unit tested, and this is the only place in the repository where that is already a convention.

The engine's inputs are the merge-base diff, `go list -deps -json ./...` run in the job (a few seconds with a warm module cache, and always accurate for the pull request's own imports rather than a snapshot of `main`'s), and the checked-in manifest.

### 4.2 The manifest, in four tiers

Every tracked path resolves through exactly one tier, tried in order.

**Tier 0 — substrate, fail closed.** Any change to `.github/**`, `Makefile`, `scripts/makefiles/**`, `go.mod`, `go.sum`, `Dockerfile*`, `.codecov.yml`, or the `idl` / `jaeger-ui` submodule pointers disables selection entirely for that run. These are the inputs that can change the meaning of every job, including the relevance engine itself, and no analysis of them is worth attempting. This tier is deliberately broad: it makes "the CI change ran under the old CI rules" impossible.

**Tier 1 — Go packages and their embeds, derived.** Changed `.go` files map to their owning package; `EmbedFiles` maps embedded assets to the same package. The engine inverts the import graph to get the affected package set. Zero maintenance, recomputed every run.

**Tier 2a — executed-package sets per leg, measured.** For each job key, the set of module packages observed with a non-zero coverage counter on `main`. A leg runs iff its set intersects the tier-1 affected set. Regenerated by the `push`-to-`main` run from the `coverage-*` artifacts and `covdata` output that the run already produces; stored as a checked-in JSON so that a change in what a leg covers appears as a reviewable diff rather than as invisible cache state.

**Tier 2b — the Go-test-free legs.** `spm` and `ui-reverse-proxy` have no in-process profile. They run the same instrumented binary inside `docker-compose`, so the same measurement is available in principle by mounting a `GOCOVERDIR` volume and stopping the compose services gracefully. Until that exists these two legs are classified by tier 3 alone, which for `spm` means it runs on any change to the metrics pipeline, the four metricstore backends, or its compose stack — conservative, and it still skips `spm` for a pure Cassandra or Badger change.

**Tier 3 — runtime inputs, declared and guarded.** Explicit path-to-job-key rules for the §3.4 residue. Two properties keep this tier from rotting into §1.3's rejected design. It is **generated where it can be**: the config files a leg uses are named in that leg's test source or shell script, so a reference scan proposes the mapping and the guard verifies that every `cmd/jaeger/config-*.yaml` and every `docker-compose/**` file is claimed by at least one leg that references it. And it is **exhaustive by enforcement**, per the next section.

### 4.3 The guards

These are the substance of the proposal. The tiers decide; the guards are what make the decision trustworthy, and each one converts a class of silent error into a loud one.

**G1 — exhaustiveness.** A check in stage 1 fails if any tracked file is classified by no tier. Adding `docker-compose/foundationdb/docker-compose.yml` fails CI until someone assigns it, and the failure names the file. This is the guard that §1.3's design lacks and the reason a declared tier is tolerable at all. Its converse also holds: a tier-3 rule matching no existing file fails too, so silent moves cannot leave a dead glob behind.

**G2 — staleness disables selection, it does not corrupt it.** On `main`, the engine compares each leg's freshly measured package set against the recorded one. A set that *grew* means the map under-claims — the unsafe direction — so the run opens or updates a bot pull request with the regenerated manifest and marks selection **disabled** until it lands. Pull requests then fall back to full CI. This is the property that makes the map safe to rely on: it cannot be quietly wrong, only temporarily unused.

**G3 — backtest before enforcement.** The recorded outcomes of merged pull requests are queryable: for each, the diff and the pass/fail result of every job. Replaying the engine over the last several hundred merged pull requests answers the question that matters — *how many times would this selection have skipped a job that actually failed* — before the selection is allowed to skip anything. A non-zero miss rate is a defect to diagnose, not a statistic to accept, and this backtest is re-runnable whenever the manifest or the engine changes.

**G4 — shadow mode.** Before enforcement, the `relevance` job computes and publishes the selection while everything continues to run. Live disagreements are then observable on real traffic, and the same instrumentation stays afterward as the mechanism for auditing decisions on a specific pull request.

**G5 — the merge queue is the sound gate.** Restated as a guard because it is what bounds the damage: `merge_group` and `push` to `main` never select. The precondition is that pull requests actually merge through the queue; the administrator bypasses forced by the coverage freeze that [RFC 0013](./0013-coverage-gating-and-e2e-coverage.md) M1 has now fixed were an anomaly, and if bypassing became routine the backstop would weaken to post-merge detection on `main`.

**G6 — explainability and override.** The `relevance` job writes a step summary table — every job key, ran or skipped, and the rule that decided it — so a reviewer can see the reasoning without reading the engine. A `ci:full` label forces a full run, mirroring how `ci:parallel` already works.

### 4.4 Making the required checks tolerate a partial run

Each of §2.2's three problems has a mechanism already designed for it.

**Codecov's upload count.** Replace `after_n_builds` with `codecov.notify.manual_trigger: true` and a `codecovcli send-notifications` call from the fan-in. Instead of Codecov guessing that the report is complete by counting to 12, [`ci-summary-report.yml`](../../.github/workflows/ci-summary-report.yml) — which runs after `ci-success` and is the only job that sees the whole run — tells it. This removes the hand-maintained mirror of the job count that [RFC 0013 §1.3](./0013-coverage-gating-and-e2e-coverage.md) identifies as the root cause of the freeze, so it is worth doing on its own merits and is a prerequisite here rather than a workaround.

**The project-total comparison.** Mark each flag `carryforward: true`. Carryforward is Codecov's mechanism for precisely this situation: when a flag receives no upload for a commit, its coverage is carried forward from the base, so the merged total stays comparable and `Coverage Gate`'s baseline check remains meaningful. Diff-level gating is the more robust answer and is already specified as [RFC 0013](./0013-coverage-gating-and-e2e-coverage.md) M6: patch coverage asks whether *the changed lines* are covered, which is a well-posed question under selection as long as the jobs covering those lines ran — which is what the engine selects for.

**Metrics snapshots.** [`verify-metrics-snapshot`](../../.github/actions/verify-metrics-snapshot/action.yaml) compares per-storage snapshots; a deselected leg contributes none. The fan-in should compare the snapshots present and report the absent ones as deselected, not as regressions — the same treatment coverage flags get.

### 4.5 What this does not change

The trust-based sequential/parallel split, the stage boundaries, `ci-success` as the single required check, and the individual workflows' internal structure all stay as they are. Selection is an additional input to existing `if:` conditions.

---

## 5. Options considered

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

<sup>1</sup> A stale or wrong rule reports success for a job that never ran, with no signal. <sup>2</sup> Nothing prevents the same filters from applying in the merge queue, and the temptation to apply them there is what makes the unsoundness reach `main`. <sup>3</sup> By construction: selection is restricted to `pull_request` events (§3.1). <sup>4</sup> Storage failures surface at merge time, after review, which for outside contributors means a rejected queue entry instead of a red check. <sup>5</sup> Achievable by hand, but with no basis for the assignment beyond someone's belief. <sup>6</sup> `cmd/jaeger`'s closure is 170 of 242 packages (§3.2), so nearly every Go change marks every leg relevant. <sup>7</sup> Requires per-backend test targets and hermetic backend stacks, which is most of the migration cost. <sup>8</sup> `//go:embed` assets only. <sup>9</sup> Tier 3 remains declared, held to exhaustiveness by G1 (§4.3). <sup>10</sup> The merge queue still runs everything, and a queue rejection re-runs it. <sup>11</sup> Depends on [RFC 0013](./0013-coverage-gating-and-e2e-coverage.md)'s `-cover` instrumentation for the spawned binary. <sup>12</sup> Tiers 0–2 are derived; tier 3 is ~60 files and guarded. <sup>13</sup> Via G6's step-summary table.

**Recommendation: D, restricted to `pull_request` events, keeping E's guarantee as the backstop.** D is the only option that both discriminates between legs linking the same binary and reports when its own map has gone stale, and §3.1's restriction to pull-request events removes the soundness objection that would otherwise disqualify any predictor. E is not adopted as a strategy — deferring all storage validation past review is a real cost to contributors and to reviewers — but its guarantee is retained wholesale: the merge queue and `main` always run everything.

B is rejected for §1.3's reasons, C is rejected on the measurement in §3.2, and F is rejected on cost: a build-system migration to obtain change analysis would dominate every other engineering effort in the project, and D reaches most of the same outcome using coverage instrumentation that is already being built for a different reason.

The sequencing recommendation is separate from the option choice, and matters more for early value. Two changes need no relevance engine at all and are sound by construction:

- **Documentation-only changes.** A pull request touching only `**/*.md`, `docs/**` (excluding embedded assets), or `.github/ISSUE_TEMPLATE/**` cannot affect any compiled artifact. Today it costs 311 machine-minutes and 15 minutes of an author's attention.
- **Version-matrix narrowing on pull-request events.** Run one version per backend on pull requests — the newest Cassandra, Elasticsearch and OpenSearch — and the full matrix in the merge queue and on `main`. This is the identical trade `ci-docker-build.yml` already makes for platforms, applied to the 16 cells and 131 machine-minutes that §1.1 identifies as the largest concentration of cost, and it retires the cells that differ only by a backend version the change does not mention.

Together those recover roughly 42% of the run before any relevance is computed, which is why they are M2 rather than M6.

---

## 6. Implementation plan

### M1 — Advisory relevance job ⬜

Add the `relevance` job and the Node engine with its Jest suite, computing and publishing a selection that nothing consumes (G4). Land the backtest harness (G3) in the same milestone: a script that replays the engine over merged pull requests using their diffs and recorded job outcomes, reporting the miss rate. The deliverable is evidence, not behavior change, and the miss rate it produces is the input to every later decision here.

### M2 — Sound wins with no engine ⬜

Skip stages 2 and 3 for documentation-only pull requests, and narrow the Cassandra, Elasticsearch and OpenSearch version matrices on `pull_request` events while `merge_group` and `push` keep the full matrix. Both are event-and-path conditions that need no manifest and no map, and both are independently revertible. Requires `ci-success` to accept `skipped` for deselected jobs, and the M3 items for the documentation-only case, since it deselects coverage-uploading jobs.

### M3 — Make the required checks tolerate a partial run ⬜

`codecov.notify.manual_trigger: true` plus `codecovcli send-notifications` from the fan-in, replacing `after_n_builds`; `carryforward: true` on every flag; and snapshot-absence handled as deselection rather than regression in the metrics comparison. Ordered before any selection that touches a coverage-uploading job. Coordinate with [RFC 0013](./0013-coverage-gating-and-e2e-coverage.md) M2, whose upload invariant is stated in terms of `after_n_builds` and will need restating in terms of the explicit completion signal.

### M4 — Manifest tiers 0/1/3 and the exhaustiveness guard ⬜

The manifest file, the substrate rule, build-graph inversion including `EmbedFiles`, the generated-then-verified tier-3 rules, and G1 as a stage 1 check. At the end of this milestone the engine can classify every file in the repository, and selection is still advisory.

### M5 — Tier 2: measured executed-package sets ⬜

Extract per-leg package sets from the `coverage-*` artifacts of `main` runs, starting with the 11 `direct` legs whose profiles are already real. Extend to the spawned-binary legs once [RFC 0013](./0013-coverage-gating-and-e2e-coverage.md) M3–M5 lands, noting §3.3's independence: the map needs the package set regardless of RFC 0013 M4's materiality verdict. Implement G2 so a grown set opens a refresh pull request and disables selection rather than silently under-claiming.

### M6 — Enforce ⬜

Consume the selection at the `ci-e2e-all.yml` and `ci-orchestrator-stage3.yml` call sites for `pull_request` events only, gated on a clean backtest (G3) and a shadow period (G4) with no disagreements that would have hidden a failure. Ship G6's summary table and the `ci:full` label in the same milestone — the override has to exist before the first contributor needs it.

### M7 — Extend to image builds and static analysis ⬜

Bring `docker-build`, `docker-all-in-one`, `docker-hotrod`, `ocb-build`, `build-binaries`, CodeQL and FOSSA under the same selection, which is the request in [#3113](https://github.com/jaegertracing/jaeger/issues/3113). Sequenced last because these are 65 machine-minutes against e2e's 212, and because image builds feed the `spm` and `ui-reverse-proxy` legs, so their selection is entangled with tier 2b.

### M8 — Record the outcome ⬜

Capture the resulting arrangement — the tiers, the guards, where the decision is computed, and what remains unconditional — as an ADR, and mark this RFC Implemented with a pointer to it.

---

## 7. Open questions

- **Matrix narrowing without the engine may be most of the win.** If M2 recovers 42% of the run and the backtest shows a low ceiling on what tiers 1–3 add beyond it, the honest conclusion could be to stop after M4. M1's backtest should be able to estimate that ceiling before M5's cost is committed.
- **Whether `spm` justifies tier 2b.** Instrumenting a binary inside `docker-compose` and shutting the stack down gracefully is real work for 21 machine-minutes. Leaving `spm` permanently on tier-3 rules may be the better trade.
- **Where the observed-package manifest lives.** A checked-in JSON is reviewable but produces bot pull requests; an Actions cache keyed like the existing `coverage-baseline_` entry produces none but is invisible. This RFC recommends the checked-in file for reviewability, and the choice is reversible.
- **Whether a queue rejection is acceptable feedback.** Selection accepts that a badly selected pull request fails at merge-queue time. If that proves disruptive in practice, the response is to widen the selection, not to extend it into the queue.
- **Interaction with the sequential path.** External contributors get both the slowest path and, for now, the same full suite. Whether selection should apply there first — where the 31-minute wall clock hurts most — or last, where the trust argument is weakest, is a policy call rather than a technical one.

---

## 8. References

- [#3113](https://github.com/jaegertracing/jaeger/issues/3113) — Rethink our CI to avoid building all the docker images every time
- [#1476](https://github.com/jaegertracing/jaeger/issues/1476) — Optimize CI
- [#1784](https://github.com/jaegertracing/jaeger/issues/1784) — Elasticsearch integration tests started taking too long
- [RFC 0013](./0013-coverage-gating-and-e2e-coverage.md) — Reliable Coverage Gating and Real E2E Coverage
- [ADR-004](../adr/004-migrating-coverage-gating-to-github-actions.md) — Migrating Coverage Gating to GitHub Actions
- [`.github/workflows/README.md`](../../.github/workflows/README.md) — the staged orchestrator architecture
- [Azure DevOps Test Impact Analysis](https://learn.microsoft.com/en-us/azure/devops/pipelines/test/test-impact-analysis?view=azure-devops) — selection from a dynamic dependency map built during test execution
- [Develocity Predictive Test Selection](https://docs.gradle.com/develocity/2026.1/using-develocity/predictive-test-selection/) — the model-trained alternative to a measured map
- [Codecov `manual_trigger` and `send-notifications`](https://docs.codecov.com/docs/notifications) — explicit report completion instead of counting uploads
- [Troubleshooting required status checks](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/collaborating-on-repositories-with-code-quality-features/troubleshooting-required-status-checks) — why a path-filtered required check hangs, and why `ci-success` avoids it
