#!/bin/bash

# Copyright (c) 2025 The Jaeger Authors.
# SPDX-License-Identifier: Apache-2.0

# OpenSearch Observability Stack Cleanup Script 

main() {
  echo "Starting OpenSearch Observability Stack Cleanup"

  # Stop any existing port forwards
  echo "Stopping any existing port-forward processes..."
  pkill -f "kubectl port-forward" 2>/dev/null || true
  echo "✅ Port-forward processes stopped"

  # Uninstall OTEL Demo
  echo " Uninstalling OTEL Demo..."
  if helm list -n otel-demo | grep -q otel-demo; then
    helm uninstall otel-demo -n otel-demo
    echo "✅ OTEL Demo uninstalled"
  else
    echo "⚠️ OTEL Demo not found or already uninstalled"
  fi

  # Uninstall Jaeger
  echo "Uninstalling Jaeger..."
  if helm list -n jaeger | grep -q jaeger; then
    helm uninstall jaeger -n jaeger
    echo "✅ Jaeger uninstalled"
  else
    echo "⚠️ Jaeger not found or already uninstalled"
  fi

  # Uninstall OpenSearch Dashboards
  echo "Uninstalling OpenSearch Dashboards..."
  if helm list -n opensearch | grep -q opensearch-dashboards; then
    helm uninstall opensearch-dashboards -n opensearch
    echo "✅ OpenSearch Dashboards uninstalled"
  else
    echo "⚠️ OpenSearch Dashboards not found or already uninstalled"
  fi

  # Uninstall OpenSearch
  echo " Uninstalling OpenSearch..."
  if helm list -n opensearch | grep -q opensearch; then
    helm uninstall opensearch -n opensearch
    echo "✅ OpenSearch uninstalled"
  else
    echo "⚠️ OpenSearch not found or already uninstalled"
  fi

  # Wait for pods to terminate
  echo "Waiting for pods to terminate..."
  sleep 10

  # Delete namespaces
  echo "Deleting namespaces..."
  for ns in otel-demo jaeger opensearch; do
    if kubectl get namespace "$ns" > /dev/null 2>&1; then
      kubectl delete namespace "$ns" --force --grace-period=0 2>/dev/null || true
      echo "✅ Namespace $ns deleted"
    else
      echo "⚠️ Namespace $ns not found or already deleted"
    fi
  done

  # Clean up any remaining resources (PVCs, etc.)
  echo "Cleaning up any remaining PVCs..."
  kubectl get pvc -A | grep -E "(opensearch|jaeger|otel-demo)" || echo "No remaining PVCs found"

  # Final verification
  echo "Performing final verification..."
  remaining_pods=$(kubectl get pods -A | grep -E "(opensearch|jaeger|otel-demo)" || true)
  if [ -z "$remaining_pods" ]; then
    echo "All components cleaned up successfully!"
  else
    echo "⚠️ Some pods may still be terminating:"
    echo "$remaining_pods"
    echo "This is normal and they should disappear shortly"
  fi

  echo ""
  echo "✅ Cleanup Complete!"
  echo ""
  echo " All OpenSearch observability stack components have been removed"
  echo ""
}

main "$@"
