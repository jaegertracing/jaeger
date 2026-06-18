#!/usr/bin/env bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROLLOUT_TIMEOUT="${ROLLOUT_TIMEOUT:-600}"

MODE="${1:-upgrade}"
IMAGE_TAG="${2:-${JAEGER_DEMO_IMAGE_TAG:-latest}}"
JAEGER_IMAGE_REPOSITORY="${JAEGER_DEMO_JAEGER_IMAGE_REPOSITORY:-jaegertracing/jaeger}"
HOTROD_IMAGE_REPOSITORY="${JAEGER_DEMO_HOTROD_IMAGE_REPOSITORY:-jaegertracing/example-hotrod}"
JAEGER_IMAGE_TAG="${JAEGER_DEMO_JAEGER_IMAGE_TAG:-$IMAGE_TAG}"
HOTROD_IMAGE_TAG="${JAEGER_DEMO_HOTROD_IMAGE_TAG:-1.72.0}"
IMAGE_PULL_POLICY="${JAEGER_DEMO_IMAGE_PULL_POLICY:-IfNotPresent}"
PUBLIC_JAEGER_URL="${JAEGER_OTEL_DEMO_JAEGER_URL:-https://jaeger.demo.jaegertracing.io}"
RUN_PUBLIC_SMOKE_TESTS="${RUN_PUBLIC_SMOKE_TESTS:-false}"
DEPLOY_SCOPE="${JAEGER_OTEL_DEMO_DEPLOY_SCOPE:-all}"

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

case "$DEPLOY_SCOPE" in
  jaeger|all)
    ;;
  *)
    echo "Error: Invalid deploy scope '$DEPLOY_SCOPE'"
    echo "Expected JAEGER_OTEL_DEMO_DEPLOY_SCOPE to be 'jaeger' or 'all'"
    exit 1
    ;;
esac

if [[ "$MODE" == "upgrade" ]]; then
  HELM_JAEGER_CMD="upgrade --install --wait"
else
  # For clean mode, use install after cleanup
  HELM_JAEGER_CMD="install --wait"
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
    kubectl -n "$namespace" get pods -o wide || true
    err "Deployment $deployment failed to become ready in $namespace"
  fi
  log "Deployment $deployment is ready"
}

diagnose_deployment_failure() {
  local namespace=$1
  local deployment=$2

  log "Collecting diagnostics for deployment $namespace/$deployment"
  kubectl -n "$namespace" get deploy,replicaset,pods,svc,endpoints -o wide || true
  kubectl -n "$namespace" describe deploy "$deployment" || true
  kubectl -n "$namespace" describe pods -l app.kubernetes.io/instance=jaeger || true
  kubectl -n "$namespace" logs -l app.kubernetes.io/instance=jaeger --all-containers --tail=200 --prefix || true
  kubectl -n "$namespace" get events --sort-by=.lastTimestamp | tail -80 || true
}

