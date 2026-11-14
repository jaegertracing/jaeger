<!--
Copyright (c) 2026 The Jaeger Authors.
SPDX-License-Identifier: Apache-2.0
-->

# Hosted demo maintenance

Renovate checks the two hosted demo directories without the repository's monthly
update schedule. Releases must be at least seven days old, and maintainers review
every update. OpenSearch and Dashboards remain a matched chart pair. Other updates
are grouped by deployment component. Charts own their bundled image versions;
direct image overrides and load-generator requirements are tracked separately.
PostgreSQL, Valkey, and OpenSearch stay within their configured major lines until
a reviewed migration or compatibility change updates that constraint.

The deployment workflow runs after relevant changes merge into main and daily.
`deploy.py` fingerprints each component's deployment inputs and records successful
revisions in the `default/jaeger-demo-deployment-revisions` ConfigMap. Failed
revisions are not recorded, so the next reconciliation retries them. The daily
run also refreshes Jaeger and HotROD snapshots in both demos. New installations
without revision records reconcile every component once. A partial failure does
not prevent independent components from deploying, but OpenSearch failure blocks
the dependent OTel Jaeger and application updates in that run.

Deployment, recovery, and volume expansion share the existing workflow concurrency
lock. Database updates run the existing retained snapshot and isolated restore
test within that lock, then the deployment script checks the recovery point and
upgrades OpenSearch before Dashboards. A version check and a fresh collector-to-
query trace check must pass before recording the revision. Recovery failure stops
the update. Automated runs never waive recovery or perform a clean reinstall.

The manual `demo_stack`, `otel_deploy_scope`, image overrides, `deploy_mode`, and
recovery inputs are preserved. The historical `otel_deploy_scope` input also
accepts `monitoring` for an isolated OCI monitoring upgrade. Invalid stack/scope
pairs fail. Direct script callers can select a scope through
`JAEGER_DEMO_DEPLOY_SCOPE`; the legacy OTel environment variable remains accepted.

For the initial catch-up, review and merge the paired OpenSearch/Dashboards 3.9.0
Renovate update first. Keep other component updates separate. Major database
changes require a tested migration in their reviewed change. Existing missing
OCI monitoring releases and pending Helm operations must be repaired under
#10002; an upgrade reports that failure instead of skipping monitoring and
reporting success. A `failed` monitoring release is upgraded in place by the
next reconciliation or `monitoring`-scope run.

Run the maintenance tests through `scripts/utils/run-tests.sh`, alongside the
repository-required formatting, lint, and test targets. To inspect discovery
without updating dependencies, use Renovate's local platform with
`--dry-run=extract`. Kubernetes and OCI infrastructure/add-on maintenance is
outside this workflow.
