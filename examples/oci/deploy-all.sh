#!/bin/bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

MODE="${1:-upgrade}"
IMAGE_TAG="${2:-latest}"

case "$MODE" in
  upgrade|clean|local)
    echo "🔵 Running in '$MODE' mode..."
    ;;
  *)
    echo "❌ Error: Invalid mode '$MODE'"
    echo "Usage: $0 [upgrade|clean|local] [image-tag]"
    echo ""
    echo "Modes:"
    echo "  upgrade  - Upgrade existing deployment or install if not present (default)"
    echo "  clean    - Clean install (removes existing deployment first)"
    echo "  local    - Deploy using local registry images (localhost:5000)"
    echo ""
    echo "Examples:"
    echo "  $0                    # Upgrade mode with latest tag"
    echo "  $0 clean              # Clean install"
    echo "  $0 local <image_tag>       # Local mode with specific image tag"
    exit 1
    ;;
esac

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

# Navigate to the script's directory (examples/oci)
cd $(dirname $0)

# Clone Jaeger Helm Charts if not already present
if [ ! -d "helm-charts" ]; then
  echo "📥 Cloning Jaeger Helm Charts..."
  git clone https://github.com/jaegertracing/helm-charts.git
  cd helm-charts
  echo "Using v2 branch for Jaeger v2..."
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

# Set image repositories and deploy based on mode
if [[ "$MODE" == "local" ]]; then

  echo "🟣 Deploying Jaeger with local registry images..."
  helm $HELM_JAEGER_CMD --timeout 10m0s jaeger ./helm-charts/charts/jaeger \
    --set provisionDataStore.cassandra=false \
    --set allInOne.enabled=true \
    --set storage.type=memory \
    --set hotrod.enabled=true \
    --set allInOne.image.repository="localhost:5000/jaegertracing/jaeger" \
    --set allInOne.image.tag="${IMAGE_TAG}"  \
    --set allInOne.image.pullPolicy="Never" \
    --set hotrod.image.repository="localhost:5000/jaegertracing/example-hotrod" \
    --set hotrod.image.tag="${IMAGE_TAG}"  \
    --set hotrod.image.pullPolicy="Never" \
    --set-file userconfig="./config.yaml" \
    --set-file uiconfig="./ui-config.json" \
    -f ./jaeger-values.yaml
else
  echo "🟣 Deploying Jaeger..."
  helm $HELM_JAEGER_CMD --timeout 10m0s jaeger ./helm-charts/charts/jaeger \
    --set provisionDataStore.cassandra=false \
    --set allInOne.enabled=true \
    --set storage.type=memory \
    --set allInOne.image.repository="jaegertracing/jaeger" \
    --set-file userconfig="./config.yaml" \
    --set-file uiconfig="./ui-config.json" \
    -f ./jaeger-values.yaml
fi

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
echo ""
echo "📝 Note: If you made changes to Jaeger configuration files (e.g., config.yaml, ui-config.json), you may need to run this script in clean mode:"
echo "    ./deploy-all.sh clean"
echo "Or manually restart the CI workflow to ensure your changes are applied."
