#!/bin/bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODE="${1:-upgrade}"
DEPLOY_SCOPE="${JAEGER_DEMO_DEPLOY_SCOPE:-${JAEGER_OTEL_DEMO_DEPLOY_SCOPE:-all}}"
case "$DEPLOY_SCOPE" in
  jaeger|monitoring|all) ;;
  *) echo "Invalid OCI deploy scope: $DEPLOY_SCOPE" >&2; exit 1 ;;
esac
if [[ "$MODE" != "upgrade" ]]; then
  DEPLOY_SCOPE=all
fi

deploy_jaeger() { [[ "$DEPLOY_SCOPE" == "jaeger" || "$DEPLOY_SCOPE" == "all" ]]; }
deploy_monitoring() { [[ "$DEPLOY_SCOPE" == "monitoring" || "$DEPLOY_SCOPE" == "all" ]]; }
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
# renovate: datasource=helm depName=kube-prometheus-stack registryUrl=https://prometheus-community.github.io/helm-charts
PROMETHEUS_STACK_CHART_VERSION="${PROMETHEUS_STACK_CHART_VERSION:-82.10.4}"
CLEAN_UNINSTALL_TIMEOUT="${JAEGER_DEMO_CLEAN_UNINSTALL_TIMEOUT:-10m0s}"

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

  if ! output=$(helm status "$1" 2>&1); then
    if grep -q "release: not found" <<< "$output"; then
      return 0
    fi
    echo "$output" >&2
    return 1
  fi
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
  if deploy_jaeger; then
    prepare_upgrade_release jaeger
  fi
  if deploy_monitoring; then
    PROMETHEUS_RELEASE_STATUS=$(release_status prometheus)
    # Unlike Jaeger, a failed Prometheus release is upgraded in place instead of
    # being reinstalled: its pods usually recover after the readiness timeout
    # that failed the release, and an upgrade leaves them running. Helm refuses
    # to upgrade while another operation is pending.
    case "$PROMETHEUS_RELEASE_STATUS" in
      deployed|failed) ;;
      *)
        echo "Prometheus cannot be upgraded (status: ${PROMETHEUS_RELEASE_STATUS:-missing}); repair it separately before upgrading" >&2
        exit 1
        ;;
    esac
  fi
  HELM_JAEGER_CMD="upgrade --install --wait --atomic"
  HELM_PROM_CMD="upgrade --install --wait"
else
  echo "🟣 Clean mode: Uninstalling Jaeger and Prometheus..."
  uninstall_release jaeger
  uninstall_release prometheus
  HELM_JAEGER_CMD="install --wait"
  HELM_PROM_CMD="install --wait"
fi

# Navigate to the script's directory (examples/oci)
cd "$SCRIPT_DIR"

if deploy_jaeger; then
  # shellcheck source=scripts/demo/jaeger-chart.sh
  source "$SCRIPT_DIR/../../scripts/demo/jaeger-chart.sh"
  prepare_jaeger_chart "$SCRIPT_DIR/helm-charts"
fi

if deploy_jaeger; then
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

fi

if deploy_monitoring; then
  helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
  helm repo update prometheus-community
  echo "🟢 Deploying Prometheus..."
  kubectl apply -f prometheus-svc.yaml
  helm $HELM_PROM_CMD --timeout 20m0s prometheus "$PROMETHEUS_STACK_CHART" \
    --version "$PROMETHEUS_STACK_CHART_VERSION" \
    --set crds.upgradeJob.enabled=true \
    --set crds.upgradeJob.forceConflicts=true \
    -f monitoring-values.yaml
fi

if deploy_jaeger; then
  # Create ConfigMap for Trace Generator
  echo "🔵 Step 3: Creating ConfigMap for Trace Generator..."
  kubectl create configmap trace-script --from-file=./load-generator/generate_traces.py --from-file=./load-generator/requirements.txt --dry-run=client -o yaml | kubectl apply -f -

  # Deploy Trace Generator Pod
  echo "🟡 Step 4: Deploying Trace Generator Pod..."
  kubectl apply -f ./load-generator/load-generator.yaml

  # Deploy ingress changes
  echo "🟡 Step 5: Deploying Ingress Resource..."
  kubectl apply -f ingress.yaml

fi

echo "🔎 Step 6: Verifying rollout status..."
if deploy_jaeger; then
  kubectl rollout status deployment/jaeger --timeout=5m
  kubectl rollout status deployment/jaeger-hotrod --timeout=5m
  kubectl rollout restart deployment/trace-generator
  kubectl rollout status deployment/trace-generator --timeout=5m
fi
if deploy_monitoring; then
  kubectl rollout status deployment/prometheus-grafana --timeout=5m
fi
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

if [[ "$RUN_PUBLIC_SMOKE_TESTS" == "true" ]] && deploy_jaeger; then
  echo "🔎 Step 7: Verifying public demo endpoints..."
  smoke_expect "${PUBLIC_BASE_URL}/hotrod/dispatch?customer=123" '"Driver"' /tmp/hotrod-smoke.json
  smoke_expect "${PUBLIC_BASE_URL}/jaeger/api/v3/services" '"frontend"' /tmp/jaeger-services.json
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
