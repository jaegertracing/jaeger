#!/bin/bash

# OpenSearch Observability Stack Cleanup Script
# This script removes all components deployed by the deployment script

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging function
log() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')] $1${NC}"
}

success() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] $1${NC}"
}

warning() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')]  $1${NC}"
}

error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')]  $1${NC}"
}

# Main cleanup function
main() {
    log "Starting OpenSearch Observability Stack Cleanup"
    
    # Stop any existing port forwards
    log "Stopping any existing port-forward processes..."
    pkill -f "kubectl port-forward" 2>/dev/null || true
    success "Port-forward processes stopped"
    
    
    # Uninstall OTEL Demo
    log "Uninstalling OTEL Demo..."
    if helm list -n otel-demo | grep -q otel-demo; then
        helm uninstall otel-demo -n otel-demo
        success "OTEL Demo uninstalled"
    else
        warning "OTEL Demo not found or already uninstalled"
    fi
    
    # Uninstall Jaeger
    log "Uninstalling Jaeger..."
    if helm list -n jaeger | grep -q jaeger; then
        helm uninstall jaeger -n jaeger
        success "Jaeger uninstalled"
    else
        warning "Jaeger not found or already uninstalled"
    fi
    
    # Uninstall OpenSearch Dashboards
    log "Uninstalling OpenSearch Dashboards..."
    if helm list -n opensearch | grep -q opensearch-dashboards; then
        helm uninstall opensearch-dashboards -n opensearch
        success "OpenSearch Dashboards uninstalled"
    else
        warning "OpenSearch Dashboards not found or already uninstalled"
    fi
    
    # Uninstall OpenSearch
    log "Uninstalling OpenSearch..."
    if helm list -n opensearch | grep -q opensearch; then
        helm uninstall opensearch -n opensearch
        success "OpenSearch uninstalled"
    else
        warning "OpenSearch not found or already uninstalled"
    fi
    
    # Wait for pods to terminate
    log "Waiting for pods to terminate..."
    sleep 10
    
    # Delete namespaces
    log "Deleting namespaces..."
    for ns in otel-demo jaeger opensearch; do
        if kubectl get namespace $ns > /dev/null 2>&1; then
            kubectl delete namespace $ns --force --grace-period=0 2>/dev/null || true
            success "Namespace $ns deleted"
        else
            warning "Namespace $ns not found or already deleted"
        fi
    done
    
    # Clean up any remaining resources (PVCs, etc.)
    log "Cleaning up any remaining PVCs..."
    kubectl get pvc -A | grep -E "(opensearch|jaeger|otel-demo)" || warning "No remaining PVCs found"
    
    # Final verification
    log "Performing final verification..."
    remaining_pods=$(kubectl get pods -A | grep -E "(opensearch|jaeger|otel-demo)" || true)
    if [ -z "$remaining_pods" ]; then
        success "All components cleaned up successfully!"
    else
        warning "Some pods may still be terminating:"
        echo "$remaining_pods"
        log "This is normal and they should disappear shortly"
    fi
    
    echo ""
    success "Cleanup Complete! "
    echo ""
    log "All OpenSearch observability stack components have been removed"
    echo ""
}


main "$@"