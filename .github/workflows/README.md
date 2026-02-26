# CI Workflows

This directory contains GitHub Actions workflows for the Jaeger project. The workflows are organized into a staged architecture to optimize CI resource usage and provide fail-fast behavior.

## Architecture Overview

The CI system uses a **Forked DAG (Directed Acyclic Graph)** orchestrated by `ci-orchestrator.yml`. The orchestrator supports two execution paths based on the context of the run:

- **Sequential path (~30m)**: Default for external contributors. Stage 1 must pass before Stage 2, and Stage 2 must pass before Stage 3. Provides fail-fast behavior that saves resources when linting or unit tests fail.
- **Parallel path (~10m)**: For trusted maintainers, merge queue, and main branch builds. All three stages start simultaneously after a setup step.

### CI Orchestrator

The main entry point for PR and branch CI is **`ci-orchestrator.yml`**, which:
1. Runs a **`setup`** job to determine the execution mode (parallel or sequential)
2. Triggers either the sequential or parallel path based on the result

#### Setup Job: Execution Mode Detection

The `setup` job determines whether to use parallel execution based on these **OR** conditions:

| Condition | Rationale |
|-----------|-----------|
| Push to `main` branch | Already merged, fully trusted |
| `merge_group` event | Merge Queue entry, high confidence |
| PR author is an org member (`MEMBER` or `OWNER`) | Trusted maintainer |
| PR author login is `dependabot[bot]` or `renovate-bot` | Dependency automation bots |
| PR has the `ci:parallel` label | Explicit opt-in |

#### Stage Workflows (DRY Encapsulation)

Each stage is encapsulated in a reusable "stage" workflow:

- **ci-orchestrator-stage1.yml** - Stage 1 workflows (Linters only — fast fail-fast gate)
- **ci-orchestrator-stage2.yml** - Stage 2 workflows (Unit Tests)
- **ci-orchestrator-stage3.yml** - Stage 3 workflows (Docker, E2E, Binaries, Static Analysis)

This avoids duplication: both the sequential and parallel paths call the same stage workflows.

#### Stage 1: Fast Gate (Linters only)
- **ci-lint-checks.yaml** - Go linting, DCO checks, generated files validation, shell script linting

#### Stage 2: Unit Tests
- **ci-unit-tests.yml** - Full unit test suite with coverage

#### Stage 3: Expensive Checks & Static Analysis
Executes in parallel within the stage:
- **ci-build-binaries.yml** - Multi-platform binary builds
- **ci-docker-build.yml** - Docker images for all components
- **ci-docker-all-in-one.yml** - All-in-one Docker image
- **ci-docker-hotrod.yml** - HotROD demo application image
- **ci-e2e-all.yml** - E2E test suite orchestrator (calls individual E2E workflows)
- **ci-e2e-spm.yml** - Service Performance Monitoring tests
- **ci-e2e-tailsampling.yml** - Tail sampling processor tests
- **codeql.yml** - Security scanning with CodeQL
- **dependency-review.yml** - Dependency vulnerability checks
- **fossa.yml** - License compliance scanning

#### Gatekeeper Job
The orchestrator includes a final **`ci-success`** job that:
- Runs after all stage jobs (regardless of which path was taken)
- Determines which path was used and validates its results
- Should be used as the required status check in GitHub branch protection rules

### Execution Flow Diagram

```
                         ┌─────────┐
                         │  setup  │
                         └────┬────┘
              parallel=false  │  parallel=true
           ┌──────────────────┴──────────────────┐
           │  Sequential Path                     │  Parallel Path
           │                                      │
      ┌────▼──────┐                  ┌────────────┼────────────┐
      │ stage1-seq│                  │            │            │
      └────┬──────┘             ┌────▼───┐  ┌────▼───┐  ┌────▼───┐
           │                    │stage1- │  │stage2- │  │stage3- │
      ┌────▼──────┐             │  fast  │  │  fast  │  │  fast  │
      │ stage2-seq│             └────┬───┘  └────┬───┘  └────┬───┘
      └────┬──────┘                  │            │           │
           │                         └────────────┴───────────┘
      ┌────▼──────┐                              │
      │ stage3-seq│                              │
      └────┬──────┘                              │
           └──────────────────┬──────────────────┘
                         ┌────▼────┐
                         │ci-success│
                         └─────────┘
```

