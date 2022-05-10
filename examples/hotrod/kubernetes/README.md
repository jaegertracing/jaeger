# Hot R.O.D. - Rides on Demand on Kubernetes

Example k8s manifests for deploying the [hotrod app](..) to your k8s environment of choice. e.g. minikube, k3s, EKS, GKE

## Usage

```bash
kustomize build . | kubectl apply -f -
kubectl port-forward -n example-hotrod service/example-hotrod 8080:frontend
# In another terminal
kubectl port-forward -n example-hotrod service/jaeger 16686:frontend

# To cleanup
kustomize build . | kubectl delete -f -
```

Access Jaeger UI at <http://localhost:16686> and HotROD app at <http://localhost:8080>
