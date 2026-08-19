# Migrate Coverage Gating from Codecov to GitHub Actions

* **Status**: Accepted (implemented)
* **Date**: 2026-03-01, extended 2026-07-29 (binary coverage, upload-invariant check, Codecov follow-up resolved), extended 2026-08-19 (the in-run summary check is the required context)

## Context

Jaeger uses [Codecov](https://codecov.io) for two functions:

1. **Long-term trend tracking**: Coverage is uploaded after each CI run via the Codecov Action.
2. **PR gating**: Codecov's GitHub status check blocks merges when coverage drops below a threshold.

Coverage is collected across 11 CI jobs (unit tests + E2E), uploaded through `.github/actions/upload-codecov/action.yml`.

### Problem

Codecov's PR status checks suffer from latency (results lag behind CI completion) and intermittent rate-limit failures that block PRs even when coverage is healthy. The gating logic should run entirely within GitHub Actions for faster, more reliable feedback.

## Decision

Extend the existing `CI Summary Report` fan-in workflow to add coverage aggregation and gating alongside the existing metrics comparison. Codecov uploads are retained for long-term historical trending and per-flag breakdown views.

### Requirements

1. Coverage must be merged from all CI jobs (unit tests and E2E) into a single profile.
2. Two independent gates must be applied:
   - **Absolute floor**: total coverage ≥ 95%, matching the Codecov project target.
   - **No regression**: total coverage must not drop compared to the `main` baseline.
3. The merged profile must be filtered using the same exclusions as `.codecov.yml` (generated files, mocks, integration test infrastructure) so both tools report from a single source of truth.
4. A `Coverage Gate` check-run must always be posted to the PR — even when no coverage data is available — so it can be used as a required status check in branch protection.
5. The workflow must run for `pull_request`, `merge_group`, and `push` (to `main`) events triggered through the CI Orchestrator, as well as via manual `workflow_dispatch`.
6. On `main`-branch runs, the coverage baseline must be cached for future PR comparisons.

### Success Criteria

- `Coverage Gate` and `Metrics Comparison` check-runs appear on every PR and merge-queue run.
- Coverage regressions block PRs when `Coverage Gate` is added to required status checks.
- Manual re-runs via `workflow_dispatch` allow re-posting checks from any branch.

## Implementation Overview

### Coverage Artifact Pipeline

Each CI job uploads its coverage profile as a `coverage-<flag>` artifact (7-day retention) via `.github/actions/upload-codecov/action.yml`, alongside the existing Codecov upload.

The e2e legs additionally contribute coverage of the jaeger binary they spawn. Those legs run jaeger as a separate OS process and drive it over the wire, so `go test -coverpkg` in the test process cannot observe it — before this was added, their profiles covered only `cmd/jaeger/internal/integration`, which `.codecov.yml` ignores, so 15 of the 27 uploads contributed nothing countable. The binary is built with `go build -cover -covermode=atomic`, writes counters into a directory passed as `JAEGER_BINARY_COVERDIR` and mapped onto `GOCOVERDIR` for the child only, and `go tool covdata textfmt` plus `gocovmerge` merge the result into the leg's `cover.out` (`scripts/makefiles/IntegrationTests.mk`). Three constraints are load-bearing:

1. **The binary must exit normally.** Go flushes coverage counters from the runtime exit path, so `Binary.Stop` sends `SIGTERM` and escalates to `SIGKILL` only after a timeout. Under `SIGKILL` the directory gets a meta file and no counters.
2. **The destination cannot be named `GOCOVERDIR`.** Under `go test -coverprofile` the toolchain sets `GOCOVERDIR` in the test process for its own use, so an inherited value is overwritten and the binary's counters land where the build discards them.
3. **Both meta and counter files must exist before merging.** `covdata` converts a meta-only directory without error, emitting every instrumented statement at zero; merging that adds ~22k uncovered statements and collapses the reported total, so missing counters would surface as a coverage regression.

Beyond coverage, this is the only source of per-leg package attribution: the `cassandra` leg's profile names 13 cassandra-specific packages the `memory_v2` leg's does not. Static analysis cannot make that distinction, because the single binary links every backend.

### Fan-in Workflow (`ci-summary-report.yml`)

The single `summary-report` job:

1. **Resolves the source run** — determines the CI Orchestrator run ID (from `workflow_run` event or `workflow_dispatch` input), validates it succeeded, and extracts PR metadata (number + head SHA) via the GitHub API.
2. **Downloads all artifacts** — uses `gh run download` to fetch all artifacts from the source run.
3. **Checks the upload invariant** — `scripts/e2e/check_coverage_uploads.py` verifies that `after_n_builds` in `.codecov.yml` equals the number of uploads carrying countable coverage, and fails the run when at least that many jobs uploaded yet fewer carry coverage. Codecov withholds *every* notification — including the required `codecov/patch` and `codecov/project` statuses — until the threshold is met, so drift in that number blocks all merges with no failing check to point at. The check verifies uploads *carry coverage* rather than counting upload call sites: when this last drifted, the number of call sites stayed at 27 and only the content of 15 of them changed.
4. **Merges and gates coverage** — merges all `coverage-*/*.out` profiles with `gocovmerge`, filters excluded paths, and applies the two coverage gates.
5. **Fails the run on a blocking verdict** — exits 1 when the coverage gate failed or the metrics comparison hit an infrastructure error, which is what gates the merge (see *Where the gating happens* below). It also writes the two verdicts to a `ci-summary` artifact, from which `ci-summary-report-publish.yml` posts the sticky PR comment and the two advisory per-gate check runs.
6. **Saves baseline on `main`** — caches the coverage percentage for future PR comparisons.

### Key Files

| File | Role |
|------|------|
| `.github/workflows/ci-summary-report.yml` | Fan-in workflow, and the gating check |
| `.github/workflows/ci-summary-report-publish.yml` | Sticky PR comment and the advisory per-gate check runs |
| `.github/actions/upload-codecov/action.yml` | Coverage artifact upload |
| `.github/workflows/ci-orchestrator.yml` | Triggers the fan-in |
| `scripts/e2e/filter_coverage.py` | Applies `.codecov.yml` exclusions |
| `internal/tools/tools.go` | `gocovmerge` tool dependency |
| `.codecov.yml` | Single source of truth for ignore patterns, and the `after_n_builds` upload threshold |
| `scripts/e2e/check_coverage_uploads.py` | Upload-invariant check |
| `scripts/makefiles/IntegrationTests.mk` | Instrumented e2e binary build and coverage merge |
| `cmd/jaeger/internal/integration/binary.go` | Graceful shutdown so coverage counters flush |

### Where the gating happens

The fan-in job is the gate. `ci-summary-report.yml` ends with a step that exits 1 when the coverage gate failed or the metrics comparison hit an infrastructure error, so the job fails inside the CI Orchestrator run that GitHub associates with the commit. Its check run renders as `CI Summary Report / Summary Report`, from app id 15368, and that is the context branch protection requires. The full required set on `main` is `codecov/patch`, `codecov/project`, `check-label`, `All CI Checks Passed`, and `CI Summary Report / Summary Report`.

`Coverage Gate` and `Metrics Comparison` are advisory. `ci-summary-report-publish.yml` posts them so each gate has a check run of its own, carrying the coverage percentage and the metric change count in its summary, and branch protection deliberately requires neither.

Requiring them is what breaks. Both are created out of band: `workflow_run` puts the publish workflow's own head on `main`'s commit, so it reaches over to the pull request's head SHA with `github.rest.checks.create({ head_sha, name, conclusion })`. A required context created that way can sit unsatisfied in the merge box while the Checks API reports it `completed/success` on that exact SHA, from the app the context is pinned to. Observed on [#9354](https://github.com/jaegertracing/jaeger/pull/9354): all six required contexts green, `mergeable_state: blocked`, and the box listing those two as "Expected — Waiting for status to be reported". Deleting the superseded orchestrator run did not clear it, and neither did re-posting both check runs through the publish workflow's `workflow_dispatch` path; only a new head commit did. `gh-readonly-queue/*` commits get the same stamping, which is how a merge-queue entry is dropped with every check green. A context produced by the commit's own run is not exposed to this, because GitHub Actions creates that check run as part of the run instead of addressing it to a SHA afterwards.

Gating on the fan-in job means it fails closed. An infrastructure failure inside it — the artifact download, `check_coverage_uploads.py` — blocks merges, where the same failure previously only reddened a check that nothing required.

## Consequences

### Positive

- **Faster feedback**: coverage gate result appears as soon as the CI Orchestrator completes.
- **Reliability**: eliminates Codecov rate-limit failures blocking PRs.
- **Consolidated reporting**: performance metrics and coverage appear in a single sticky PR comment.
- **Required status check safe**: `Coverage Gate` is always created, even when coverage is skipped.

### Negative

- **Artifact storage cost**: `coverage-*` artifacts add ~50–100 MB per CI run (7-day retention).
- **One tool dependency**: `github.com/wadey/gocovmerge` in `internal/tools/go.mod`.

### Neutral

- Codecov remains active for long-term trending; removing it can be a follow-up decision.

### Follow-up resolved: Codecov stays a required gate

That follow-up decision is **declined**. `codecov/patch` enforces coverage on the changed lines, and the two gates above cannot approximate it: both derive from one project-wide percentage, and with the total near 97.4% against a 95% floor a pull request can leave a substantial diff uncovered without moving it measurably. The two systems are complements that fail in opposite directions — the project gates catch slow erosion across many pull requests, `codecov/patch` catches a single diff landing poorly covered code inside the headroom.

### Known gaps

Recorded so they are not mistaken for oversights:

1. **No diff-level gate here.** The 95% patch target that `AGENTS.md` documents is enforced solely by `codecov/patch`. Until this workflow computes coverage over the changed lines, Codecov cannot be retired as a required check, and a Codecov outage removes diff-level enforcement entirely.
2. **jaeger-v2 is compiled twice per e2e cell** — once as the instrumented binary the harness spawns, once as the test binary.

## References

- [CI Summary Report workflow](/.github/workflows/ci-summary-report.yml)
- [Coverage upload action](/.github/actions/upload-codecov/action.yml)
- [CI Orchestrator](/.github/workflows/ci-orchestrator.yml)
- [Coverage filter script](/scripts/e2e/filter_coverage.py)
- [Tool registry](/internal/tools/tools.go)
- [Coverage policy](/.codecov.yml)
- Delivering PRs for the 2026-07-29 extension: [#9130](https://github.com/jaegertracing/jaeger/pull/9130) (upload threshold), [#9133](https://github.com/jaegertracing/jaeger/pull/9133) (upload-invariant check), [#9139](https://github.com/jaegertracing/jaeger/pull/9139) (graceful shutdown), [#9140](https://github.com/jaegertracing/jaeger/pull/9140) (binary coverage); analysis in [#9084](https://github.com/jaegertracing/jaeger/issues/9084) and [#9131](https://github.com/jaegertracing/jaeger/pull/9131)
