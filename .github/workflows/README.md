# CI Workflows

This directory contains GitHub Actions workflows for the Jaeger project. The workflows are organized into a tiered architecture to optimize CI resource usage and provide fail-fast behavior.

## Architecture Overview

The CI system uses a **3-Tier Sequential Pipeline** orchestrated by `ci-orchestrator.yml`. This design ensures that expensive checks (builds, E2E tests) only run after cheaper checks (linting, unit tests) have passed, saving resources and providing faster feedback.

### CI Orchestrator

The main entry point for PR and branch CI is **`ci-orchestrator.yml`**, which coordinates all CI checks in three tiers:

#### Tier 1: Cheap Checks (Linters & Static Analysis)
These run first and in parallel:
- **ci-lint-checks.yaml** - Go linting, DCO checks, generated files validation, shell script linting
- **codeql.yml** - Security scanning with CodeQL
- **dependency-review.yml** - Dependency vulnerability checks
- **fossa.yml** - License compliance scanning

#### Tier 2: Unit Tests
Runs only if Tier 1 passes:
- **ci-unit-tests.yml** - Full unit test suite with coverage

#### Tier 3: Expensive Checks
Runs only if Tier 2 passes, executes in parallel:
- **ci-build-binaries.yml** - Multi-platform binary builds
- **ci-docker-build.yml** - Docker images for all components
- **ci-docker-all-in-one.yml** - All-in-one Docker image
- **ci-docker-hotrod.yml** - HotROD demo application image
- **ci-e2e-all.yml** - E2E test suite orchestrator (calls individual E2E workflows)
- **ci-e2e-spm.yml** - Service Performance Monitoring tests
- **ci-e2e-tailsampling.yml** - Tail sampling processor tests

#### Gatekeeper Job
The orchestrator includes a final **`ci-success`** job that:
- Waits for all Tier 3 jobs to complete
- Reports overall CI status
- Should be used as the required status check in GitHub branch protection rules

### Concurrency Control

The orchestrator manages concurrency centrally:
```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.event.pull_request.number || github.ref }}
  cancel-in-progress: true
```

This allows a single "kill-switch" to cancel older runs when new commits are pushed to a PR.

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

1. **Fail-Fast**: Expensive checks only run after cheaper ones pass
2. **Resource Optimization**: Saves ~60-70% of CI time on PRs with basic issues
3. **Simplified Branch Protection**: Single status check instead of 10+
4. **Better Concurrency**: Centralized cancel-in-progress logic
5. **Maintainability**: Individual workflows remain decoupled and testable

## Migration Notes

This architecture was introduced to address high CI resource usage from concurrent workflow execution. The change:
- Preserves all existing workflow logic
- Maintains all security and quality checks
- Reduces redundant work through sequential tiering
- Provides faster feedback for common issues (linting, unit tests)
