#!/usr/bin/env bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROLLOUT_TIMEOUT="${ROLLOUT_TIMEOUT:-600}"

MODE="${1:-upgrade}"
IMAGE_TAG="${2:-latest}"

case "$MODE" in
  upgrade|clean)
    echo " Running in '$MODE' mode..."
    ;;
  *)
    echo "Error: Invalid mode '$MODE'"
    echo "Usage: $0 [upgrade|clean] [image-tag]"
    echo ""
    echo "Modes:"
    echo "  upgrade  - Upgrade existing deployment or install if not present (default)"
    echo "  clean    - Clean install (removes existing deployment first)"
    echo ""
    echo "Examples:"
    echo "  $0                    # Upgrade mode with latest tag"
    echo "  $0 clean              # Clean install"
    exit 1
    ;;
 esac

if [[ "$MODE" == "upgrade" ]]; then
  HELM_JAEGER_CMD="upgrade --install --force"
else
  # For clean mode, use install after cleanup
  HELM_JAEGER_CMD="install"
fi

log() { echo "[$(date +"%F %T")] $*"; }
err() { echo "[$(date +"%F %T")] ERROR: $*" >&2; exit 1; }

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    err "$1 is required but not installed"
  fi
}

check_cluster() {
  if ! kubectl cluster-info >/dev/null 2>&1; then
    err "Cannot reach a Kubernetes cluster with kubectl"
  fi
}

check_required_files() {
  local files=(
    "$SCRIPT_DIR/opensearch-values.yaml"
    "$SCRIPT_DIR/opensearch-dashboard-values.yaml"
    "$SCRIPT_DIR/jaeger-values.yaml"
    "$SCRIPT_DIR/jaeger-config.yaml"
    "$SCRIPT_DIR/otel-demo-values.yaml"
    "$SCRIPT_DIR/jaeger-query-service.yaml"
  )
  for f in "${files[@]}"; do
    [[ -f "$f" ]] || err "Missing required file: $f"
  done
}

wait_for_deployment() {
  local namespace="$1"
  local deployment="$2"
  local timeout="${3:-${ROLLOUT_TIMEOUT}s}"
  log "Waiting for deployment $deployment in $namespace..."
  if ! kubectl rollout status "deployment/$deployment" -n "$namespace" --timeout="$timeout"; then
    kubectl -n "$namespace" get deploy "$deployment" -o wide || true
    kubectl -n "$namespace" describe deploy "$deployment" || true
    kubectl -n "$namespace" get pods -l app.kubernetes.io/name="$deployment" -o wide || true
    err "Deployment $deployment failed to become ready in $namespace"
  fi
  log "Deployment $deployment is ready"
}

wait_for_statefulset() {
  local namespace="$1"
  local sts="$2"
  local timeout="${3:-${ROLLOUT_TIMEOUT}s}"
  log "Waiting for statefulset $sts in $namespace..."
  if ! kubectl rollout status "statefulset/$sts" -n "$namespace" --timeout="$timeout"; then
    kubectl -n "$namespace" get statefulset "$sts" -o wide || true
    kubectl -n "$namespace" describe statefulset "$sts" || true
    kubectl -n "$namespace" get pods -l statefulset.kubernetes.io/pod-name -o wide || true
    err "StatefulSet $sts failed to become ready in $namespace"
  fi
  log "StatefulSet $sts is ready"
}

cleanup() {
  log "Cleanup: uninstalling existing releases if present"
  helm uninstall opensearch -n opensearch >/dev/null 2>&1 || true
  helm uninstall opensearch-dashboards -n opensearch >/dev/null 2>&1 || true
  helm uninstall jaeger -n jaeger >/dev/null 2>&1 || true
  helm uninstall otel-demo -n otel-demo >/dev/null 2>&1 || true

  log "Cleanup: deleting namespaces (may take time)"
  for ns in jaeger otel-demo opensearch; do
    kubectl delete namespace "$ns" --ignore-not-found=true >/dev/null 2>&1 || true
  done

  # Wait for namespaces to disappear
  for ns in jaeger otel-demo opensearch; do
    for i in {1..120}; do
      if ! kubectl get namespace "$ns" >/dev/null 2>&1; then
        break
      fi
      sleep 2
    done
  done
  log "Cleanup complete"
}