### Concurrency Control

The orchestrator manages concurrency centrally:
```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true
```

This allows a single "kill-switch" to cancel older runs when new commits are pushed to a PR.

### Permissions Model

The orchestrator uses `permissions: write-all` to allow maximum flexibility for child workflows:

```yaml
permissions: write-all
```

This grants broad permissions at the orchestrator level, allowing child workflows to request the specific permissions they need. Child workflows then apply the principle of least privilege by downgrading to only the permissions they require:

- **codeql.yml**: `security-events: write`, `actions: read` (for security scanning)
- **ci-unit-tests.yml**: `checks: write` (for reporting test results)
- **ci-docker-all-in-one.yml**: `packages: read` (for pulling from GHCR)
- Other workflows: typically `contents: read` only

**Why write-all?** When using `workflow_call`, GitHub Actions requires the caller workflow to grant permissions that called workflows can then use or downgrade. Without `write-all`, child workflows would be restricted to `contents: read` only, causing failures for workflows that need additional permissions like CodeQL or test reporting.

## Independent Workflows

The following workflows operate independently and are **not** part of the orchestrator:

### Release & Deployment
- **ci-release.yml** - Triggered on release events to build and publish artifacts
- **ci-deploy-demo.yml** - Scheduled/manual deployment to demo environment

### Automated Checks
- **ci-compare-metrics.yml** - Compares metrics from E2E tests (triggered by workflow_run)
- **label-check.yml** - Verifies PR labels
- **pr-quota-manager.yml** - PR management automation
- **dco_merge_group.yml** - DCO verification for merge groups

### Scheduled Maintenance
- **stale.yml** - Marks and closes stale issues/PRs
- **waiting-for-author.yml** - PR status management
- **scorecard.yml** - Security scorecard scanning

### Special Cases
- **ci-unit-tests-go-tip.yml** - Tests against Go development version (runs on main or when workflow modified)
- **codeql.yml** - Also runs on schedule (weekly) in addition to being called by orchestrator

## E2E Test Workflows

Individual E2E test workflows are called by `ci-e2e-all.yml`:
- ci-e2e-badger.yaml
- ci-e2e-cassandra.yml
- ci-e2e-clickhouse.yml
- ci-e2e-elasticsearch.yml
- ci-e2e-grpc.yml
- ci-e2e-kafka.yml
- ci-e2e-memory.yaml
- ci-e2e-opensearch.yml
- ci-e2e-query.yml

These workflows use `workflow_call` only and don't have independent triggers.

## Branch Protection

To require CI checks before merging, configure branch protection to require:
- **CI Orchestrator / ci-success** - This single check represents the entire CI pipeline

This is much simpler than requiring 10+ individual workflow checks.

## Local Development

Individual workflows can still be triggered manually via the GitHub Actions UI for testing or debugging purposes. However, on PR events, only the orchestrator runs to avoid duplicate work.

## Benefits

1. **Reduced Feedback Loop**: Trusted contributors get ~10m feedback instead of ~30m
2. **Fail-Fast for External Contributors**: Expensive checks only run after cheaper ones pass, saving resources
3. **Simplified Branch Protection**: Single `ci-success` check represents the entire CI pipeline
4. **Centralized Concurrency Control**: Single kill-switch via `cancel-in-progress: true`
5. **DRY Stage Workflows**: Both execution paths reuse the same `ci-orchestrator-stage*.yml` workflows
6. **Maintainability**: Individual child workflows remain decoupled and independently testable
