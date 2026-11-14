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
lock. Hosted demo traces are disposable, so scheduled, post-merge, and default
manual runs use `opensearch_recovery=waive`: a database update upgrades OpenSearch
before Dashboards without a snapshot or restore test. A version check and a fresh
collector-to-query trace check must pass before recording the revision. During
a deployment, the retained snapshot and isolated restore test run only when a
manual upgrade dispatch that includes OpenSearch selects
`opensearch_recovery=required`; the deployment script then checks the recovery
point, and a recovery failure stops the update. Automated runs never perform a
clean reinstall.

The manual `demo_stack`, `otel_deploy_scope`, image overrides, `deploy_mode`, and
recovery inputs are preserved. The historical `otel_deploy_scope` input also
accepts `monitoring` for an isolated OCI monitoring upgrade. Invalid stack/scope
pairs fail. Direct script callers can select a scope through
`JAEGER_DEMO_DEPLOY_SCOPE`; the legacy OTel environment variable remains accepted.

For the initial catch-up, review and merge the paired OpenSearch/Dashboards 3.9.0
Renovate update first. Keep other component updates separate. Major database
changes require a tested migration in their reviewed change. The post-merge run
deploys a merged database update under the waiver, and `required` cannot be
selected for it. To keep a tested recovery point for existing data, run the
manual `Verify OTel Demo OpenSearch Recovery` workflow on main and confirm that
it succeeded before merging. Existing missing or failed OCI monitoring releases
must be repaired under #10002; an upgrade reports that failure instead of
skipping monitoring and reporting success.

Run the maintenance tests through `scripts/utils/run-tests.sh`, alongside the
repository-required formatting, lint, and test targets. To inspect discovery
without updating dependencies, use Renovate's local platform with
`--dry-run=extract`. Kubernetes and OCI infrastructure/add-on maintenance is
outside this workflow.
