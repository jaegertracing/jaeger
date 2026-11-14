# OpenTelemetry Demo app + HotRODapp + Jaeger + OpenSearch

This example provides a one-command deployment of a complete observability stack on Kubernetes:
- Jaeger (all-in-one) for tracing
- OpenSearch and OpenSearch Dashboards
- OpenTelemetry Demo application (multi-service web store)
- HotRod application

It is driven by `deploy-all.sh`, which supports both clean installs and upgrades.

## Prerequisites
- Kubernetes cluster reachable via `kubectl`
- Installed CLIs: `bash`, `git`, `curl`, `kubectl`, `helm`
- Network access to Helm repositories

## Quick start
- Clean install (removes previous releases/namespaces, then installs everything):
```bash path=null start=null
./deploy-all.sh clean
```
- Upgrade (default) — installs if missing, upgrades if present:
```bash path=null start=null
./deploy-all.sh
# or explicitly
./deploy-all.sh upgrade
```
- Specify Jaeger all-in-one image tag:
```bash path=null start=null
./deploy-all.sh upgrade <image-tag>
# Example
./deploy-all.sh upgrade latest
```

Environment variables:
- ROLLOUT_TIMEOUT: rollout wait timeout in seconds (default 600)
- OPENSEARCH_DASHBOARDS_HELM_TIMEOUT: Helm wait timeout for OpenSearch Dashboards (default 60m)

```bash path=null start=null
ROLLOUT_TIMEOUT=900 ./deploy-all.sh clean
```

## What gets deployed
- Namespace `opensearch`:
  - OpenSearch (single node) StatefulSet
  - OpenSearch Dashboards Deployment
- Namespace `jaeger`:
  - Jaeger all-in-one Deployment with OpenSearch-backed trace storage
  - HOTROD application
  - Jaeger Query ClusterIP service (jaeger-query-clusterip)
- Namespace `otel-demo`:
  - OpenTelemetry Demo (frontend, load-generator, and supporting services)


## Verifying the deployment
- Pods status:
```bash path=null start=null
kubectl get pods -n opensearch
kubectl get pods -n jaeger
kubectl get pods -n otel-demo
```
- Services:
```bash path=null start=null
kubectl get svc -n opensearch
kubectl get svc -n jaeger
kubectl get svc -n otel-demo
```


## Automatic port-forward using scrpit
 - OpenSearch Dashboards:
```bash path=null start=null
./start-port-forward.sh
```


## Customization
- Helm values provided in this directory:
  - `opensearch-values.yaml`
  - `opensearch-dashboard-values.yaml`
  - `jaeger-values.yaml`
  - `jaeger-config.yaml`
  - `otel-demo-values.yaml`
  - `jaeger-query-service.yaml`

You can adjust these files and re-run `./deploy-all.sh upgrade` to apply changes.

### UI analytics

Both public Jaeger demos use the shared [UI configuration](../oci/ui-config.json)
with GA4 measurement ID `G-S88L6V684T` and JavaScript error tracking enabled.
`deploy-all.sh` passes this file to Helm as `uiconfig`; Jaeger Query reads it
from `/etc/jaeger/ui-config/ui-config.json`.

The measurement ID belongs to **CNCF Projects → jaegertracing.io - GA4**, which
also collects the main website's traffic. To separate the reports while retaining
all collected data, use these saved comparisons under **Admin → Data display →
Comparisons**. Set the dimension to **Hostname**, the match type to **exactly
matches**, and select the hostnames as alternative values in the same condition:

| Comparison | Hostnames |
| --- | --- |
| Jaeger Website | `jaegertracing.io`, `www.jaegertracing.io` |
| Jaeger Demos | `demo.jaegertracing.io`, `jaeger.demo.jaegertracing.io` |

