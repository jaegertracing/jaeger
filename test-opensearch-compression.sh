#!/bin/bash

set -e

echo "=== OpenSearch Compression Test Script ==="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to wait for service
wait_for_service() {
    local url=$1
    local name=$2
    local max_attempts=30
    local attempt=1
    
    echo "Waiting for $name to be ready..."
    while [ $attempt -le $max_attempts ]; do
        if curl -s "$url" > /dev/null 2>&1; then
            echo -e "${GREEN}$name is ready!${NC}"
            return 0
        fi
        echo -n "."
        sleep 2
        ((attempt++))
    done
    echo -e "${RED}$name failed to start after $max_attempts attempts${NC}"
    return 1
}

# Clean up previous runs
echo "Cleaning up previous test environment..."
docker-compose -f docker-compose-opensearch-test.yml down -v 2>/dev/null || true

# Start OpenSearch
echo ""
echo "Starting OpenSearch..."
docker-compose -f docker-compose-opensearch-test.yml up -d opensearch

# Wait for OpenSearch to be ready
wait_for_service "http://localhost:9200" "OpenSearch"

# Check OpenSearch version
echo ""
echo "OpenSearch version info:"
curl -s http://localhost:9200 | jq '.version' || echo "Failed to get version"

# Test 1: Original version (should fail)
echo ""
echo -e "${YELLOW}=== Test 1: Original version with compression enabled ===${NC}"
echo "Building and starting original Jaeger..."

# Build original version
docker-compose -f docker-compose-opensearch-test.yml build jaeger-original

# Start original version and capture logs
echo "Starting original Jaeger (this should fail with compression error)..."
docker-compose -f docker-compose-opensearch-test.yml up jaeger-original 2>&1 | tee original-jaeger.log &
ORIGINAL_PID=$!

# Wait a bit and check if it failed
sleep 15

if grep -q "Compressor detection can only be called on some xcontent bytes" original-jaeger.log; then
    echo -e "${GREEN}✓ Bug reproduced successfully! Found expected error.${NC}"
else
    if curl -s http://localhost:16686 > /dev/null 2>&1; then
        echo -e "${RED}✗ Original version started successfully - bug NOT reproduced${NC}"
    else
        echo -e "${YELLOW}? Original version failed but with different error${NC}"
    fi
fi

# Stop original version
docker-compose -f docker-compose-opensearch-test.yml stop jaeger-original

# Test 2: Fixed version (should work)
echo ""
echo -e "${YELLOW}=== Test 2: Fixed version with compression enabled ===${NC}"
echo "Building and starting fixed Jaeger..."

# First, ensure we're on the fix branch
git checkout fix-opensearch-compression

# Build fixed version
docker-compose -f docker-compose-opensearch-test.yml build jaeger-fixed

# Start fixed version
echo "Starting fixed Jaeger (this should work)..."
docker-compose -f docker-compose-opensearch-test.yml up jaeger-fixed 2>&1 | tee fixed-jaeger.log &
FIXED_PID=$!

# Wait for startup
sleep 15

# Check if it's running successfully
if curl -s http://localhost:16687 > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Fixed version started successfully!${NC}"
    
    # Check if templates were created
    echo ""
    echo "Checking if index templates were created:"
    curl -s http://localhost:9200/_cat/templates?v | grep jaeger || echo "No templates found"
    
    # Send a test span
    echo ""
    echo "Sending test span..."
    curl -X POST http://localhost:14269/api/traces \
        -H "Content-Type: application/x-thrift" \
        -H "Content-Length: 0" 2>/dev/null || true
    
    # Check if spans can be stored
    sleep 2
    echo ""
    echo "Checking indices:"
    curl -s http://localhost:9200/_cat/indices?v | grep jaeger || echo "No indices found"
    
else
    echo -e "${RED}✗ Fixed version failed to start${NC}"
    tail -20 fixed-jaeger.log
fi

# Clean up background processes
kill $ORIGINAL_PID 2>/dev/null || true
kill $FIXED_PID 2>/dev/null || true

# Show summary
echo ""
echo -e "${YELLOW}=== Test Summary ===${NC}"
echo "1. Original version with compression: $(grep -q 'Compressor detection' original-jaeger.log && echo -e "${GREEN}Failed as expected${NC}" || echo -e "${RED}Did not fail as expected${NC}")"
echo "2. Fixed version with compression: $(curl -s http://localhost:16687 > /dev/null 2>&1 && echo -e "${GREEN}Working correctly${NC}" || echo -e "${RED}Failed${NC}")"

echo ""
echo "Test logs saved to:"
echo "- original-jaeger.log"
echo "- fixed-jaeger.log"

# Optional: Clean up
read -p "Clean up test environment? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    docker-compose -f docker-compose-opensearch-test.yml down -v
fi