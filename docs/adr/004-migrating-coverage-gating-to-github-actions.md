# Migrate Coverage Gating from Codecov to GitHub Actions

## Status

Accepted

## Context

### Current Architecture

Jaeger uses [Codecov](https://codecov.io) for two functions:

1. **Long-term trend tracking**: Coverage is uploaded after each CI run via the Codecov Action.
2. **PR gating**: Codecov's GitHub status check blocks merges when coverage drops below a threshold.

Coverage is collected across 11 CI jobs and uploaded through `.github/actions/upload-codecov/action.yml`. The jobs and their coverage files are:

| Workflow | Coverage files | Codecov flag |
|----------|---------------|--------------|
| `ci-unit-tests.yml` | `cover.out` | `unittests` |
| `ci-e2e-badger.yaml` | `cover.out` | `badger_<version>` |
| `ci-e2e-cassandra.yml` | `cover.out` | `cassandra-<major>-<jaeger>-<schema>` |
| `ci-e2e-clickhouse.yml` | `cover.out` | `clickhouse` |
| `ci-e2e-elasticsearch.yml` | `cover.out`, `cover-index-cleaner.out`, `cover-index-rollover.out` | `elasticsearch-<major>-<jaeger>` |
| `ci-e2e-grpc.yml` | `cover.out` | `grpc_<version>` |
| `ci-e2e-kafka.yml` | `cover.out` | `kafka-<version>-v2` |
| `ci-e2e-memory.yaml` | `cover.out` | `memory_v2` |
| `ci-e2e-opensearch.yml` | `cover.out`, `cover-index-cleaner.out`, `cover-index-rollover.out` | `opensearch-<major>-<jaeger>` |
| `ci-e2e-query.yml` | `cover.out` | `query` |
| `ci-e2e-tailsampling.yml` | `cover.out` | `tailsampling-processor` |

After all CI stages complete, `.github/workflows/ci-summary-report.yml` runs as a `workflow_run` fan-in triggered by the `"CI Orchestrator"` workflow. It downloads all artifacts from the triggering run via the GitHub REST API, generates a metrics comparison report, and posts it as a sticky PR comment.

### Problem

Codecov's PR status checks suffer from latency (results lag behind CI completion) and intermittent rate-limit failures that block PRs even when coverage is healthy. The gating logic should run entirely within GitHub Actions for faster, more reliable feedback.

## Decision

Extend the existing fan-in workflow pattern to add coverage aggregation and gating alongside the existing metrics comparison. This maximizes reuse of the established `github-script` artifact download infrastructure.

The change is additive: Codecov uploads are retained for long-term historical trending and per-flag breakdown views.

Key design choices:

- **Trigger on the CI Orchestrator**: the `workflow_run` trigger fires on `["CI Orchestrator"]` completion, ensuring the fan-in has access to unit test coverage artifacts as well as E2E coverage. The CI Orchestrator (`ci-orchestrator.yml`) completes only after all stages (lint, unit tests, E2E) finish.

- **Artifacts for cross-run data sharing**: `workflow_run` jobs run with write permissions (required to post PR comments from fork PRs) but cannot access artifacts from the triggering run directly — they must use the GitHub REST API. The existing `github-script` download loop already handles this and automatically picks up `coverage-*` artifacts alongside metrics artifacts.

- **Single job for both PR analysis and baseline updates**: the job runs for `pull_request` events and for pushes to `main`. PR-specific steps (metrics comparison, coverage gate, PR comment, check runs) are conditioned on `pr_number` being set; baseline-save steps are conditioned on `head_branch == 'main'`. Coverage computation runs unconditionally so both flows share the same merge-and-measure logic. This follows the same pattern as the existing metrics snapshot baseline.

- **Coverage policy**: two gates matching `.codecov.yml`:
  1. Absolute floor: fail if total coverage drops below 95%.
  2. No regression: fail if total coverage dropped compared to the `main` baseline.

## Implementation

### `upload-codecov` action (`upload-codecov/action.yml`)

- Rename the `flags` input to `flag` (singular — all callers pass exactly one value).
- After staging the coverage files, upload them as a `coverage-<flag>` artifact (7-day retention) **before** the Codecov upload step, so the artifact is available to the fan-in even if the Codecov upload fails (e.g. rate-limit).
- The artifact name is derived as `coverage-<flag>`, removing the need for a separate `artifact-name` input.

### Caller workflows (11 files)

Update every `upload-codecov` call site to use `flag:` (singular) instead of `flags:`. No other change is needed — artifact naming is derived automatically from the flag value.

### `gocovmerge` tool (`internal/tools/`)

Add `github.com/wadey/gocovmerge` as a pinned blank import in `internal/tools/tools.go` and a corresponding `install-coverage-tools` Make target in `scripts/makefiles/Tools.mk`. Coverage percentage is computed with `go tool cover -func` from the standard Go toolchain — no additional binary is required.

### Fan-in workflow (`ci-compare-metrics.yml` → `ci-summary-report.yml`)

Rename the workflow. The single `summary-report` job runs for both `pull_request` events and pushes to `main`:

- Downloads all artifacts to `.artifacts/` via the existing `github-script` loop. On `main`-branch runs no PR number is found; a warning is logged and the step succeeds so subsequent steps can continue.
- Runs `scripts/e2e/metrics_summary.sh` for metrics comparison (PR runs only, gated on `pr_number`).
- Unconditionally merges all `coverage-*/**.out` profiles with `gocovmerge` and computes total percentage with `go tool cover -func`.
- On PR runs: restores baseline from `actions/cache`, applies the two coverage gates, appends a coverage section to `combined_summary.md`, posts a sticky PR comment, and creates `Metrics Comparison` and `Coverage Gate` check runs.
- On main-branch runs: saves the computed coverage percentage to `actions/cache` under `coverage-baseline_<run_id>` (prefix `coverage-baseline`) for future PR comparisons.

### Branch protection (optional)

Add `Coverage Gate` to the required status checks alongside the existing `Metrics Comparison` check.

## Consequences

### Positive

- **Faster feedback**: coverage gate result appears as soon as the CI Orchestrator completes, without waiting for Codecov's external processing pipeline.
- **Reliability**: eliminates Codecov rate-limit failures and network timeouts blocking PRs.
- **Consolidated reporting**: performance metrics and coverage appear in a single sticky PR comment, reducing comment noise.
- **Minimal new infrastructure**: the `github-script` artifact download loop and `actions/cache` baseline pattern are reused directly from existing workflows.

### Negative

- **Artifact storage cost**: `coverage-*` artifacts are ~1–5 MB each × ~20 matrix jobs = ~50–100 MB per CI run, retained 7 days. GitHub-hosted storage is generally within free-tier limits for open-source projects.
- **Longer summary workflow**: `make install-coverage-tools`, `gocovmerge`, and `go tool cover` add steps to the fan-in job.
- **One new tool dependency**: `github.com/wadey/gocovmerge` is added to `internal/tools/go.mod`, version-pinned like all other tools.
- **Trigger change latency**: triggering on `"CI Orchestrator"` means the fan-in waits for all three stages, which is consistent with the existing Codecov reporting latency.

### Neutral

- Codecov remains active; removing it can be a follow-up decision once the new gate has been validated.

## References

- Reusable coverage action: `.github/actions/upload-codecov/action.yml`
- CI Summary Report workflow: `.github/workflows/ci-summary-report.yml`
- Metrics snapshot reusable action (cache pattern): `.github/actions/verify-metrics-snapshot/action.yaml`
- Main CI orchestrator: `.github/workflows/ci-orchestrator.yml`
- Tool registry: `internal/tools/tools.go`, `internal/tools/go.mod`
- Tool install targets: `scripts/makefiles/Tools.mk`
- Coverage policy: `.codecov.yml`
