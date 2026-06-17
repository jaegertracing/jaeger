#!/bin/bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

MODE="${1:-upgrade}"
IMAGE_TAG="${2:-${JAEGER_DEMO_IMAGE_TAG:-}}"
LOCAL_IMAGE_TAG="${2:-${JAEGER_DEMO_IMAGE_TAG:-latest}}"
JAEGER_IMAGE_REPOSITORY="${JAEGER_DEMO_JAEGER_IMAGE_REPOSITORY:-jaegertracing/jaeger}"
HOTROD_IMAGE_REPOSITORY="${JAEGER_DEMO_HOTROD_IMAGE_REPOSITORY:-jaegertracing/example-hotrod}"
JAEGER_IMAGE_TAG="${JAEGER_DEMO_JAEGER_IMAGE_TAG:-$IMAGE_TAG}"
HOTROD_IMAGE_TAG="${JAEGER_DEMO_HOTROD_IMAGE_TAG:-$IMAGE_TAG}"
IMAGE_PULL_POLICY="${JAEGER_DEMO_IMAGE_PULL_POLICY:-IfNotPresent}"
PUBLIC_BASE_URL="${JAEGER_DEMO_PUBLIC_BASE_URL:-https://demo.jaegertracing.io}"
RUN_PUBLIC_SMOKE_TESTS="${RUN_PUBLIC_SMOKE_TESTS:-false}"
PROMETHEUS_STACK_CHART="prometheus-community/kube-prometheus-stack"
PROMETHEUS_STACK_CHART_VERSION="${PROMETHEUS_STACK_CHART_VERSION:-82.10.4}"
CLEAN_UNINSTALL_TIMEOUT="${JAEGER_DEMO_CLEAN_UNINSTALL_TIMEOUT:-10m0s}"
DEPLOY_PROMETHEUS=true

uninstall_release() {
  local name=$1

  if ! helm uninstall "$name" --ignore-not-found --wait --timeout "$CLEAN_UNINSTALL_TIMEOUT"; then
    echo "❌ Failed to uninstall Helm release $name within $CLEAN_UNINSTALL_TIMEOUT"
    helm status "$name" || true
    helm list --all --filter "^${name}$" || true
    return 1
  fi
}

release_status() {
  local output

  output=$(helm status "$1" 2>/dev/null || true)
  awk -F': ' '$1 == "STATUS" { print $2 }' <<< "$output"
}

prepare_upgrade_release() {
  local name=$1
  local status

  status=$(release_status "$name")
  case "$status" in
    ""|deployed)
      ;;
    *)
      echo "🟡 Helm release $name is in '$status' state. Reinstalling before upgrade."
      uninstall_release "$name"
      ;;
  esac
}

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
  prepare_upgrade_release jaeger
  PROMETHEUS_RELEASE_STATUS=$(release_status prometheus)
  case "$PROMETHEUS_RELEASE_STATUS" in
    deployed)
      DEPLOY_PROMETHEUS=true
      ;;
    "")
      DEPLOY_PROMETHEUS=false
      echo "🟡 Prometheus Helm release is not installed. Skipping Prometheus in upgrade mode."
      ;;
    *)
      DEPLOY_PROMETHEUS=false
      echo "🟡 Prometheus Helm release is in '$PROMETHEUS_RELEASE_STATUS' state. Skipping Prometheus in upgrade mode."
      echo "🟡 Use clean mode or repair the Prometheus release separately to reinstall monitoring."
      ;;
  esac
  HELM_JAEGER_CMD="upgrade --install --wait"
  HELM_PROM_CMD="upgrade --install --wait"
else
  echo "🟣 Clean mode: Uninstalling Jaeger and Prometheus..."
  uninstall_release jaeger
  uninstall_release prometheus
  HELM_JAEGER_CMD="install --wait"
  HELM_PROM_CMD="install --wait"
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
    --set global.imageRegistry="" \
    --set allInOne.image.repository="localhost:5000/jaegertracing/jaeger" \
    --set allInOne.image.tag="${LOCAL_IMAGE_TAG}"  \
    --set allInOne.image.pullPolicy="Never" \
    --set hotrod.image.repository="localhost:5000/jaegertracing/example-hotrod" \
    --set hotrod.image.tag="${LOCAL_IMAGE_TAG}"  \
    --set hotrod.image.pullPolicy="Never" \
    --set-file userconfig="./config.yaml" \
    --set-file uiconfig="./ui-config.json" \
    -f ./jaeger-values.yaml
