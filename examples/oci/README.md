# Jaeger + Prometheus + HotROD Demo Setup (Helm v2 Branch)

This guide walks you through deploying **Jaeger** (using the v2 Helm chart), **Prometheus**, and the **HotROD demo app** on Kubernetes.

## Prerequisites

Ensure the following tools are installed and configured:

- A Kubernetes cluster (e.g., Minikube, kind, or cloud-based)
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/)
- [`Helm 3`](https://helm.sh/docs/intro/install/)
- `git`

---

## Deploy the Jaeger Demo Setup

The following components are deployed as part of the Jaeger demo setup:

- **Jaeger All-in-One**: Tracing backend (collector, query, UI, agent in one pod)
- **HotROD Demo App**: Sample microservices application for tracing demonstration
- **Prometheus Monitoring Stack**: Includes Prometheus, Grafana, and Alertmanager for metrics and dashboards
- **Load Generator**: Continuously generates traces from the HotROD app

To deploy the entire infrastructure with a single command, run:

```bash
bash ./deploy-all.sh
```
This script will automatically install and configure all components on your Kubernetes , To deal with individual components refer to deploy-all.sh script . 

## Access the Deployment

After deploying, you can access each component locally using the following port-forward commands in separate terminals:

```bash
# Jaeger UI
kubectl port-forward svc/jaeger-query 16686:16686

# Prometheus UI
kubectl port-forward svc/prometheus 9090:9090

# Grafana Dashboard
kubectl port-forward svc/prometheus-grafana 9091:80

# HotROD UI
kubectl port-forward svc/jaeger-hotrod 8080:80
```

Then, open the following URLs in your browser:

- **Jaeger UI:** [http://localhost:16686](http://localhost:16686)
- **Prometheus:** [http://localhost:9090](http://localhost:9090)
- **Grafana:** [http://localhost:9091](http://localhost:9091)
- **HotROD Demo App:** [http://localhost:8080](http://localhost:8080)

🔧 Remarks

📌 The current configuration is set to run in the default namespace.
You can use any custom namespace by making minor adjustments in:
``` bash
Helm --namespace flags
Kubernetes manifests (metadata.namespace)
Prometheus scrape configs and service selectors if targeting Jaeger in a different namespace
```
📌 The default credentials for Grafana dashboards are:

- **Username:** `admin`
- **Password:** `prom-operator`

Once logged in, you can explore the pre-built dashboards or add your own tracing and metrics visualizations.