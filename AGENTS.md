# AI Coding Agents Guide for Jaeger

This document provides guidance for AI coding agents (such as GitHub Copilot, Cursor, and other AI-assisted development tools) working with the Jaeger repository. It supplements the [CONTRIBUTING.md](./CONTRIBUTING.md) and [CONTRIBUTING_GUIDELINES.md](./CONTRIBUTING_GUIDELINES.md) files with AI-specific context.

## Table of Contents

- [Project Overview](#project-overview)
- [Repository Structure](#repository-structure)
- [Development Workflow](#development-workflow)
- [Code Style and Conventions](#code-style-and-conventions)
- [Testing Guidelines](#testing-guidelines)
- [Common Commands](#common-commands)
- [Important Constraints](#important-constraints)
- [AI Agent Best Practices](#ai-agent-best-practices)

## Project Overview

Jaeger is a distributed tracing platform that is a CNCF graduated project. Key facts:

- **Primary Language**: Go (version 1.24+)
- **License**: Apache 2.0
- **Architecture**: Built on OpenTelemetry Collector components
- **Purpose**: End-to-end distributed tracing for monitoring and troubleshooting microservices

### Key Technologies

- Go modules for dependency management
- OpenTelemetry Collector framework
- Storage backends: Cassandra, Elasticsearch, ClickHouse, Badger, Memory
- gRPC and HTTP protocols
- Protocol Buffers and Thrift for data serialization

## Repository Structure

```
github.com/jaegertracing/jaeger
├── cmd/                    # Binary executables
│   ├── all-in-one/        # Jaeger all-in-one for local testing
│   ├── jaeger/            # Jaeger V2 binary
│   ├── collector/         # Span collection component
│   ├── query/             # Query service and UI API
│   ├── ingester/          # Kafka-to-storage pipeline
│   └── ...                # Other utilities
├── internal/              # Internal packages
│   ├── storage/          # Storage interface and implementations
│   └── ...
├── examples/
│   ├── hotrod/           # Demo application
│   └── ...
├── jaeger-ui/            # UI submodule (Node.js)
├── idl/                  # API definitions submodule
├── docker-compose/       # Deployment examples
└── scripts/              # Build and maintenance scripts
```

## Development Workflow

### Setup Requirements

```bash
# Clone repository with submodules
git submodule update --init --recursive

# Install required tools
make install-tools

# Run tests
make test
```

### Before Submitting Changes

All changes MUST:

1. **Use a named branch** (not `main`) - CI will fail otherwise
2. **Sign all commits** with DCO sign-off using `git commit -s`
3. **Pass linting**: `make lint`
4. **Pass tests**: `make test`
5. **Be formatted**: `make fmt` (uses `gofumpt`)

### Branching

- **Never** commit directly to `main` branch
- Create feature branches from `main`
- Use descriptive branch names (e.g., `feat/add-metrics`, `fix/memory-leak`)

## Code Style and Conventions

### Import Grouping

Always group imports in this order:

```go
import (
    // 1. Standard library
    "context"
    "fmt"

    // 2. External packages
    "github.com/uber/jaeger-lib/metrics"
    "go.uber.org/zap"

    // 3. Jaeger internal packages
    "github.com/jaegertracing/jaeger/cmd/agent/app"
    "github.com/jaegertracing/jaeger/internal/storage"
)
```

### Formatting

- Use `gofumpt` for formatting (stricter than `gofmt`)
- Configure your editor to run `gofumpt` on save
- Run `make fmt` before committing

### Linting

- Uses `golangci-lint` with specific rules (see `.golangci.yml`)
- Enabled linters include: `gosec`, `staticcheck`, `gocritic`, `revive`, etc.
- Run `make lint` to check for issues

### File Headers

All new Go files must include:

```go
// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
```

## Testing Guidelines

### Coverage Requirements

- Target: **95% code coverage**
- All packages MUST have at least one `*_test.go` file
- Packages without tests need an `empty_test.go` file
- Untestable packages (e.g., requiring external dependencies) need a `.nocover` file

### Test Organization

```go
// Example test structure
func TestFunctionName(t *testing.T) {
    // Use testify/assert for assertions
    assert := assert.New(t)
    
    // Setup
    // Exercise
    // Verify
    // Teardown
}
```

### Running Tests

```bash
# All tests with race detector
make test

# Specific package
go test -v ./internal/storage/...

# With coverage
go test -cover ./...
```

## Common Commands

```bash
# Development
make install-tools        # Install development tools
make fmt                  # Format code with gofumpt
make lint                 # Run all linters
make test                 # Run all tests
make build                # Build all binaries
make run-all-in-one      # Run Jaeger all-in-one locally

# Code Generation
make proto                # Generate protobuf files
make generate-mocks       # Generate test mocks

# Cleanup
make clean                # Clean build artifacts

# UI Development (requires Node.js 6+)
make build-ui            # Build UI assets

# Docker
make docker              # Build Docker images
```

## Important Constraints

### Security

- **Never commit secrets** or sensitive data
- Use `gosec` for security scanning (included in lint)
- Be cautious with user input sanitization
- Follow secure coding practices

### Deprecation Policy

When deprecating features:
- Provide **3 months** or **two minor versions** grace period (whichever is later)
- Add deprecation messages with removal timeline:
  ```
  (deprecated, will be removed after yyyy-mm-dd or in release vX.Y.Z, whichever is later)
  ```
- Document in CHANGELOG.md

### Breaking Changes

- Use OpenTelemetry Collector feature gates for breaking changes
- Start with Alpha (disabled by default) or Beta (enabled, can disable)
- Progress to Stable (cannot disable) after two releases
- Remove gate after another two releases

### Version Compatibility

- Maintain configuration compatibility with OpenTelemetry Collector
- Support currently maintained Go versions (N and N-1)
- Removing support for unsupported Go versions is not breaking

## AI Agent Best Practices

### When Making Changes

1. **Understand the context**: Read related code and documentation before suggesting changes
2. **Minimal changes**: Make the smallest possible change that solves the problem
3. **Test your changes**: Ensure tests pass and coverage is maintained
4. **Follow existing patterns**: Match the coding style and patterns in nearby code
5. **Document non-obvious logic**: Add comments for complex algorithms or business logic

### Common Pitfalls to Avoid

- ❌ Modifying auto-generated files (`*.pb.go`, `*_mock.go`)
- ❌ Changing code in `vendor/`, `idl/`, or `jaeger-ui/` directories
- ❌ Adding dependencies without checking for security vulnerabilities
- ❌ Removing or modifying tests without understanding their purpose
- ❌ Ignoring linter warnings
- ❌ Making changes to `main` branch
- ❌ Forgetting DCO sign-off on commits

### When Suggesting Code

- **Prefer standard library**: Use Go standard library when possible
- **Consider performance**: Jaeger processes high-throughput tracing data
- **Think about storage**: Different backends have different capabilities
- **Check OpenTelemetry patterns**: Align with OTel Collector conventions
- **Review security implications**: Especially for data handling code

### Working with Dependencies

- Dependencies managed via Go modules (`go.mod`)
- Use `go get` to add new dependencies
- Minimize external dependencies
- Check licenses are compatible with Apache 2.0
- Run `go mod tidy` after changes

### Documentation

- Update relevant `.md` files when changing functionality
- Keep inline code comments accurate
- Document complex algorithms or non-obvious design decisions
- Update API documentation for public interfaces

## Additional Resources

- [Contributing Guidelines](./CONTRIBUTING.md) - Detailed contribution process
- [Contributing Guidelines Extended](./CONTRIBUTING_GUIDELINES.md) - Workflow details
- [Governance](./GOVERNANCE.md) - Project governance and maintainer process
- [Release Process](./RELEASE.md) - How releases are managed
- [Security](./SECURITY.md) - Security policies and reporting
- [Documentation](https://www.jaegertracing.io/docs/) - Official documentation
- [OpenTelemetry Collector](https://github.com/open-telemetry/opentelemetry-collector) - Upstream project

## Questions or Issues?

- **Slack**: [#jaeger on CNCF Slack](https://cloud-native.slack.com/archives/CGG7NFUJ3)
- **Mailing List**: [jaeger-tracing@googlegroups.com](https://groups.google.com/forum/#!forum/jaeger-tracing)
- **GitHub Issues**: [jaegertracing/jaeger/issues](https://github.com/jaegertracing/jaeger/issues)
- **Discussions**: [jaegertracing/jaeger/discussions](https://github.com/jaegertracing/jaeger/discussions)

---

**Note**: This guide is specifically for AI coding agents. Human contributors should primarily refer to [CONTRIBUTING.md](./CONTRIBUTING.md) and [CONTRIBUTING_GUIDELINES.md](./CONTRIBUTING_GUIDELINES.md).
