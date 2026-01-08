#!/usr/bin/env bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROLLOUT_TIMEOUT="${ROLLOUT_TIMEOUT:-600}"

# Versions
OPENSEARCH_VERSION="${OPENSEARCH_VERSION:-3.3.2}"
OPENSEARCH_DASHBOARDS_VERSION="${OPENSEARCH_DASHBOARDS_VERSION:-3.3.0}"
JAEGER_CHART_VERSION="${JAEGER_CHART_VERSION:-4.2.3}"

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
    echo "Environment Variables:"
    echo "  OPENSEARCH_VERSION             - Version of OpenSearch (default: 3.3.2)"
    echo "  OPENSEARCH_DASHBOARDS_VERSION  - Version of OpenSearch Dashboards (default: 3.3.0)"
    echo "  JAEGER_CHART_VERSION           - Version of Jaeger Helm Chart (default: 4.2.3)"
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


  log "Deploying OpenSearch"
  helm upgrade --install opensearch opensearch/opensearch \
    --namespace opensearch --create-namespace \
    --version "${OPENSEARCH_VERSION}" \
    --set image.tag="${OPENSEARCH_VERSION}" \
    -f "$SCRIPT_DIR/opensearch-values.yaml" \
    --wait --timeout 10m
  wait_for_statefulset opensearch opensearch-cluster-single "${ROLLOUT_TIMEOUT}s"

  log "Deploying OpenSearch Dashboards"
  helm upgrade --install opensearch-dashboards opensearch/opensearch-dashboards \
    --namespace opensearch \
    --version "${OPENSEARCH_DASHBOARDS_VERSION}" \
    --set image.tag="${OPENSEARCH_DASHBOARDS_VERSION}" \
    -f "$SCRIPT_DIR/opensearch-dashboard-values.yaml" \
    --wait --timeout 10m
  wait_for_deployment opensearch opensearch-dashboards "${ROLLOUT_TIMEOUT}s"

  
  log "Deploying Jaeger (all-in-one, no storage)"
  helm $HELM_JAEGER_CMD jaeger jaegertracing/jaeger \
    --version "${JAEGER_CHART_VERSION}" \
    --namespace jaeger --create-namespace \
    --set allInOne.enabled=true \
    --set storage.type=none \
    --set allInOne.image.repository=jaegertracing/jaeger \
    --set allInOne.image.tag="${IMAGE_TAG}" \
    --set-file userconfig="$SCRIPT_DIR/jaeger-config.yaml" \
    -f "$SCRIPT_DIR/jaeger-values.yaml" \
    --wait --timeout 10m
  wait_for_deployment jaeger jaeger "${ROLLOUT_TIMEOUT}s"

  log "Deploying HotROD app..."
  kubectl apply -n jaeger -f "$SCRIPT_DIR/hotrod.yaml"
  wait_for_deployment jaeger jaeger-hotrod "${ROLLOUT_TIMEOUT}s"

  
  log "Creating Jaeger query ClusterIP service..."
  kubectl apply -n jaeger -f "$SCRIPT_DIR/jaeger-query-service.yaml"
  log "Jaeger query ClusterIP service created"

  log "Ensuring Jaeger Collector service endpoints are ready before deploying the demo"
  wait_for_service_endpoints jaeger jaeger 180

  log "Ensuring HotROD service endpoints are ready"
  wait_for_service_endpoints jaeger jaeger-hotrod 180

  log "Deploying HotROD trace generator"
  kubectl -n jaeger create configmap trace-script --from-file="$SCRIPT_DIR/generate_traces.py" --dry-run=client -o yaml | kubectl apply -f -
  kubectl apply -n jaeger -f "$SCRIPT_DIR/load-generator.yaml"
  wait_for_deployment jaeger trace-generator "${ROLLOUT_TIMEOUT}s"

  log "Deploying OpenTelemetry Demo (with in-cluster Collector)"
  helm upgrade --install otel-demo open-telemetry/opentelemetry-demo \
    -f "$SCRIPT_DIR/otel-demo-values.yaml" \
    --namespace otel-demo --create-namespace \
    --wait --timeout 15m
  wait_for_deployment otel-demo otel-collector "${ROLLOUT_TIMEOUT}s"
  wait_for_deployment otel-demo frontend "${ROLLOUT_TIMEOUT}s"
  wait_for_deployment otel-demo load-generator "${ROLLOUT_TIMEOUT}s"

  log "All components deployed successfully"

  # Deploy HTTPS ingress
  deploy_ingress

  

  # Deploy Spark Dependencies CronJob
  log "Deploying Spark Dependencies CronJob"
  if kubectl apply -f "$SCRIPT_DIR/spark-dependencies-cronjob-opensearch.yaml"; then
    log "Spark Dependencies CronJob deployed"
    
    # Trigger the job immediately
    log "Triggering initial Spark Dependencies job..."
    JOB_NAME="init-spark-dep-$(date +%s)"
    
    # Create a manual job from the cronjob template
    if kubectl create job --from=cronjob/jaeger-spark-dependencies "$JOB_NAME" -n jaeger; then
      log "Initial job '$JOB_NAME' triggered successfully"
    else
      log " Failed to trigger initial job"
    fi
  else
    log "Failed to deploy Spark Dependencies CronJob"
  fi
 

  log "🎉 Deployment complete! Stack is ready."
}


main
