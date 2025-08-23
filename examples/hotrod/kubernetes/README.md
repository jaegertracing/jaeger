# Hot R.O.D. - Rides on Demand on Kubernetes

Example deployment for the [hotrod app](..) using Helm charts in your Kubernetes environment (Kind, minikube, k3s, EKS, GKE).

## Prerequisites

- Kubernetes cluster
- Helm 3.x installed
- kubectl configured

## Usage

### Deploy with Default Configuration

```bash
cd examples/oci
./deploy-all.sh clean
```

### Deploy with Local Images (for development)

```bash
# For Jaeger v2 with local images
cd examples/oci
./deploy-all.sh local <image-tag>
```

### Deploy Modes

- **`upgrade`** (default): Upgrade existing deployment or install if not present
- **`local`**: Deploy using local registry images (localhost:5000)
- **`clean`**: Clean install (removes existing deployment first)

### Access Applications

After deployment completes, use port-forwarding:

```bash
# Jaeger UI
kubectl port-forward svc/jaeger-query 16686:16686

# HotROD application
kubectl port-forward svc/jaeger-hotrod 8080:80

# Prometheus (optional)
kubectl port-forward svc/prometheus 9090:9090

# Grafana (optional)
kubectl port-forward svc/prometheus-grafana 9091:80
```

Then access:
- 🔍 **Jaeger UI**: http://localhost:16686/jaeger
- 🚕 **HotROD App**: http://localhost:8080
- 📈 **Prometheus**: http://localhost:9090
- 📊 **Grafana**: http://localhost:9091

## Configuration

The deployment uses:
- **Helm charts** from [jaeger-helm-charts](https://github.com/jaegertracing/helm-charts)
- **Prometheus** for metrics collection
- **Load generator** for creating sample traces

Configuration files:
- `jaeger-values.yaml` - Jaeger Helm chart values
- `config.yaml` - Jaeger configuration
- `ui-config.json` - Jaeger UI configuration
- `monitoring-values.yaml` - Prometheus configuration
