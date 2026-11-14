# OpenTelemetry Demo app + HotRODapp + Jaeger + OpenSearch

This example provides a one-command deployment of a complete observability stack on Kubernetes:
- Jaeger (all-in-one) for tracing
- OpenSearch and OpenSearch Dashboards
- OpenTelemetry Demo application (multi-service web store)
- HotRod application

It is driven by `deploy-all.sh`, which supports both clean installs and upgrades.

## Collector batching migration

OpenTelemetry has [deprecated the `batch` processor](https://opentelemetry.io/blog/2026/collector-batching-migration/),
with exporter batching defaults planned for v0.171.0 (February 2027) and processor
removal for v0.177.0 (May 2027). Both ingestion stages in this demo use explicit
exporter batching: the demo Collector uses `otlp/jaeger.sending_queue.batch`, and
Jaeger uses `jaeger_storage_exporter.queue.batch`.

Jaeger's queue retains one consumer, 10 queued requests, blocking overflow, and
an 8192-span or 200ms flush threshold. The demo Collector retains its OTLP
exporter's queue defaults (1000 requests and 10 consumers). These queue limits
count requests before batching; review capacity when migrating from a queue
that previously received already-batched requests. Jaeger acknowledges enqueue,
so this pipeline does not guarantee a durable OpenSearch write before success.

Deploy matching images and configurations, then reconcile both the `jaeger` and
`otel-demo` scopes through the existing deployment workflow. Verify new demo
traces can be queried and that SPM metrics still work. Roll back the affected
Helm release to its previous image and configuration on a failed smoke check.
Existing operator configurations using `batch` remain supported during migration.

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

### OpenSearch recovery gate

The isolated OpenSearch upgrade scope fails closed unless a recent OCI CSI
volume snapshot has passed an isolated restore test against the exact live
OpenSearch image. Run the manual `Verify OTel Demo OpenSearch Recovery`
workflow before an OpenSearch version upgrade. The workflow:

1. verifies that the live volume uses the OCI Block Volume CSI driver;
2. flushes OpenSearch and creates a retained `VolumeSnapshot`;
3. restores it to a new PVC;
4. boots the exact live OpenSearch and Jaeger image digests without a Service
   or ingress; and
5. validates cluster recovery, every pre-snapshot Jaeger index, template, and
   alias mapping, document floors, and representative service and trace queries
   before marking the snapshot tested.

Ingestion remains active, so the flushed provider backup is crash-consistent,
not application-consistent. The isolated restore test is therefore mandatory.
The restore pod requests the target OpenSearch memory sizing and must run on a
different worker from the live OpenSearch pod, so it also proves that current
cluster capacity can host the recovery test without colocating both databases.

The snapshot and restored PVC are retained deliberately. The workflow removes
only the exact run-specific disposable restore pod after the test, including on
failure, so it does not expose restored data or reserve capacity needed by the
live rollout. Removing either retained storage object is a separate operational
action. Never use the demo's `clean` mode as a database rollback. Restore the
retained volume into a fresh installation of the prior OpenSearch version
instead. See the
[OpenSearch recovery procedure](./OPENSEARCH_RECOVERY.md) for both intact- and
replaced-namespace recovery.

If the trace data is disposable, a maintainer may explicitly select
`opensearch_recovery=waive` for a manual workflow dispatch from `main` whose
other inputs are exactly `demo_stack=otel`, `deploy_mode=upgrade`, and
`otel_deploy_scope=opensearch`. No other event, branch, stack, mode, or scope
accepts the waiver. This skips only the recovery-point check: it does not run
`clean`, delete data or storage, change the OpenSearch-before-Dashboards rollout
order, or remove health waits. The waived upgrade has no tested rollback or
data-recovery path if the existing persistent data becomes unusable.

The waiver does not waive cluster-capacity checks. Confirm sufficient memory
and disk headroom separately before dispatching the upgrade.

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

> **Warning:** This is not an upgrade or recovery path. It removes namespaced
> recovery objects. Confirm that the retained `VolumeSnapshotContent` and its
> private recovery record are available before using it.

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
