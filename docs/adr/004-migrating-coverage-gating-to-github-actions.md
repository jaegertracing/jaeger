# Migrate Coverage Gating from Codecov to GitHub Actions

## Status

Accepted (implemented)

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

### Fan-in Workflow (`ci-summary-report.yml`)

The single `summary-report` job:

1. **Resolves the source run** — determines the CI Orchestrator run ID (from `workflow_run` event or `workflow_dispatch` input), validates it succeeded, and extracts PR metadata (number + head SHA) via the GitHub API.
2. **Downloads all artifacts** — uses `gh run download` to fetch all artifacts from the source run.
3. **Merges and gates coverage** — merges all `coverage-*/*.out` profiles with `gocovmerge`, filters excluded paths, and applies the two coverage gates.
4. **Posts results** — creates `Metrics Comparison` and `Coverage Gate` check-runs on the PR. When no coverage data exists, `Coverage Gate` reports success with a "skipped" note to satisfy branch protection.
5. **Saves baseline on `main`** — caches the coverage percentage for future PR comparisons.

### Key Files

| File | Role |
|------|------|
| `.github/workflows/ci-summary-report.yml` | Fan-in workflow |
| `.github/actions/upload-codecov/action.yml` | Coverage artifact upload |
| `.github/workflows/ci-orchestrator.yml` | Triggers the fan-in |
| `scripts/e2e/filter_coverage.py` | Applies `.codecov.yml` exclusions |
| `internal/tools/tools.go` | `gocovmerge` tool dependency |
| `.codecov.yml` | Single source of truth for ignore patterns |

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

## References

- [CI Summary Report workflow](/.github/workflows/ci-summary-report.yml)
- [Coverage upload action](/.github/actions/upload-codecov/action.yml)
- [CI Orchestrator](/.github/workflows/ci-orchestrator.yml)
- [Coverage filter script](/scripts/e2e/filter_coverage.py)
- [Tool registry](/internal/tools/tools.go)
- [Coverage policy](/.codecov.yml)
