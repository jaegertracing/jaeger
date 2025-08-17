#!/bin/bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

MODE="${1:-upgrade}"

if [[ "$MODE" == "upgrade" ]]; then
  HELM_JAEGER_CMD="upgrade --install --force"
  HELM_PROM_CMD="upgrade --install --force"
else
  echo "🟣 Clean mode: Uninstalling Jaeger and Prometheus..."
  helm uninstall jaeger --ignore-not-found || true
  helm uninstall prometheus --ignore-not-found || true
  for name in jaeger prometheus; do
    while helm list --filter "^${name}$" | grep "$name" &>/dev/null; do
      echo "Waiting for Helm release $name to be deleted..."
    done
  done
  HELM_JAEGER_CMD="install"
  HELM_PROM_CMD="install"
fi

# Clone Jaeger Helm Charts (v2 branch) if not already present
if [ ! -d "helm-charts" ]; then
  echo "📥 Cloning Jaeger Helm Charts..."
  git clone https://github.com/jaegertracing/helm-charts.git
  cd helm-charts
  git checkout v2
  echo "Adding required Helm repositories..."
  helm repo add bitnami https://charts.bitnami.com/bitnami
  helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
  helm repo add incubator https://charts.helm.sh/incubator
  helm repo update
  helm dependency build ./charts/jaeger
  cd ..
else
  echo "📁 Jaeger Helm Charts already exist. Skipping clone."
fi

# Navigate into examples/oci if not already in it
if [[ "$(basename $PWD)" != "oci" ]]; then
  if [ -d "./examples/oci" ]; then
    echo "📂 Changing to ./examples/oci directory..."
    cd ./examples/oci
  else
    echo "❌ Cannot find ./examples/oci directory. Exiting."
    exit 1
  fi
fi

echo "🟣 Deploying Jaeger..."
helm $HELM_JAEGER_CMD jaeger ./helm-charts/charts/jaeger \
  --set provisionDataStore.cassandra=false \
  --set allInOne.enabled=true \
  --set storage.type=memory \
  --set-file userconfig="./config.yaml" \
  --set-file uiconfig="./ui-config.json" \
  -f ./jaeger-values.yaml

echo "🟢 Deploying Prometheus..."
kubectl apply -f prometheus-svc.yaml
helm $HELM_PROM_CMD prometheus -f monitoring-values.yaml prometheus-community/kube-prometheus-stack

# Create ConfigMap for Trace Generator
echo "🔵 Step 3: Creating ConfigMap for Trace Generator..."
kubectl create configmap trace-script --from-file=./load-generator/generate_traces.py --dry-run=client -o yaml | kubectl apply -f -

# Deploy Trace Generator Pod
echo "🟡 Step 4: Deploying Trace Generator Pod..."
kubectl apply -f ./load-generator/load-generator.yaml

# Deploy ingress changes 
echo "🟡 Step 5: Deploying Ingress Resource..."
kubectl apply -f ingress.yaml

# Output Port-forward Instructions
echo "✅ Deployment Complete!"
echo ""
echo "📡 Port-forward the following to access UIs locally:"
echo ""
echo "kubectl port-forward svc/jaeger-query 16686:16686      # Jaeger UI"
echo "kubectl port-forward svc/prometheus 9090:9090          # Prometheus UI"
echo "kubectl port-forward svc/prometheus-grafana 9091:80    # Grafana UI"
echo "kubectl port-forward svc/jaeger-hotrod 8080:80         # HotROD UI"
echo ""
echo "Then open:"
echo "🔍 Jaeger: http://localhost:16686/jaeger"
echo "📈 Prometheus: http://localhost:9090"
echo "📊 Grafana: http://localhost:9091"
echo "🚕 HotROD: http://localhost:8080"