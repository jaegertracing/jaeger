# Crossdock End-to-End Tests

This document describes the end-to-end (E2E) testing infrastructure implemented in the `ci-crossdock.yml` GitHub Actions workflow.

## Overview

The Crossdock tests are E2E integration tests that verify Jaeger's ability to receive traces from various client libraries and encoding formats. The tests use the [Crossdock](https://github.com/crossdock/crossdock) framework to orchestrate multi-container test scenarios.

## Architecture

The test environment consists of the following components:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Docker Compose Environment                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌──────────────────┐         ┌──────────────────────────────────────────┐  │
│  │   Crossdock      │         │        Zipkin-Brave Clients              │  │
│  │   Orchestrator   │ ──────► │  (zipkin-brave-thrift/json/json-v2/proto)│  │
│  │                  │         └───────────────────┬──────────────────────┘  │
│  └────────┬─────────┘                             │                         │
│           │                                       │                         │
│           ▼                                       ▼                         │
│  ┌──────────────────┐                  ┌──────────────────┐                 │
│  │   Test Driver    │                  │ Jaeger Collector │                 │
│  │ (jaegertracing/  │ ───────────────► │  (port 9411 for  │                 │
│  │  test-driver)    │                  │   Zipkin format) │                 │
│  └────────┬─────────┘                  └────────┬─────────┘                 │
│           │                                     │                           │
│           │                                     ▼                           │
│           │                            ┌──────────────────┐                 │
│           │                            │ Jaeger Remote    │                 │
│           │                            │ Storage (memory) │                 │
│           │                            └────────┬─────────┘                 │
│           │                                     │                           │
│           ▼                                     ▼                           │
│  ┌──────────────────────────────────────────────────────────┐               │
│  │                    Jaeger Query                          │               │
│  │                    (port 16686)                          │               │
│  └──────────────────────────────────────────────────────────┘               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Components

1. **Crossdock Orchestrator** (`crossdock/crossdock`): The test framework that coordinates test execution across multiple containers.

2. **Test Driver** (`jaegertracing/test-driver`): A custom Go application (built from `/crossdock/main.go`) that:
   - Waits for Jaeger components to be healthy
   - Instructs client services to create traces
   - Queries Jaeger Query API to verify traces were stored correctly

3. **Zipkin-Brave Clients** (`jaegertracing/xdock-zipkin-brave`): Java-based trace generators using the Zipkin Brave library. Multiple instances test different encoding formats:
   - `zipkin-brave-thrift`: Thrift encoding
   - `zipkin-brave-json`: JSON encoding (v1)
   - `zipkin-brave-json-v2`: JSON encoding (v2)
   - `zipkin-brave-proto`: Protobuf encoding

4. **Jaeger Backend**: Full Jaeger deployment including:
   - `jaeger-collector`: Receives spans via Zipkin-compatible endpoint (port 9411)
   - `jaeger-query`: Provides API for trace retrieval
   - `jaeger-remote-storage`: In-memory storage backend

## Test Flow

1. **Setup Phase**:
   - Docker Compose starts all containers
   - Test Driver waits for Jaeger Query and Collector health checks

2. **Test Execution** (EndToEnd behavior):
   - Crossdock orchestrator calls Test Driver with a service parameter
   - Test Driver sends HTTP POST to the specified client service (e.g., `zipkin-brave-thrift:8081/create_traces`)
   - Client service creates traces with unique random tags and sends them to Jaeger Collector
   - Test Driver queries Jaeger Query API to retrieve traces matching the tags
   - Test validates that expected traces were received and stored correctly

3. **Validation**:
   - Verifies correct number of traces received
   - Validates that all expected tags are present in stored spans

## Files and Structure

```
crossdock/
├── Dockerfile                  # Builds the test-driver image
├── docker-compose.yml          # Defines crossdock services and Zipkin clients
├── jaeger-docker-compose.yml   # Defines Jaeger backend services
├── main.go                     # Test Driver entry point
├── rules.mk                    # Make targets for running crossdock
└── services/
    ├── collector.go            # Collector service client (sampling API)
    ├── query.go                # Query service client (trace retrieval)
    ├── tracehandler.go         # Test logic and validation
    └── common.go               # Shared utilities
```

## Running Locally

```bash
# Build and run crossdock tests
make build-and-run-crossdock

# View logs on failure
make crossdock-logs

# Clean up containers
make crossdock-clean
```

## GitHub Actions Workflow

The `ci-crossdock.yml` workflow:
1. Triggers on pushes to `main`, pull requests, and merge queue
2. Builds all required Docker images (Jaeger binaries + test driver)
3. Runs the crossdock test suite via `scripts/build/build-crossdock.sh`
4. On success for `main` branch: publishes test-driver image to Docker Hub and Quay.io
5. On failure: outputs container logs for debugging

## Test Behaviors

| Behavior | Description |
|----------|-------------|
| `endtoend` | Creates traces via client services and verifies they are queryable in Jaeger |
| `adaptive` | (Legacy) Tests adaptive sampling rate calculation and propagation |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `JAEGER_COLLECTOR_HOST_PORT` | Collector endpoint for clients |
| `JAEGER_COLLECTOR_HC_HOST_PORT` | Collector health check endpoint |
| `JAEGER_QUERY_HOST_PORT` | Query API endpoint |
| `JAEGER_QUERY_HC_HOST_PORT` | Query health check endpoint |