else
  image_args=(
    --set allInOne.image.repository="${JAEGER_IMAGE_REPOSITORY}"
    --set allInOne.image.pullPolicy="${IMAGE_PULL_POLICY}"
    --set hotrod.image.repository="${HOTROD_IMAGE_REPOSITORY}"
    --set hotrod.image.pullPolicy="${IMAGE_PULL_POLICY}"
  )
  if [[ -n "$JAEGER_IMAGE_TAG" ]]; then
    image_args+=(--set allInOne.image.tag="${JAEGER_IMAGE_TAG}")
  fi
  if [[ -n "$HOTROD_IMAGE_TAG" ]]; then
    image_args+=(--set hotrod.image.tag="${HOTROD_IMAGE_TAG}")
  fi

  echo "🟣 Deploying Jaeger image ${JAEGER_IMAGE_REPOSITORY}:${JAEGER_IMAGE_TAG:-chart-default}"
  echo "🟣 Deploying HotROD image ${HOTROD_IMAGE_REPOSITORY}:${HOTROD_IMAGE_TAG:-values-default}"

  helm $HELM_JAEGER_CMD --timeout 10m0s jaeger ./helm-charts/charts/jaeger \
    --set provisionDataStore.cassandra=false \
    --set allInOne.enabled=true \
    --set storage.type=memory \
    "${image_args[@]}" \
    --set-file userconfig="./config.yaml" \
    --set-file uiconfig="./ui-config.json" \
    -f ./jaeger-values.yaml
fi

echo "🟢 Deploying Prometheus..."
kubectl apply -f prometheus-svc.yaml
if [[ "$DEPLOY_PROMETHEUS" == "true" ]]; then
  helm $HELM_PROM_CMD --timeout 10m0s prometheus "$PROMETHEUS_STACK_CHART" \
    --version "$PROMETHEUS_STACK_CHART_VERSION" \
    --set crds.upgradeJob.enabled=true \
    --set crds.upgradeJob.forceConflicts=true \
    -f monitoring-values.yaml
else
  echo "🟡 Skipped Prometheus Helm deployment."
fi

# Create ConfigMap for Trace Generator
echo "🔵 Step 3: Creating ConfigMap for Trace Generator..."
kubectl create configmap trace-script --from-file=./load-generator/generate_traces.py --dry-run=client -o yaml | kubectl apply -f -

# Deploy Trace Generator Pod
echo "🟡 Step 4: Deploying Trace Generator Pod..."
kubectl apply -f ./load-generator/load-generator.yaml

# Deploy ingress changes
echo "🟡 Step 5: Deploying Ingress Resource..."
kubectl apply -f ingress.yaml

echo "🔎 Step 6: Verifying rollout status..."
kubectl rollout status deployment/jaeger --timeout=5m
kubectl rollout status deployment/jaeger-hotrod --timeout=5m
kubectl rollout status deployment/trace-generator --timeout=5m
kubectl get pods,svc,ingress

smoke_expect() {
  local url=$1
  local expected=$2
  local output=$3

  for attempt in $(seq 1 12); do
    if curl -fsS "$url" -o "$output" && grep -q "$expected" "$output"; then
      echo "✅ Smoke check passed: $url"
      return 0
    fi
    echo "Waiting for $url to return expected content (attempt $attempt/12)..."
    sleep 10
  done

  echo "❌ Smoke check failed: $url"
  echo "Expected content: $expected"
  echo "Last response:"
  cat "$output" 2>/dev/null || true
  return 1
}

if [[ "$RUN_PUBLIC_SMOKE_TESTS" == "true" ]]; then
  echo "🔎 Step 7: Verifying public demo endpoints..."
  smoke_expect "${PUBLIC_BASE_URL}/hotrod/dispatch?customer=123" '"Driver"' /tmp/hotrod-smoke.json
  smoke_expect "${PUBLIC_BASE_URL}/jaeger/api/services" '"frontend"' /tmp/jaeger-services.json
fi

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