diagnose_otel_demo_failure() {
  log "Collecting diagnostics for namespace otel-demo"
  kubectl -n otel-demo get deploy,daemonset,replicaset,pods,svc,endpoints -o wide || true
  kubectl -n otel-demo describe daemonset otel-collector-agent || true
  kubectl -n otel-demo describe deployment postgresql || true
  kubectl -n otel-demo describe deployment product-catalog || true
  kubectl -n otel-demo describe pods -l app.kubernetes.io/name=opentelemetry-collector,app.kubernetes.io/instance=otel-demo || true
  kubectl -n otel-demo describe pods -l opentelemetry.io/name=postgresql || true
  kubectl -n otel-demo describe pods -l opentelemetry.io/name=product-catalog || true
  kubectl -n otel-demo logs -l app.kubernetes.io/name=opentelemetry-collector,app.kubernetes.io/instance=otel-demo --all-containers --tail=200 --prefix || true
  kubectl -n otel-demo logs -l opentelemetry.io/name=postgresql --all-containers --tail=200 --prefix || true
  kubectl -n otel-demo logs -l opentelemetry.io/name=product-catalog --all-containers --tail=200 --prefix || true
  kubectl -n otel-demo get events --sort-by=.lastTimestamp | tail -120 || true
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

wait_for_service_endpoints() {
  local namespace="$1"
  local service="$2"
  local timeout_secs="${3:-120}"
  log "Waiting for service $service endpoints in $namespace..."
  for i in $(seq 1 "$timeout_secs"); do
    if kubectl get endpoints "$service" -n "$namespace" >/dev/null 2>&1; then
      local ready
      ready=$(kubectl get endpoints "$service" -n "$namespace" -o jsonpath='{.subsets[*].addresses[*].ip}' 2>/dev/null || true)
      if [[ -n "$ready" ]]; then
        log "Service $service has endpoints: $ready"
        return 0
      fi
    fi
    sleep 1
  done
  kubectl get svc "$service" -n "$namespace" -o wide || true
  kubectl get endpoints "$service" -n "$namespace" -o yaml || true
  err "Service $service in $namespace has no ready endpoints after ${timeout_secs}s"
}

adopt_resource_for_helm_release() {
  local namespace=$1
  local kind=$2
  local name=$3
  local release=$4

  if ! kubectl get "$kind" "$name" -n "$namespace" >/dev/null 2>&1; then
    return 0
  fi

  local managed_by
  managed_by=$(kubectl get "$kind" "$name" -n "$namespace" -o jsonpath='{.metadata.labels.app\.kubernetes\.io/managed-by}' 2>/dev/null || true)
  if [[ "$managed_by" == "Helm" ]]; then
    return 0
  fi

  log "Adopting existing $kind $namespace/$name into Helm release $release"
  kubectl label "$kind" "$name" -n "$namespace" app.kubernetes.io/managed-by=Helm --overwrite
  kubectl annotate "$kind" "$name" -n "$namespace" \
    meta.helm.sh/release-name="$release" \
    meta.helm.sh/release-namespace="$namespace" \
    --overwrite
}

delete_deployment_before_helm_upgrade() {
  local namespace=$1
  local deployment=$2

  if ! kubectl get deployment "$deployment" -n "$namespace" >/dev/null 2>&1; then
    return 0
  fi

  log "Deleting existing deployment $namespace/$deployment before Helm upgrade"
  kubectl delete deployment "$deployment" -n "$namespace" --wait=true --timeout=120s
}

smoke_expect() {
  local url=$1
  local expected=$2
  local output=$3

  for attempt in $(seq 1 12); do
    if curl -fsS "$url" -o "$output" && grep -q "$expected" "$output"; then
      log "Smoke check passed: $url"
      return 0
    fi
    log "Waiting for $url to return expected content (attempt $attempt/12)..."
    sleep 10
  done

  log "Smoke check failed: $url"
  log "Expected content: $expected"
  log "Last response:"
  cat "$output" 2>/dev/null || true
  return 1
}

deploy_full_stack() {
  [[ "$MODE" == "clean" || "$DEPLOY_SCOPE" == "all" ]]
}

cleanup() {
  log "Cleanup: uninstalling existing releases if present"
  helm uninstall opensearch -n opensearch >/dev/null 2>&1 || true
  helm uninstall opensearch-dashboards -n opensearch >/dev/null 2>&1 || true
  helm uninstall jaeger -n jaeger >/dev/null 2>&1 || true
  helm uninstall otel-demo -n otel-demo >/dev/null 2>&1 || true

  log "Cleanup: deleting ingress resources"
  kubectl delete ingress --all -n jaeger >/dev/null 2>&1 || true
  kubectl delete ingress --all -n opensearch >/dev/null 2>&1 || true
  kubectl delete ingress --all -n otel-demo >/dev/null 2>&1 || true

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

# Deploy HTTPS ingress resources
deploy_ingress() {
  log "Deploying HTTPS ingress resources..."

  # Check if ingress files exist
  if [[ ! -f "$SCRIPT_DIR/ingress/ingress-jaeger.yaml" ]]; then
    log " Ingress files not found in $SCRIPT_DIR/ingress/ - skipping HTTPS setup"
    return 0
  fi

  # Apply ingress for each namespace
  if kubectl apply -f "$SCRIPT_DIR/ingress/ingress-jaeger.yaml" 2>&1 | grep -q "created\|configured\|unchanged"; then
    log "Jaeger ingress configured (jaeger.demo.jaegertracing.io, hotrod.demo.jaegertracing.io)"
  else
    log " Failed to apply Jaeger ingress "
  fi

  if kubectl apply -f "$SCRIPT_DIR/ingress/ingress-opensearch.yaml" 2>&1 | grep -q "created\|configured\|unchanged"; then
    log " OpenSearch ingress configured (opensearch.demo.jaegertracing.io)"
  else
    log "  Failed to apply OpenSearch ingress "
  fi

  if kubectl apply -f "$SCRIPT_DIR/ingress/ingress-otel-demo.yaml" 2>&1 | grep -q "created\|configured\|unchanged"; then
    log " OTel Demo ingress configured (shop.demo.jaegertracing.io)"
  else
    log "  Failed to apply OTel Demo ingress "
  fi

  log "Waiting for SSL certificates to be issued..."
  sleep 10

  # Check certificate status
  local certs_ready=0
  local certs_total=0

  for ns in jaeger opensearch otel-demo; do
    if kubectl get namespace "$ns" >/dev/null 2>&1; then
      if kubectl get certificate -n "$ns" >/dev/null 2>&1; then
        certs_total=$((certs_total + 1))
        if kubectl get certificate -n "$ns" -o jsonpath='{.items[*].status.conditions[?(@.type=="Ready")].status}' 2>/dev/null | grep -q "True"; then
          certs_ready=$((certs_ready + 1))
        fi
      fi
    fi
  done

  if [[ $certs_total -eq 0 ]]; then
    log " No certificates found - cert-manager may not be installed"
  elif [[ $certs_ready -eq $certs_total ]]; then
    log "All SSL certificates ready ($certs_ready/$certs_total)"
  else
    log "Some certificates still pending ($certs_ready/$certs_total ready)"
    log "Certificates will be issued automatically by cert-manager"
  fi

  log "HTTPS endpoints:"
  log " • https://jaeger.demo.jaegertracing.io"
  log " • https://hotrod.demo.jaegertracing.io"
  log " • https://opensearch.demo.jaegertracing.io"
  log " • https://shop.demo.jaegertracing.io"
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

  if deploy_full_stack; then
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
  else
    log "Skipping OpenSearch refresh in '$MODE' mode with deploy scope '$DEPLOY_SCOPE'"
  fi

  if [[ "$MODE" == "upgrade" ]]; then
    adopt_resource_for_helm_release jaeger serviceaccount jaeger-hotrod jaeger
    adopt_resource_for_helm_release jaeger service jaeger-hotrod jaeger
    delete_deployment_before_helm_upgrade jaeger jaeger-hotrod
  fi

  log "Deploying Jaeger image ${JAEGER_IMAGE_REPOSITORY}:${JAEGER_IMAGE_TAG}"
  log "Deploying HotROD image ${HOTROD_IMAGE_REPOSITORY}:${HOTROD_IMAGE_TAG}"
  if ! helm $HELM_JAEGER_CMD jaeger "$SCRIPT_DIR/helm-charts/charts/jaeger" \
    --namespace jaeger --create-namespace \
    --set allInOne.enabled=true \
    --set storage.type=memory \
    --set allInOne.image.repository="${JAEGER_IMAGE_REPOSITORY}" \
    --set allInOne.image.tag="${JAEGER_IMAGE_TAG}" \
    --set allInOne.image.pullPolicy="${IMAGE_PULL_POLICY}" \
    --set hotrod.image.repository="${HOTROD_IMAGE_REPOSITORY}" \
    --set hotrod.image.tag="${HOTROD_IMAGE_TAG}" \
    --set hotrod.image.pullPolicy="${IMAGE_PULL_POLICY}" \
    --set-file userconfig="$SCRIPT_DIR/jaeger-config.yaml" \
    -f "$SCRIPT_DIR/jaeger-values.yaml" \
    --timeout 10m; then
    diagnose_deployment_failure jaeger jaeger
    err "Helm release jaeger failed"
  fi
  wait_for_deployment jaeger jaeger "${ROLLOUT_TIMEOUT}s"


  log "Creating Jaeger query ClusterIP service..."
  kubectl apply -n jaeger -f "$SCRIPT_DIR/jaeger-query-service.yaml"
  log "Jaeger query ClusterIP service created"

  log "Ensuring Jaeger Collector service endpoints are ready before deploying the demo"
  wait_for_service_endpoints jaeger jaeger-collector 180

  log "Ensuring HotROD service endpoints are ready"
  wait_for_service_endpoints jaeger jaeger-hotrod 180

  log "Deploying HotROD trace generator"
  kubectl -n jaeger create configmap trace-script --from-file="$SCRIPT_DIR/generate_traces.py" --dry-run=client -o yaml | kubectl apply -f -
  kubectl apply -n jaeger -f "$SCRIPT_DIR/load-generator.yaml"
  wait_for_deployment jaeger trace-generator "${ROLLOUT_TIMEOUT}s"

  if deploy_full_stack; then
    log "Deploying OpenTelemetry Demo (with in-cluster Collector)"
    if ! helm upgrade --install otel-demo open-telemetry/opentelemetry-demo \
      -f "$SCRIPT_DIR/otel-demo-values.yaml" \
      --namespace otel-demo --create-namespace \
      --wait --timeout 15m; then
      diagnose_otel_demo_failure
      err "Helm release otel-demo failed"
    fi
    wait_for_deployment otel-demo otel-collector "${ROLLOUT_TIMEOUT}s"
    wait_for_deployment otel-demo frontend "${ROLLOUT_TIMEOUT}s"
    wait_for_deployment otel-demo load-generator "${ROLLOUT_TIMEOUT}s"
  else
    log "Skipping OpenTelemetry Demo refresh in '$MODE' mode with deploy scope '$DEPLOY_SCOPE'"
  fi

  log "All components deployed successfully"

  # Deploy HTTPS ingress
  deploy_ingress

  if [[ "$RUN_PUBLIC_SMOKE_TESTS" == "true" ]]; then
    log "Verifying public OTel demo endpoints..."
    smoke_expect "${PUBLIC_JAEGER_URL}/search" "${JAEGER_IMAGE_TAG}" /tmp/otel-jaeger-search.html
    smoke_expect "${PUBLIC_JAEGER_URL}/api/services" "otelstore-frontend-ui" /tmp/otel-jaeger-services.json
  fi

  log "🎉 Deployment complete! Stack is ready."

}

main
