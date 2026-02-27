# Migrate Coverage Gating from Codecov to GitHub Actions

## Status

Proposed

## Context

### Current Architecture

Jaeger uses [Codecov](https://codecov.io) for two functions:

1. **Long-term trend tracking**: Coverage is uploaded after each CI run via the Codecov Action.
2. **PR gating**: Codecov's GitHub status check blocks merges when coverage drops below a threshold.

Coverage is collected across 11 CI jobs and uploaded through the `.github/actions/upload-codecov/action.yml` reusable action (`upload-codecov/action.yml:21-33`). The jobs and their coverage files are:

| Workflow | Coverage files | Codecov flag |
|----------|---------------|--------------|
| `ci-unit-tests.yml:39` | `cover.out` | `unittests` |
| `ci-e2e-badger.yaml:45` | `cover.out` | `badger_<version>` |
| `ci-e2e-cassandra.yml` | `cover.out` | `cassandra-<major>-<jaeger>-<schema>` |
| `ci-e2e-clickhouse.yml` | `cover.out` | `clickhouse` |
| `ci-e2e-elasticsearch.yml:63` | `cover.out`, `cover-index-cleaner.out`, `cover-index-rollover.out` | `elasticsearch-<major>-<jaeger>` |
| `ci-e2e-grpc.yml` | `cover.out` | `grpc_<version>` |
| `ci-e2e-kafka.yml` | `cover.out` | `kafka-<version>-v2` |
| `ci-e2e-memory.yaml` | `cover.out` | `memory_v2` |
| `ci-e2e-opensearch.yml` | `cover.out`, `cover-index-cleaner.out`, `cover-index-rollover.out` | `opensearch-<major>-<jaeger>` |
| `ci-e2e-query.yml` | `cover.out` | `query` |
| `ci-e2e-tailsampling.yml` | `cover.out` | `tailsampling-processor` |

After the E2E tests complete, `.github/workflows/ci-compare-metrics.yml` runs as a `workflow_run` fan-in triggered by the `"E2E Tests"` workflow (`ci-e2e-all.yml`). It uses an `actions/github-script` step (`ci-compare-metrics.yml:26-94`) to download all artifacts from the triggering run via the GitHub REST API, then calls `scripts/e2e/metrics_summary.sh` to generate a comparison report and posts it as a sticky PR comment.

### Problem

Codecov's PR status checks suffer from latency (results lag behind CI completion) and intermittent rate-limit failures that block PRs even when coverage is healthy. The gating logic should run entirely within GitHub Actions for faster, more reliable feedback.

## Decision

Extend the existing fan-in workflow pattern to add coverage aggregation and gating alongside the existing metrics comparison. This maximizes reuse of the established `github-script` artifact download infrastructure.

The change is additive: Codecov uploads are retained for long-term historical trending and per-flag breakdown views.

Key design choices:

- **Trigger on the CI Orchestrator** (not just `"E2E Tests"`): changing the `workflow_run` trigger to `["CI Orchestrator"]` ensures the fan-in fires after all stages complete, giving access to unit test coverage artifacts as well as E2E coverage. The CI Orchestrator (`ci-orchestrator.yml`) completes only after all stages (stage 1 lint, stage 2 unit tests, stage 3 E2E) finish.

- **Artifacts for cross-run data sharing**: `workflow_run` jobs run with write permissions (required to post PR comments from fork PRs) but cannot access artifacts from the triggering run directly—they must use the GitHub REST API. The existing `github-script` download loop (`ci-compare-metrics.yml:26-55`) already handles this and will automatically pick up `coverage-*` artifacts alongside metrics artifacts.

- **Baseline storage via `actions/cache`**: reuse the same `actions/cache/save` + `actions/cache/restore` pattern used by `.github/actions/verify-metrics-snapshot/action.yaml:12-30` for metrics baselines on `main`.

## Implementation Plan

### Step 1: Extend `.github/actions/upload-codecov/action.yml`

Rename the `flags` input to `flag` (singular — all callers pass exactly one value) and remove the separate `artifact-name` input, deriving the artifact name as `coverage-<flag>` internally. The staging and upload steps become unconditional:

```yaml
inputs:
  files:
    description: 'Coverage files to upload (comma-separated)'
    required: true
  flag:
    description: 'Codecov flag for this upload; also used as the artifact name suffix (coverage-<flag>)'
    required: true
runs:
  using: 'composite'
  steps:
    - name: Retry upload        # existing step, unchanged
      uses: Wandalen/wretry.action@...
      with: |
        flags: ${{ inputs.flag }}
        ...

    - name: Stage coverage files for artifact upload
      shell: bash
      run: |
        mkdir -p /tmp/coverage-staging
        IFS=',' read -ra FILES <<< "${{ inputs.files }}"
        for f in "${FILES[@]}"; do
          [ -f "$f" ] && cp "$f" /tmp/coverage-staging/
        done

    - name: Upload coverage profiles as artifact
      uses: actions/upload-artifact@v4
      with:
        name: coverage-${{ inputs.flag }}
        path: /tmp/coverage-staging/
        retention-days: 7
```

### Step 2: Rename `flags` → `flag` in all 11 callers

Replace `flags:` with `flag:` at every call site (no other change needed — artifact naming is derived automatically):

| File | `flag` value |
|------|-------------|
| `ci-unit-tests.yml` | `unittests` |
| `ci-e2e-badger.yaml` | `badger_${{ matrix.version }}` |
| `ci-e2e-cassandra.yml` | `cassandra-${{ matrix.version.major }}-${{ matrix.jaeger-version }}-${{ matrix.create-schema }}` |
| `ci-e2e-clickhouse.yml` | `clickhouse` |
| `ci-e2e-elasticsearch.yml` | `${{ matrix.version.distribution }}-${{ matrix.version.major }}-${{ matrix.version.jaeger }}` |
| `ci-e2e-grpc.yml` | `grpc_${{ matrix.version }}` |
| `ci-e2e-kafka.yml` | `kafka-${{ matrix.kafka-version }}-v2` |
| `ci-e2e-memory.yaml` | `memory_v2` |
| `ci-e2e-opensearch.yml` | `${{ matrix.version.distribution }}-${{ matrix.version.major }}-${{ matrix.version.jaeger }}` |
| `ci-e2e-query.yml` | `query` |
| `ci-e2e-tailsampling.yml` | `tailsampling-processor` |

### Step 3: Add `gocovmerge` and `go-coverage-report` to `internal/tools/`

Following the established pattern in `scripts/makefiles/Tools.mk` and `internal/tools/tools.go`:

1. Add blank imports to `internal/tools/tools.go`:

```go
_ "github.com/wadey/gocovmerge"
_ "github.com/fgrosse/go-coverage-report/cmd/go-coverage-report"
```

2. Run `cd internal/tools && go get github.com/wadey/gocovmerge github.com/fgrosse/go-coverage-report` to add entries to `go.mod`/`go.sum`.

3. Add a named variable and a new target to `scripts/makefiles/Tools.mk`:

```makefile
GOCOVMERGE := $(TOOLS_BIN_DIR)/gocovmerge

.PHONY: install-coverage-tools
install-coverage-tools: $(GOCOVMERGE)
```

The generic build rule on `Tools.mk:37-38` automatically handles building the binary from `internal/tools/go.mod`.

**Note on `go-coverage-report`**: the `github.com/fgrosse/go-coverage-report` module declares `go 1.21` in its `go.mod`, which prevents blank-importing its `package main` command in Go 1.21+. Rather than work around this limitation, the per-file coverage report is generated using the `go tool cover -func` command that ships with the Go toolchain, requiring no additional dependency.

### Step 4: Rename and extend `ci-compare-metrics.yml` → `ci-summary-report.yml`

**4a. Change trigger and workflow name** (`ci-compare-metrics.yml:1,3-4`):

```yaml
# Before:
name: Metrics Comparison and Post Comment
  workflow_run:
    workflows: ["E2E Tests"]

# After:
name: CI Summary Report
  workflow_run:
    workflows: ["CI Orchestrator"]
```

**4b. Artifact download** (`ci-compare-metrics.yml:24-55`): no changes needed. The existing loop downloads ALL artifacts from the triggering workflow run. Coverage artifacts (`coverage-*`) will automatically be extracted to `.metrics/coverage-<name>/` alongside metrics artifacts.

**4c. Add coverage processing steps** after the existing "Compare metrics and generate summary" step:

```yaml
    - name: Install coverage tools
      run: make install-coverage-tools

    - name: Merge coverage profiles
      id: merge-coverage
      run: |
        mapfile -t COVER_FILES < <(find .metrics -path "*/coverage-*/*.out" -type f)
        if [ ${#COVER_FILES[@]} -eq 0 ]; then
          echo "No coverage files found; skipping."
          echo "skipped=true" >> "$GITHUB_OUTPUT"
          exit 0
        fi
        ./.tools/gocovmerge "${COVER_FILES[@]}" > .metrics/merged-coverage.out
        echo "skipped=false" >> "$GITHUB_OUTPUT"

    - name: Calculate current coverage percentage
      id: coverage
      if: steps.merge-coverage.outputs.skipped == 'false'
      run: |
        PCT=$(go tool cover -func=.metrics/merged-coverage.out \
          | grep "^total:" | awk '{print $3}' | tr -d '%')
        echo "percentage=${PCT}" >> "$GITHUB_OUTPUT"
        echo "${PCT}" > .metrics/current-coverage.txt

    - name: Restore baseline coverage from main
      id: restore-baseline
      if: steps.merge-coverage.outputs.skipped == 'false'
      uses: actions/cache/restore@v4
      with:
        path: .metrics/baseline-coverage.txt
        key: coverage-baseline
        restore-keys: |
          coverage-baseline

    - name: Gate on coverage regression
      id: coverage-gate
      if: steps.merge-coverage.outputs.skipped == 'false'
      run: |
        CURRENT="${{ steps.coverage.outputs.percentage }}"
        MINIMUM=95.0
        failure_reasons=()

        # Gate 1: absolute minimum (matches .codecov.yml target: 95%)
        if (( $(echo "$CURRENT < $MINIMUM" | bc -l) )); then
          failure_reasons+=("coverage ${CURRENT}% is below required minimum ${MINIMUM}%")
        fi

        # Gate 2: no regression vs main baseline
        BASELINE_MSG="(no baseline yet)"
        if [ -f .artifacts/baseline-coverage.txt ]; then
          BASELINE=$(cat .artifacts/baseline-coverage.txt)
          BASELINE_MSG="(baseline ${BASELINE}%)"
          if (( $(echo "$CURRENT < $BASELINE" | bc -l) )); then
            failure_reasons+=("coverage dropped from ${BASELINE}% to ${CURRENT}%")
          fi
        fi

        if [ ${#failure_reasons[@]} -gt 0 ]; then
          msg=$(IFS='; '; echo "${failure_reasons[*]}")
          echo "conclusion=failure" >> "$GITHUB_OUTPUT"
          echo "summary=${msg}" >> "$GITHUB_OUTPUT"
        else
          echo "conclusion=success" >> "$GITHUB_OUTPUT"
          echo "summary=Coverage ${CURRENT}% ${BASELINE_MSG}" >> "$GITHUB_OUTPUT"
        fi

    - name: Save coverage baseline on main branch
      if: |
        steps.merge-coverage.outputs.skipped == 'false' &&
        github.event.workflow_run.head_branch == 'main'
      run: cp .metrics/current-coverage.txt .metrics/baseline-coverage.txt
    - name: Cache new baseline
      if: |
        steps.merge-coverage.outputs.skipped == 'false' &&
        github.event.workflow_run.head_branch == 'main'
      uses: actions/cache/save@v4
      with:
        path: .metrics/baseline-coverage.txt
        key: coverage-baseline_${{ github.run_id }}

    - name: Append coverage section to combined summary
      if: steps.merge-coverage.outputs.skipped == 'false'
      run: |
        {
          echo ""
          echo "## Code Coverage"
          echo ""
          echo "${{ steps.coverage-gate.outputs.summary }}"
        } >> .metrics/combined_summary.md

    - name: Create check run for coverage gate
      if: steps.merge-coverage.outputs.skipped == 'false'
      uses: actions/github-script@v8
      with:
        script: |
          await github.rest.checks.create({
            owner: context.repo.owner,
            repo: context.repo.repo,
            name: 'Coverage Gate',
            head_sha: context.payload.workflow_run.head_sha,
            status: 'completed',
            conclusion: '${{ steps.coverage-gate.outputs.conclusion }}',
            output: {
              title: 'Coverage Gate',
              summary: '${{ steps.coverage-gate.outputs.summary }}'
            }
          });
      env:
        GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

**4d. Update the PR comment step** (`ci-compare-metrics.yml:120-127`): the `combined_summary.md` now includes the coverage section, so the existing `thollander/actions-comment-pull-request` call requires no change. Update the comment tag from `"## Metrics Comparison Summary"` to `"## CI Summary Report"` to match the renamed workflow.

**4e. Update the condition** on the PR comment step (`ci-compare-metrics.yml:121`): add `|| steps.coverage-gate.outputs.conclusion == 'failure'` so coverage regressions always post a comment even if metrics are unchanged.

### Step 5: (Optional) Add coverage check to branch protection

Add `Coverage Gate` to the required status checks in the repository's branch protection rules, alongside the existing `Metrics Comparison` check.

## Consequences

### Positive

- **Faster feedback**: coverage gate result appears as soon as the CI Orchestrator completes, without waiting for Codecov's external processing pipeline.
- **Reliability**: eliminates Codecov rate-limit failures and network timeouts blocking PRs.
- **Consolidated reporting**: performance metrics and coverage appear in a single sticky PR comment, reducing comment noise.
- **Minimal new infrastructure**: the `github-script` artifact download loop (`ci-summary-report.yml:26-55`, renamed from `ci-compare-metrics.yml`) and `actions/cache` baseline pattern (`.github/actions/verify-metrics-snapshot/action.yaml:12-30`) are reused directly.

### Negative

- **Artifact storage cost**: `coverage-*` artifacts are ~1–5 MB each × ~20 matrix jobs = ~50–100 MB per CI run, retained 7 days. GitHub-hosted storage is generally within free-tier limits for open-source projects.
- **Longer summary workflow**: `make install-coverage-tools`, `gocovmerge`, and `go tool cover` add steps to the fan-in job. These run sequentially after the existing metrics comparison.
- **One new tool dependency**: `github.com/wadey/gocovmerge` must be added to `internal/tools/go.mod`. It is version-pinned there like all other tools, providing supply-chain guarantees consistent with the rest of the project. Coverage reporting uses `go tool cover -func` from the standard Go toolchain (no extra binary needed).
- **Trigger change latency**: triggering on `"CI Orchestrator"` instead of `"E2E Tests"` means the fan-in waits for all three stages. In sequential mode this is ~30 minutes after PR push (unchanged from current Codecov reporting, which also waits for all uploads). In parallel mode it is ~10 minutes.

### Neutral

- Codecov remains active; removing it can be a follow-up decision once the new gate has been validated.
- The `artifact-name` input on `upload-codecov` is opt-in; existing callers that do not pass it are unaffected during a staged rollout.

## References

- Reusable coverage action: `.github/actions/upload-codecov/action.yml`
- CI Summary Report workflow (renamed from `ci-compare-metrics.yml`): `.github/workflows/ci-summary-report.yml`
- Metrics snapshot reusable action (cache pattern to reuse): `.github/actions/verify-metrics-snapshot/action.yaml`
- Main CI orchestrator (new trigger target): `.github/workflows/ci-orchestrator.yml`
- E2E aggregator workflow: `.github/workflows/ci-e2e-all.yml`
- Unit test workflow: `.github/workflows/ci-unit-tests.yml`
- Example multi-file coverage job: `.github/workflows/ci-e2e-elasticsearch.yml:63`
- Tool registry (to add new tools): `internal/tools/tools.go`, `internal/tools/go.mod`
- Tool install targets: `scripts/makefiles/Tools.mk`
