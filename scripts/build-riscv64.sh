#!/bin/bash
# Build script for RISC-V 64-bit architecture

# Set environment variables
export GOOS=linux
export GOARCH=riscv64
export CGO_ENABLED=0

# Get version information
GIT_SHA=$(git rev-parse HEAD)
GIT_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')

# Build flags
GO_BUILD_ARGS="-ldflags=-X github.com/jaegertracing/jaeger/internal/version.commitSHA=${GIT_SHA} \
                       -X github.com/jaegertracing/jaeger/internal/version.latestVersion=${GIT_TAG} \
                       -X github.com/jaegertracing/jaeger/internal/version.date=${BUILD_DATE}"

# Create build directory
mkdir -p build/riscv64

# Build all-in-one binary
go build ${GO_BUILD_ARGS} -o build/riscv64/jaeger-all-in-one ./cmd/all-in-one