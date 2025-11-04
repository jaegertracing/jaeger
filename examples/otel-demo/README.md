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

```bash path=null start=null
ROLLOUT_TIMEOUT=900 ./deploy-all.sh clean
```

## What gets deployed
- Namespace `opensearch`:
  - OpenSearch (single node) StatefulSet
  - OpenSearch Dashboards Deployment
- Namespace `jaeger`:
  - Jaeger all-in-one Deployment (storage=none)
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


## Customization
- Helm values provided in this directory:
  - `opensearch-values.yaml`
  - `opensearch-dashboard-values.yaml`
  - `jaeger-values.yaml`
  - `jaeger-config.yaml`
  - `otel-demo-values.yaml`
  - `jaeger-query-service.yaml`

You can adjust these files and re-run `./deploy-all.sh upgrade` to apply changes.

## Clean-up
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



