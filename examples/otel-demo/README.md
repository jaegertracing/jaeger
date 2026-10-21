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