Apply the comparison from the comparison picker at the top of a report.
**All Users** continues to include website and demo traffic. Add **Hostname**
as a report dimension to distinguish the two demos, including pages with the same
path. These comparisons apply to existing data as well as new events.
See [Google's comparison documentation](https://support.google.com/analytics/answer/9269518?hl=en).

After deployment, check that each demo page's HTML contains the measurement ID.
In a browser with analytics requests allowed, verify that the page loads
`www.googletagmanager.com/gtag/js?id=G-S88L6V684T` and sends Google Analytics
collection requests with the same ID and the demo's page URL. Confirm receipt
in GA4 Realtime, then verify the Hostname breakdown after the data is processed.
A hostname may only become selectable for a saved comparison after GA4 has
received events for it.

### Disposable demo data and deployment verification

Both hosted Jaeger demos contain synthetic traces generated continuously by
the demo applications and load generators. Historical trace data is disposable:
it may be discarded and regenerated, and preserving it is not a requirement for
routine maintenance.

The hosted `Deploy Jaeger Demo to OKE` workflow defaults to
`opensearch_recovery=waive` for scheduled, post-merge, and manual deployments,
including full-stack upgrades and manual dispatches from any branch. It skips
backup creation and restore testing. It does not automatically delete existing
data, run `clean`, or change the OpenSearch-before-Dashboards rollout order. An
OpenSearch upgrade still requires health waits, the OpenSearch/Dashboards
version check, and a fresh trace write/read; the Jaeger and application
components rely on rollout waits and public smoke checks, and OCI monitoring on
rollout waits alone. Extra cluster capacity solely for backup restoration is
not a prerequisite for routine demo maintenance.

If a waived upgrade leaves OpenSearch unhealthy, its revision is not recorded,
so scheduled and post-merge runs retry the same upgrade and block the dependent
OTel Jaeger and application updates until it succeeds. Because the data is
disposable, recover with a manual dispatch that selects `deploy_mode=clean` and
`demo_stack=otel`; set `image_tag=latest` if the dispatched commit has no
published snapshot image. It deletes the `opensearch`, `jaeger`, and
`otel-demo` namespaces, including the OpenSearch PVC and any namespaced
recovery snapshots and restored PVCs, and reinstalls the stack at the pinned
versions. The new PVC comes from the 10Gi claim template, so rerun the
`Expand OTel Demo OpenSearch Volume` workflow afterwards. A clean dispatch is
checked only by rollout waits and the public smoke checks and records no
revision; the next scheduled or post-merge run re-applies the pending OpenSearch
revision, runs the version and trace checks, and records it.

Standalone deployment scripts retain `required` as their default; explicitly
set `JAEGER_OTEL_DEMO_OPENSEARCH_RECOVERY=waive` when using them with disposable
data. With `required`, an upgrade whose scope includes OpenSearch does not
create a recovery point in `deploy-all.sh` itself. It only verifies that a
restore-tested snapshot of the live OpenSearch version and image, no older than
72 hours, already exists, so run `opensearch-recovery.sh backup-test` or the
recovery workflow first. Both need a running OpenSearch pod on an OCI Block
Volume; on a new or non-OCI cluster use `waive`.

### Optional backup and recovery testing

Select `opensearch_recovery=required` only when explicitly choosing to preserve
existing data. On the hosted workflow it is a manual dispatch input: scheduled
and post-merge runs always waive, so a merged OpenSearch update is deployed
before `required` can be selected. The policy applies only to upgrade-mode
deployments that include OpenSearch and does not protect data in `clean` mode.
The hosted deployment then creates a snapshot and requires an isolated restore
test against the exact live OpenSearch and Jaeger image digests. The manual
`Verify OTel Demo OpenSearch Recovery` workflow, which runs only from `main`,
can also run this test. It:

1. verifies that the live volume uses the OCI Block Volume CSI driver;
2. flushes OpenSearch and creates a retained `VolumeSnapshot`;
3. restores it to a new PVC;
4. boots the exact live OpenSearch and Jaeger image digests without a Service
   or ingress; and
5. validates cluster recovery, every pre-snapshot Jaeger index, template, and
   alias mapping, document floors, and representative service and trace queries
   before marking the snapshot tested.

Ingestion remains active, so the flushed provider backup is crash-consistent.
An isolated restore test is required before treating that backup as a tested
recovery point. Its pod requests 1.25 CPU and 6.5 GiB of memory, with limits
up to 8 GiB, and must run on a different worker from live OpenSearch. That
capacity is needed only when opting into backup/restore verification.

The snapshot and restored PVC are retained deliberately, so each run also holds
a full OCI backup and a volume of the live PVC's capacity. The workflow removes
only the exact run-specific disposable restore pod after the test, including on
failure, so it does not expose restored data or reserve capacity needed by the
live rollout. Removing either retained storage object is a separate operational
action. When existing data must be preserved, never use the demo's `clean` mode
as a database rollback. Restore the retained volume into a fresh installation
of the prior OpenSearch version instead. See the
[OpenSearch recovery procedure](./OPENSEARCH_RECOVERY.md) for both intact- and
replaced-namespace recovery.

With `waive`, historical data has no tested recovery path if it becomes
unusable. Discard it with the `clean` reinstall described above; the load
generators then regenerate traffic. The live OpenSearch workload still needs
sufficient memory and disk for normal operation.

### OpenSearch volume expansion

The manual `Expand OTel Demo OpenSearch Volume` workflow expands only the
existing OpenSearch PVC to 100Gi. It verifies the expected StatefulSet, pod,
PVC, StorageClass, and OCI CSI driver, then uses a preconditioned JSON Patch and
waits for both Kubernetes and OpenSearch to report the expanded filesystem. It
does not restart or replace the workload and is idempotent after a successful
expansion.

The live PVC intentionally remains larger than the 10Gi StatefulSet claim
template after this operation. Kubernetes does not permit updating that
template in place, and changing it during the OpenSearch Helm upgrade would
fail on an immutable StatefulSet field. Reconcile the template separately only
after the staged version upgrade and soak. PVC expansion cannot be rolled back
or shrunk.

### Index retention

Jaeger writes one OpenSearch index per day, and OpenSearch has no TTL. The
`esIndexCleaner` block in `jaeger-values.yaml` enables the Jaeger chart's
CronJob, which runs `jaeger-es-index-cleaner` daily and deletes
`jaeger-main-jaeger-{span,service,dependencies,sampling}-<date>` indices older
than 30 days. The `jaeger-archive-*` indices are not touched. The cleaner's
`index-prefix` must be `jaeger-main`, the same `index_prefix` in
`jaeger-config.yaml`. The pinned 2.21.0 cleaner appends a dash and then
matches `<prefix>jaeger-{span,service,dependencies,sampling}-<date>`.
`jaeger-values.test.sh` checks that the two stay in sync.

Without retention the data volume eventually passes the OpenSearch flood-stage
watermark (95% disk), which makes the indices read-only and stops trace
ingestion while every pod still reports `Running`. To size retention, keep
`numberOfDays` times the daily index size well under the volume.

The Jaeger deployment uses `jaeger-config.yaml` to configure the Jaeger v2
`jaeger_storage` extension against the in-cluster OpenSearch service. The Helm
chart storage type is set to `elasticsearch` for compatibility with the chart's
persistent-storage wiring, while the Jaeger runtime config points at OpenSearch.

## Clean-up

> **Warning:** This is not an upgrade path or a way to recover existing data.
> It removes namespaced recovery objects. If you opted into backup/restore
> verification and want to keep that recovery point, confirm that the retained
> `VolumeSnapshotContent` and its private recovery record are available before
> using it.

- Clean uninstall using cleanup.sh :
```bash path=null start=null
./cleanup.sh
```
- Manual teardown:
```bash path=null start=null
helm uninstall opensearch -n opensearch || true
helm uninstall opensearch-dashboards -n opensearch || true
helm uninstall jaeger -n jaeger || true
helm uninstall otel-demo -n otel-demo || true
kubectl delete namespace opensearch jaeger otel-demo --ignore-not-found=true
```
