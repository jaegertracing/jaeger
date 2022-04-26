# Hot R.O.D. - Rides on Demand on Kubernetes

Example k8s manifests for deploying the [hotrod app](../hotrod) to your k8s environment of choice. e.g. minikube, k3s, EKS, GKE

## Features

- Optional [example configuration](kustomize.yaml) for using [grafana-agent-traces](https://grafana.com/docs/grafana-cloud/traces/set-up-and-use-tempo/) as a backend.

## Deploy with Kustomize

```
kustomize build . | kubectl apply -f -
```