# Clone Jaeger Helm chart and prepare dependencies
clone_jaeger_v2() {
  local dest="$SCRIPT_DIR/helm-charts"
  if [[ ! -d "$dest" ]]; then
    log "Cloning Jaeger Helm Charts..."
    git clone https://github.com/jaegertracing/helm-charts.git "$dest"
    (
      cd "$dest"
      log "Using v2 branch for Jaeger v2..."
      git checkout v2
      log "Adding required Helm repositories..."
      helm repo add bitnami https://charts.bitnami.com/bitnami >/dev/null 2>&1 || true
      helm repo add prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null 2>&1 || true
      helm repo add incubator https://charts.helm.sh/incubator >/dev/null 2>&1 || true
      helm repo update >/dev/null
      helm dependency build ./charts/jaeger
    )
  else
    log "Jaeger Helm Charts already exist. Skipping clone."
    # Ensure required repos exist even if charts folder already exists
    helm repo add bitnami https://charts.bitnami.com/bitnami >/dev/null 2>&1 || true
    helm repo add prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null 2>&1 || true
    helm repo add incubator https://charts.helm.sh/incubator >/dev/null 2>&1 || true
    helm repo update >/dev/null
  fi
}



main() {
  log "Starting CI deploy (weekly refresh)"
  need bash
  need git
  need curl
  need kubectl
  need helm
  check_required_files
  check_cluster


  if [[ "$MODE" == "clean" ]]; then
    cleanup
  fi

  log "Adding/updating Helm repos"
  helm repo add opensearch https://opensearch-project.github.io/helm-charts >/dev/null 2>&1 || true
  helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts >/dev/null 2>&1 || true
  helm repo add jaegertracing https://jaegertracing.github.io/helm-charts >/dev/null 2>&1 || true
  helm repo update >/dev/null
  clone_jaeger_v2

  log "Deploying OpenSearch"
  helm upgrade --install opensearch opensearch/opensearch \
    --namespace opensearch --create-namespace \
    --version 2.19.0 \
    --set image.tag=2.11.0 \
    -f "$SCRIPT_DIR/opensearch-values.yaml" \
    --wait --timeout 10m
  wait_for_statefulset opensearch opensearch-cluster-single "${ROLLOUT_TIMEOUT}s"

  log "Deploying OpenSearch Dashboards"
  helm upgrade --install opensearch-dashboards opensearch/opensearch-dashboards \
    --namespace opensearch \
    -f "$SCRIPT_DIR/opensearch-dashboard-values.yaml" \
    --wait --timeout 10m
  wait_for_deployment opensearch opensearch-dashboards "${ROLLOUT_TIMEOUT}s"

  
  log "Deploying Jaeger (all-in-one, no storage)"
  helm $HELM_JAEGER_CMD jaeger "$SCRIPT_DIR/helm-charts/charts/jaeger" \
    --namespace jaeger --create-namespace \
    --set allInOne.enabled=true \
    --set storage.type=none \
    --set allInOne.image.repository=jaegertracing/jaeger \
    --set allInOne.image.tag="${IMAGE_TAG}" \
    --set-file userconfig="$SCRIPT_DIR/jaeger-config.yaml" \
    -f "$SCRIPT_DIR/jaeger-values.yaml" \
    --wait --timeout 10m
  wait_for_deployment jaeger jaeger "${ROLLOUT_TIMEOUT}s"

  
  log "Creating Jaeger query ClusterIP service..."
  kubectl apply -n jaeger -f "$SCRIPT_DIR/jaeger-query-service.yaml"
  log "Jaeger query ClusterIP service created"

  log "Deploying OpenTelemetry Demo"
  helm upgrade --install otel-demo open-telemetry/opentelemetry-demo \
    -f "$SCRIPT_DIR/otel-demo-values.yaml" \
    --namespace otel-demo --create-namespace \
    --wait --timeout 15m
  wait_for_deployment otel-demo frontend "${ROLLOUT_TIMEOUT}s"
  wait_for_deployment otel-demo load-generator "${ROLLOUT_TIMEOUT}s"

  log "All components deployed successfully"

}

main