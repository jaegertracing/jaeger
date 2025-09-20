#!/bin/bash

# OpenSearch Observability Stack Port Forwarding Script
# This script sets up port forwarding for all services

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
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] $1${NC}"
    exit 1
}

# Function to check if service exists
check_service() {
    local service=$1
    local namespace=$2
    
    if kubectl get svc $service -n $namespace > /dev/null 2>&1; then
        return 0
    else
        return 1
    fi
}

# Main function
main() {
    log "Starting Port Forwarding for OpenSearch Observability Stack"
    
    # Check if kubectl is available
    if ! command -v kubectl &> /dev/null; then
        error "kubectl is required but not installed."
    fi
    
    # Check if cluster is accessible
    if ! kubectl cluster-info > /dev/null 2>&1; then
        error "Cannot connect to Kubernetes cluster. Please ensure minikube is running."
    fi
    
    # Stop any existing port forwards first
    log "Stopping any existing port-forward processes..."
    pkill -f "kubectl port-forward" 2>/dev/null || true
    sleep 2
    
    # Array to track started services
    started_services=()
    failed_services=()
    
    # Start port forwarding for each service
    log "Starting port forwarding services..."
    
    # Jaeger Query UI
    if check_service "jaeger-query-clusterip" "jaeger"; then
        kubectl port-forward -n jaeger svc/jaeger-query-clusterip 16686:16686 &
        started_services+=("Jaeger UI (http://localhost:16686)")
        log "Started: Jaeger UI on port 16686"
    else
        failed_services+=("Jaeger (service not found)")
        warning "Jaeger service not found"
    fi
    
    # OpenSearch Dashboards
    if check_service "opensearch-dashboards" "opensearch"; then
        kubectl port-forward -n opensearch svc/opensearch-dashboards 5601:5601 &
        started_services+=("OpenSearch Dashboards (http://localhost:5601)")
        log "Started: OpenSearch Dashboards on port 5601"
    else
        failed_services+=("OpenSearch Dashboards (service not found)")
        warning "OpenSearch Dashboards service not found"
    fi
    
    # OpenSearch API
    if check_service "opensearch-cluster-single" "opensearch"; then
        kubectl port-forward -n opensearch svc/opensearch-cluster-single 9200:9200 &
        started_services+=("OpenSearch API (http://localhost:9200)")
        log "Started: OpenSearch API on port 9200"
    else
        failed_services+=("OpenSearch API (service not found)")
        warning "OpenSearch API service not found"
    fi
    
    # OTEL Demo Frontend
    if check_service "frontend-proxy" "otel-demo"; then
        kubectl port-forward -n otel-demo svc/frontend-proxy 8080:8080 &
        started_services+=("OTEL Demo Frontend (http://localhost:8080)")
        log "Started: OTEL Demo Frontend on port 8080"
    else
        failed_services+=("OTEL Demo Frontend (service not found)")
        warning "OTEL Demo Frontend service not found"
    fi
    
    # Load Generator
    if check_service "load-generator" "otel-demo"; then
        kubectl port-forward -n otel-demo svc/load-generator 8089:8089 &
        started_services+=("Load Generator (http://localhost:8089)")
        log "Started: Load Generator on port 8089"
    else
        failed_services+=("Load Generator (service not found)")
        warning "Load Generator service not found"
    fi
    
    # HotROD Demo App (from Jaeger Helm chart v2)
    if check_service "jaeger-hotrod" "jaeger"; then
        kubectl port-forward -n jaeger svc/jaeger-hotrod 8088:80 &
        started_services+=("HotROD Demo App (http://localhost:8088)")
        log "Started: HotROD Demo App on port 8088"
    else
        failed_services+=("HotROD Demo App (service not found)")
        warning "HotROD Demo App service not found"
    fi
    
    # Wait for services to start
    sleep 3
    
    # Display summary
    echo ""
    success "Port Forwarding Setup Complete!"
    echo ""
    
    if [ ${#started_services[@]} -gt 0 ]; then
        echo -e "${GREEN}Successfully started services:${NC}"
        for service in "${started_services[@]}"; do
            echo "  • $service"
        done
        echo ""
    fi
    
    if [ ${#failed_services[@]} -gt 0 ]; then
        echo -e "${YELLOW}Failed to start services:${NC}"
        for service in "${failed_services[@]}"; do
            echo "  • $service"
        done
        echo ""
        warning "Some services may not be deployed yet. Run the deployment script first."
        echo ""
    fi
    
    if [ ${#started_services[@]} -gt 0 ]; then
        echo -e "${BLUE} Management Commands:${NC}"
        echo "  • View all port-forwards: jobs"
        echo "  • Stop all port-forwards: pkill -f 'kubectl port-forward'"
        echo "  • Stop this script: Ctrl+C"
        echo ""
        
        # Keep the script running
        log "Port forwarding is active. Press Ctrl+C to stop all port-forwards."
        
        # Wait for interrupt
        trap 'log "Stopping all port-forwards..."; pkill -f "kubectl port-forward"; success "All port-forwards stopped."; exit 0' INT
        
        # Keep script alive
        while true; do
            sleep 10
        done
    else
        error "No services were successfully started. Please check your deployment."
    fi
}


main "$@"
