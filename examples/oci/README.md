# Jaeger + Prometheus + HotROD Demo Setup (Helm v2 Branch)

This guide walks you through deploying **Jaeger** (using the v2 Helm chart), **Prometheus**, and the **HotROD demo app** on Kubernetes.

## Prerequisites

Ensure the following tools are installed and configured:

- A Kubernetes cluster (e.g., Minikube, kind, or cloud-based)
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/)
- [`Helm 3`](https://helm.sh/docs/intro/install/)
- `git`

---

## 1. Clone Jaeger Helm Charts (v2 Branch)

```bash
git clone https://github.com/jaegertracing/helm-charts.git
cd helm-charts
git checkout v2
```
After cloning, you must install chart dependencies to avoid the depedency missing error using this command :
``` bash
helm dependency build ./charts/jaeger
```
---

## 2. Prepare Your Values and Config Files

* `config.yaml`: Configuration file for the Jaeger Collector (in Jaeger binary mode).

  * Currently configured to:

    * Store **traces** in **memory**
    * Export **metrics** to **Prometheus**

* `prometheus.yml`: Configuration for Prometheus scrape targets.

---

## 3. Create ConfigMap for Prometheus

Create a ConfigMap to mount the Prometheus configuration:

```bash
kubectl create configmap prometheus-config --from-file=prometheus.yml=./prometheus.yml
```

---

## 4. Deploy Jaeger (All-in-One Mode with Memory Storage)

Ensure you're in the correct directory 
```bash
cd ./examples/oci/
```
Install the Jaeger Helm chart with memory storage and a custom config:

```bash
helm install jaeger /path/to/charts/jaeger \
  --set provisionDataStore.cassandra=false \
  --set allInOne.enabled=true \
  --set storage.type=memory \
  --set-file userconfig="./config.yaml" \
  -f ./jaeger-values.yaml
```

> 🔁 Adjust the path to match your local directory structure.

---

## 5. Deploy Prometheus (for SPM Metrics)

Apply the Prometheus deployment and service:

```bash
kubectl apply -f ./prometheus-deploy.yaml
```

This enables **Span Metrics Processor (SPM)** functionality in Jaeger.

---

## 6. Port Forward Services for Local Access

Use the following port-forward commands to access the UIs locally:

```bash
# Jaeger UI
kubectl port-forward svc/jaeger-query 16686:16686

# Prometheus UI
kubectl port-forward svc/prometheus 9090:9090

# HotROD UI
kubectl port-forward svc/jaeger-hotrod 8080:80
```

Then open in browser:

* 🔍 Jaeger: [http://localhost:16686](http://localhost:16686)
* 📈 Prometheus: [http://localhost:9090](http://localhost:9090)
* 🚕 HotROD: [http://localhost:8080](http://localhost:8080)

🔧 Remarks

📌 The current configuration is set to run in the default namespace.
You can use any custom namespace by making minor adjustments in:
``` bash
Helm --namespace flags
Kubernetes manifests (metadata.namespace)
Prometheus scrape configs and service selectors if targeting Jaeger in a different namespace
```
