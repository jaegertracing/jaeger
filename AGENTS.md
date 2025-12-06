# AI Coding Agents Guide for Jaeger

This document provides comprehensive guidance for AI coding agents (such as GitHub Copilot, Cursor, and other AI-assisted development tools) working with the Jaeger repository for the first time. It documents setup procedures, common errors, workarounds, and best practices discovered through actual development experience.

**Purpose**: Help AI agents work efficiently with this codebase by providing context that isn't obvious from reading code alone.

This guide supplements [CONTRIBUTING.md](./CONTRIBUTING.md) and [CONTRIBUTING_GUIDELINES.md](./CONTRIBUTING_GUIDELINES.md) with AI-specific context and troubleshooting information.

## Table of Contents

- [Quick Start for AI Agents](#quick-start-for-ai-agents)
- [Project Overview](#project-overview)
- [Repository Structure](#repository-structure)
- [First-Time Setup](#first-time-setup)
- [Development Workflow](#development-workflow)
- [Code Style and Conventions](#code-style-and-conventions)
- [Testing Guidelines](#testing-guidelines)
- [Common Commands](#common-commands)
- [Common Errors and Workarounds](#common-errors-and-workarounds)
- [Important Constraints](#important-constraints)
- [AI Agent Best Practices](#ai-agent-best-practices)
- [Architecture Insights](#architecture-insights)

## Quick Start for AI Agents

When approaching this repository for the first time:

1. **Understand the scale**: This is a CNCF graduated project with high code quality standards (95% test coverage)
2. **Check submodules**: Run `git submodule update --init --recursive` - the UI and API definitions are in submodules
3. **Install tools first**: Run `make install-tools` before attempting to build or test
4. **Use named branches**: CI will fail if you work on `main` branch - always create a feature branch
5. **Sign commits**: Every commit must have DCO sign-off (`git commit -s`)
6. **Test incrementally**: Run `make lint` and `make test` frequently to catch issues early
7. **Understand the architecture**: Jaeger v2 is built on OpenTelemetry Collector - many patterns come from there

## Project Overview

Jaeger is a distributed tracing platform and a CNCF graduated project created by Uber Technologies. It's used for monitoring and troubleshooting microservices-based distributed systems.

### Key Facts

- **Primary Language**: Go 1.24+ (currently using Go 1.25.x in CI)
- **License**: Apache 2.0
- **Project Status**: CNCF Graduated (October 2019)
- **Architecture**: Jaeger v2 is built on OpenTelemetry Collector components
- **Code Quality**: 95% test coverage requirement with strict enforcement

### Key Technologies

- **Language**: Go with modules for dependency management
- **Framework**: OpenTelemetry Collector (many components are OTel Collector extensions)
- **Storage Backends**: Cassandra, Elasticsearch, ClickHouse, Badger (embedded), Memory (for testing)
- **Protocols**: gRPC, HTTP/REST, Thrift (legacy)
- **Data Formats**: Protocol Buffers (primary), Thrift (legacy compatibility)
- **Testing**: testify/assert for assertions, race detector enabled by default

### Project Evolution

- **Jaeger v1**: Original architecture with separate Agent, Collector, Query components
- **Jaeger v2**: Current direction - unified binary built on OTel Collector framework
- Migration is ongoing - you'll see both v1 and v2 patterns in the codebase

## Repository Structure

```
github.com/jaegertracing/jaeger
├── cmd/                           # Binary executables (main packages)
│   ├── all-in-one/               # Jaeger all-in-one for local testing
│   ├── jaeger/                   # Jaeger V2 unified binary (NEW)
│   ├── collector/                # V1 span collection component
│   ├── query/                    # V1 query service and UI API
│   ├── ingester/                 # Kafka-to-storage pipeline
│   ├── remote-storage/           # Remote storage gRPC server
│   ├── tracegen/                 # Trace generator utility
│   └── ...                       # Other utilities (anonymizer, es-index-cleaner, etc.)
├── internal/                      # Internal packages (not importable externally)
│   ├── storage/                  # Storage interfaces and implementations
│   │   ├── v1/                   # V1 storage interfaces
│   │   └── v2/                   # V2 storage interfaces (for OTel Collector)
│   ├── sampling/                 # Sampling strategies
│   ├── proto-gen/                # Generated protobuf code (DO NOT EDIT)
│   └── ...                       # Various internal packages
├── examples/
│   ├── hotrod/                   # HotROD demo application (great for testing)
│   └── grafana-integration/      # Demo showing logs/metrics/traces correlation
├── jaeger-ui/                    # UI submodule (SEPARATE REPOSITORY - Node.js)
├── idl/                          # API definitions submodule (SEPARATE REPOSITORY)
├── docker-compose/               # Deployment examples for different backends
├── scripts/                      # Build, lint, and maintenance scripts
│   ├── makefiles/               # Makefile components
│   └── lint/                    # Linting scripts
├── docs/                         # Documentation
│   ├── adr/                     # Architecture Decision Records
│   └── release/                 # Release process documentation
├── crossdock/                    # Cross-repository integration tests
└── .github/                      # GitHub Actions workflows
    ├── workflows/               # CI/CD pipelines
    └── actions/                 # Reusable GitHub Actions
```

### Important Directory Notes

- **DO NOT MODIFY**: `idl/`, `jaeger-ui/` (git submodules), `internal/proto-gen/`, any `*_mock.go`, `*.pb.go` files
- **Auto-generated**: Files matching `*.pb.go`, `*_mock.go`, files in `internal/proto-gen/`
- **Submodules**: `idl/` and `jaeger-ui/` are separate repositories managed as submodules
- **Tools**: Build tools are tracked in `internal/tools/` and installed to `.tools/` (gitignored)

## First-Time Setup

### Prerequisites

- **Go 1.24+**: Check with `go version` (CI uses 1.25.x)
- **Git**: For cloning and submodule management
- **Make**: Build automation
- **Docker**: Optional, for running integration tests with databases
- **GNU sed**: Required on macOS (install with `brew install gnu-sed`)

### Initial Clone and Setup

```bash
# 1. Clone the repository
git clone https://github.com/jaegertracing/jaeger.git
cd jaeger

# 2. Initialize submodules (CRITICAL - UI and IDL are in submodules)
git submodule update --init --recursive

# 3. Install development tools (gofumpt, golangci-lint, mockery)
#    This downloads and builds tools to .tools/ directory
#    May take 2-3 minutes on first run
make install-tools

# 4. Download Go dependencies (optional, but makes first test run faster)
go mod download

# 5. Verify setup by running tests
#    First run will be slow as it downloads more dependencies
make test
```

### macOS-Specific Setup

On macOS, the default `sed` doesn't support the same flags as GNU `sed`. You'll encounter errors in `make` targets if you don't have GNU sed installed.

**Solution**:
```bash
# Install GNU sed
brew install gnu-sed

# Either add to PATH or set SED variable when running make
export PATH="/opt/homebrew/opt/gnu-sed/libexec/gnubin:$PATH"
# OR
make SED=gsed test
```

The Makefile automatically tries to use `gsed` on macOS, but installation is still required.

## Development Workflow

### Before Starting Work

1. **Create a named branch** (REQUIRED - CI fails on `main` branch):
   ```bash
   git checkout -b feat/my-feature-name
   # OR
   git checkout -b fix/issue-description
   ```

2. **Ensure submodules are up to date**:
   ```bash
   git submodule update --init --recursive
   ```

3. **Check that tools are installed**:
   ```bash
   make install-tools  # Idempotent - safe to run multiple times
   ```

### Development Cycle

```bash
# 1. Make your changes
# 2. Format code (auto-fixes imports and formatting)
make fmt

# 3. Run linters (catches style issues, security problems)
make lint

# 4. Run tests (includes race detector)
make test

# 5. For targeted testing (faster iteration)
go test -v ./internal/storage/...  # Test specific package
go test -v -run TestSpecificFunc  # Test specific function
```

### Before Submitting a PR

All changes MUST:

1. ✅ **Use a named branch** (not `main`) - CI will fail otherwise
2. ✅ **Sign all commits** with DCO sign-off: `git commit -s`
   - Adds `Signed-off-by: Your Name <your.email@example.com>` to commit message
   - Required for every commit - PRs with unsigned commits will be blocked
3. ✅ **Pass formatting**: `make fmt`
4. ✅ **Pass linting**: `make lint`
5. ✅ **Pass tests**: `make test`
6. ✅ **Have test coverage**: All packages need `*_test.go` files (see [Testing Guidelines](#testing-guidelines))

### Commit Message Format

Follow conventional commit style:
- **Format**: `<type>: <description>` (limit first line to 50 chars)
- **Types**: feat, fix, docs, test, refactor, chore, perf
- **Examples**:
  - `feat: add support for ClickHouse storage`
  - `fix: prevent memory leak in span processor`
  - `docs: update installation instructions`

### Branch Naming

Use descriptive names with prefixes:
- `feat/` - New features
- `fix/` - Bug fixes
- `docs/` - Documentation changes
- `test/` - Test-only changes
- `refactor/` - Code refactoring

Examples: `feat/add-metrics`, `fix/memory-leak`, `docs/update-readme`

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

### Coverage Requirements (STRICTLY ENFORCED)

Jaeger enforces a **95% code coverage** threshold. Every package with `.go` files MUST have test coverage.

**The `make test` target will FAIL if**:
- Any package lacks a `*_test.go` file
- A package has no tests and no `empty_test.go` or `.nocover` file

### Three Ways to Satisfy Coverage Requirements

#### Option 1: Write Real Tests (Preferred)

Create `*_test.go` files with actual test functions:

```go
// my_package_test.go
package mypackage

import (
    "testing"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestMyFunction(t *testing.T) {
    // Use testify/assert for assertions
    result := MyFunction("input")
    assert.Equal(t, "expected", result)
}
```

#### Option 2: Create empty_test.go (For Type-Only Packages)

If a package only defines types/interfaces with no testable logic:

```go
// empty_test.go
// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mypackage

import (
    "testing"
    
    "github.com/jaegertracing/jaeger/internal/testutils"
)

func TestMain(m *testing.M) {
    testutils.VerifyGoLeaks(m)
}
```

**Note**: The `TestMain` with `VerifyGoLeaks` is the standard pattern used in Jaeger.

#### Option 3: Create .nocover File (For Untestable Packages)

If a package genuinely cannot be tested (e.g., requires external database, generated code):

```bash
# Create .nocover file with reason
echo "requires external Cassandra database" > ./path/to/package/.nocover
```

**Important**: The `.nocover` file must contain a non-empty reason string explaining why tests are not possible.

### Test Organization Patterns

#### Standard Test Structure

```go
func TestFunctionName(t *testing.T) {
    // Setup
    ctx := context.Background()
    mockDep := setupMockDependency()
    
    // Exercise
    result, err := FunctionUnderTest(ctx, mockDep, "input")
    
    // Verify
    require.NoError(t, err)
    assert.Equal(t, expectedResult, result)
    
    // Teardown (if needed)
    mockDep.Close()
}
```

#### Table-Driven Tests (Common Pattern)

```go
func TestFunctionWithMultipleCases(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {name: "valid input", input: "test", expected: "TEST", wantErr: false},
        {name: "empty input", input: "", expected: "", wantErr: true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := FunctionUnderTest(tt.input)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.expected, result)
            }
        })
    }
}
```

### Running Tests

```bash
# Full test suite with race detector (what CI runs)
make test

# Faster iteration during development (skip race detector)
go test ./...

# Test specific package
go test -v ./internal/storage/...

# Test specific function
go test -v -run TestMySpecificFunction ./path/to/package

# With coverage report
make cover  # Generates cover.html

# Run specific test with verbose output
go test -v -race -run TestFunctionName ./path/to/package
```

### Test Assertions

**Prefer `testify/assert` and `testify/require`**:

- **`assert`**: Test continues after failure (for multiple checks)
- **`require`**: Test stops immediately on failure (for prerequisites)

```go
import (
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestExample(t *testing.T) {
    result, err := SomeFunction()
    
    // Use require for critical checks (stops test if fails)
    require.NoError(t, err, "function must not return error")
    require.NotNil(t, result, "result must not be nil")
    
    // Use assert for additional checks (test continues)
    assert.Equal(t, "expected", result.Field)
    assert.Greater(t, result.Count, 0)
}
```

### Memory Leak Detection

Most test files use `goleak` to detect goroutine leaks:

```go
func TestMain(m *testing.M) {
    testutils.VerifyGoLeaks(m)
}
```

This automatically fails tests that leak goroutines.

## Common Commands

### Development Commands

```bash
# Setup and installation
make install-tools              # Install gofumpt, golangci-lint, mockery
make install-test-tools         # Install only tools needed for testing
go mod download                 # Download Go dependencies

# Code quality
make fmt                        # Format code with gofumpt and fix imports
make lint                       # Run all linters (golangci-lint with many checkers)
make test                       # Run all tests with race detector
make test-ci                    # CI test target (includes coverage)
make cover                      # Generate coverage report (cover.html)

# Building
make build                      # Build all binaries
make build-examples             # Build example applications (hotrod, etc.)
make run-all-in-one            # Build and run Jaeger all-in-one locally

# Code generation (rarely needed - only when changing .proto files or adding mocks)
make proto                      # Generate protobuf files (requires Docker)
make generate-mocks             # Regenerate all mock files using mockery

# Cleanup
make clean                      # Remove build artifacts
```

### Targeted Testing Commands

```bash
# Test specific package
go test -v ./internal/storage/cassandra

# Test with race detector (default in make test)
go test -race ./...

# Test specific function
go test -v -run TestSpecificFunction ./path/to/package

# Test with coverage for one package
go test -cover ./internal/storage/memory

# Run tests with verbose output
go test -v ./...

# Skip long-running tests (integration tests)
go test -short ./...
```

### UI Development Commands

**Note**: UI is in a submodule and requires Node.js. Check `jaeger-ui/.nvmrc` for required version.

```bash
# Ensure submodule is initialized
git submodule update --init --recursive

# Build UI assets (compiles JS/CSS and embeds in Go)
make build-ui

# Build and run all-in-one with UI
make run-all-in-one
```

### Docker Commands

```bash
# Build all Docker images
make docker

# Build specific image
make docker-hotrod

# Run with docker-compose (various configurations)
cd docker-compose && docker-compose -f cassandra.yml up
```

### Debugging and Analysis

```bash
# Run linter on specific file
golangci-lint run path/to/file.go

# Check which packages lack test files
make nocover

# View coverage in terminal
go test -cover ./path/to/package

# Generate and view coverage HTML
make cover
open cover.html  # macOS
xdg-open cover.html  # Linux
```

## Common Errors and Workarounds

This section documents errors you're likely to encounter and their solutions.

### Error: "CI will fail - cannot push to main branch"

**Symptom**: GitHub Actions block PRs from `main` branch
```
Error: This PR cannot be merged because it is from the main branch
```

**Cause**: Working directly on `main` branch

**Solution**:
```bash
# If you already have commits on main:
git checkout -b feat/my-feature    # Create new branch from main
git push -u origin feat/my-feature # Push new branch

# For future work, always start with:
git checkout main
git pull
git checkout -b feat/new-feature
```

### Error: "no required module provides package"

**Symptom**: After adding new dependencies
```
no required module provides package github.com/some/package
```

**Solution**:
```bash
go mod tidy  # Updates go.mod and go.sum
go mod download
```

### Error: "missing go.sum entry"

**Symptom**: During build or test
```
missing go.sum entry for module providing package
```

**Solution**:
```bash
go mod tidy
```

### Error: "at least one *_test.go file must be in all directories"

**Symptom**: `make test` fails with coverage error
```
error: at least one *_test.go file must be in all directories with go files
       so that they are counted for code coverage.
       if no tests are possible for a package (e.g. it only defines types), create empty_test.go
```

**Cause**: Added new package without test files

**Solution**: Choose one of three options:
```bash
# Option 1: Create empty_test.go (for type-only packages)
cat > ./path/to/package/empty_test.go << 'EOF'
// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package yourpackage

import (
    "testing"
    
    "github.com/jaegertracing/jaeger/internal/testutils"
)

func TestMain(m *testing.M) {
    testutils.VerifyGoLeaks(m)
}
EOF

# Option 2: Create real tests (preferred)
# Create package_test.go with actual test functions

# Option 3: Add .nocover file (only if genuinely untestable)
echo "reason: requires external Cassandra connection" > ./path/to/package/.nocover
```

### Error: "DCO check failed" (Unsigned Commits)

**Symptom**: PR blocked by DCO bot
```
This PR has commits missing sign-off
```

**Cause**: Commits not signed with DCO (Developer Certificate of Origin)

**Solution for latest commit**:
```bash
git commit --amend -s  # Add sign-off to last commit
git push --force       # Force push (safe on feature branches)
```

**Solution for multiple commits**:
```bash
# Rebase and sign all commits
git rebase -i HEAD~3 -x "git commit --amend -s --no-edit"
git push --force
```

**Prevention**: Always use `git commit -s` or configure git:
```bash
# Configure git to always sign-off
git config --global alias.ci 'commit -s'
# Then use: git ci -m "message"
```

### Error: "sed: invalid option" (macOS)

**Symptom**: `make` commands fail with sed errors
```
sed: invalid option -- 'i'
```

**Cause**: macOS uses BSD sed, not GNU sed

**Solution**:
```bash
# Install GNU sed
brew install gnu-sed

# Add to PATH (add to ~/.zshrc or ~/.bash_profile)
export PATH="/opt/homebrew/opt/gnu-sed/libexec/gnubin:$PATH"

# OR use SED variable with make
make SED=gsed test
```

### Error: "golangci-lint: not found"

**Symptom**: `make lint` fails
```
make: golangci-lint: command not found
```

**Solution**:
```bash
make install-tools  # Installs golangci-lint to .tools/
```

### Error: "submodule path '...' not initialized"

**Symptom**: Missing UI or IDL files
```
fatal: No url found for submodule path 'jaeger-ui' in .gitmodules
```

**Solution**:
```bash
git submodule update --init --recursive
```

### Error: Protobuf compilation failures

**Symptom**: After modifying `.proto` files
```
protoc: error while loading shared libraries
```

**Solution**:
```bash
# Jaeger uses Docker for protobuf compilation
# Ensure Docker is running
docker ps

# Regenerate protobuf files
make proto

# Never manually edit *.pb.go files
```

### Error: "race detected during execution"

**Symptom**: Tests fail with race detector output
```
WARNING: DATA RACE
```

**Cause**: Concurrent access to shared memory without synchronization

**Solution**:
- Fix the race condition (use mutexes, channels, or atomic operations)
- Race detector is enabled by default - don't disable it
- On s390x architecture, race detector is disabled (not supported)

### Error: Import order or formatting issues

**Symptom**: `make lint` fails with import or format errors

**Solution**:
```bash
# Auto-fix most issues
make fmt

# This runs:
# 1. import-order-cleanup.py (fixes import grouping)
# 2. gofmt (standard Go formatting)
# 3. gofumpt (stricter formatting)
```

### Error: "mockery: command not found" when running generate-mocks

**Solution**:
```bash
make install-tools  # Installs mockery
make generate-mocks
```

### Warning: Changing auto-generated files

**Symptom**: PR review comments about modified generated files

**Files to NEVER manually edit**:
- `*.pb.go` (protobuf generated)
- `*_mock.go` (mockery generated)  
- Files in `internal/proto-gen/`
- Files in `idl/` or `jaeger-ui/` (submodules)

**Solution**: Regenerate instead of editing:
```bash
make proto           # For *.pb.go files
make generate-mocks  # For *_mock.go files

# For submodules, update the submodule reference:
cd idl
git checkout main && git pull
cd ..
git add idl
git commit -m "Update idl submodule"
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

### Before Making Any Changes

1. **Search for similar patterns**: Use `git grep` or code search to find existing implementations
   ```bash
   git grep "type.*Storage interface"  # Find interface definitions
   git grep "func Test.*Storage"       # Find test patterns
   ```

2. **Understand the context**: Read related code, tests, and documentation
   - Check `docs/adr/` for Architecture Decision Records
   - Look at `CONTRIBUTING.md` for project conventions
   - Review similar existing implementations

3. **Check for dependencies**: Understand what the code depends on and what depends on it
   ```bash
   go list -f '{{.Deps}}' ./path/to/package  # Show dependencies
   ```

### When Making Changes

1. **Minimal, focused changes**: Change only what's necessary to solve the problem
   - Avoid refactoring unrelated code
   - Don't fix unrelated issues in the same PR

2. **Follow existing patterns**: Match the style and patterns in nearby code
   - Look at how similar features are implemented
   - Use the same error handling patterns
   - Follow the same testing patterns

3. **Test coverage is mandatory**: 
   - Add tests for new code
   - Update tests for modified code
   - Ensure `make test` passes locally before creating PR

4. **Document non-obvious logic**:
   - Add comments for complex algorithms
   - Explain "why" not "what" (code shows what)
   - Update godoc for public APIs

5. **Incremental verification**:
   ```bash
   make fmt    # Auto-fix formatting
   make lint   # Check for issues
   make test   # Run tests
   # Fix any issues, repeat
   ```

### Common Pitfalls to Avoid

#### ❌ DON'T: Modify Auto-Generated Files

Files to **never** manually edit:
- `*.pb.go` (protobuf - regenerate with `make proto`)
- `*_mock.go` (mocks - regenerate with `make generate-mocks`)
- Files in `internal/proto-gen/` (generated protobuf code)
- Files in `vendor/` (Go modules managed dependencies)
- Files in `idl/` or `jaeger-ui/` (git submodules)

#### ❌ DON'T: Work on the `main` Branch

```bash
# Bad
git checkout main
git commit -m "my changes"  # ❌ CI will fail

# Good
git checkout -b feat/my-feature  # ✅ CI will pass
git commit -m "my changes"
```

#### ❌ DON'T: Forget DCO Sign-Off

```bash
# Bad
git commit -m "my changes"  # ❌ Missing sign-off

# Good
git commit -s -m "my changes"  # ✅ Adds Signed-off-by
```

#### ❌ DON'T: Add Test Files Without Coverage

```bash
# Bad: Empty file without TestMain
touch package_test.go  # ❌ Won't satisfy coverage

# Good: Proper empty_test.go
cat > empty_test.go << 'EOF'
package mypackage
import (
    "testing"
    "github.com/jaegertracing/jaeger/internal/testutils"
)
func TestMain(m *testing.M) {
    testutils.VerifyGoLeaks(m)
}
EOF
```

#### ❌ DON'T: Ignore Linter Warnings

- Fix warnings, don't suppress them with `//nolint` without good reason
- If you must use `//nolint`, add a comment explaining why
- Common valid suppressions: `//nolint:govet // reason here`

#### ❌ DON'T: Remove Tests Without Understanding Them

- Tests document expected behavior
- Removing tests can hide bugs or break functionality
- If a test fails, fix the code or update the test (after understanding why)

### When Suggesting Code

#### Performance Considerations

Jaeger processes **high-throughput distributed tracing data**:
- Prefer efficient algorithms (O(1) or O(log n) over O(n²))
- Avoid allocations in hot paths
- Use sync.Pool for frequently allocated objects
- Profile before optimizing (don't premature optimize)

```go
// Good: Reuse buffers
var bufferPool = sync.Pool{
    New: func() interface{} {
        return new(bytes.Buffer)
    },
}

// Good: Pre-allocate slices when size is known
spans := make([]*Span, 0, expectedCount)
```

#### Storage Backend Considerations

Different backends have different capabilities:
- **Memory**: Full query support, for testing only
- **Cassandra**: Time-range queries, high write throughput
- **Elasticsearch**: Full-text search, complex queries
- **ClickHouse**: Columnar, fast aggregations
- **Badger**: Embedded, limited query support

Design storage interfaces to work with the lowest common denominator.

#### OpenTelemetry Collector Patterns

Jaeger v2 is built on OTel Collector. Follow OTel patterns:
- Use `component.Config` for configuration
- Implement `component.Factory` for components
- Use feature gates for breaking changes (see CONTRIBUTING.md)
- Follow OTel naming conventions

```go
// OTel Collector component pattern
type Config struct {
    Endpoint string `mapstructure:"endpoint"`
    Timeout  time.Duration `mapstructure:"timeout"`
}

func (cfg *Config) Validate() error {
    if cfg.Endpoint == "" {
        return fmt.Errorf("endpoint is required")
    }
    return nil
}
```

#### Security Considerations

- **Never log sensitive data**: API keys, tokens, personal information
- **Validate input**: Especially from external sources (HTTP, gRPC)
- **Use context.Context**: For cancellation and timeouts
- **Sanitize user input**: Prevent injection attacks
- `gosec` linter catches many security issues - heed its warnings

```go
// Good: Use parameterized queries (protects against injection)
query := "SELECT * FROM traces WHERE trace_id = ?"
rows, err := db.Query(query, traceID)

// Good: Validate input
func SetTimeout(timeout time.Duration) error {
    if timeout < 0 {
        return fmt.Errorf("timeout must be positive")
    }
    // ...
}
```

### Working with Dependencies

#### Adding Dependencies

```bash
# 1. Add import in Go code
# 2. Run go mod tidy
go mod tidy

# 3. Verify it's added to go.mod and go.sum
git diff go.mod go.sum

# 4. Check license compatibility (must be compatible with Apache 2.0)
# Look at the dependency's LICENSE file

# 5. Minimize dependencies - prefer standard library when possible
```

#### Checking Licenses

Jaeger is Apache 2.0 licensed. Compatible licenses include:
- ✅ Apache 2.0, MIT, BSD (2 or 3-clause)
- ❌ GPL, LGPL (copyleft - not compatible)
- ⚠️ MPL (check with maintainers first)

### Documentation Updates

#### When to Update Documentation

Update docs when you:
- Add new features or components
- Change existing behavior
- Add new configuration options
- Deprecate features
- Change CLI flags or APIs

#### Where to Update

- `README.md`: High-level project information
- `CHANGELOG.md`: All user-facing changes (see existing entries for format)
- `docs/`: Detailed documentation and ADRs
- Inline godoc: Public APIs and complex functions
- Code comments: Complex algorithms and non-obvious design decisions

#### Godoc Guidelines

```go
// Good: Explains purpose, parameters, return values
// FindTraces searches for traces matching the query parameters.
// It returns up to maxTraces traces, ordered by start time descending.
// Returns ErrNotFound if no traces match the query.
func FindTraces(ctx context.Context, query TraceQuery, maxTraces int) ([]*Trace, error) {
    // ...
}

// Bad: States the obvious
// FindTraces finds traces
func FindTraces(ctx context.Context, query TraceQuery, maxTraces int) ([]*Trace, error) {
    // ...
}
```

## Architecture Insights

### Jaeger v1 vs v2 Architecture

**Jaeger v1** (original architecture):
- Separate binaries: jaeger-agent, jaeger-collector, jaeger-query, jaeger-ingester
- Components communicate over gRPC/HTTP
- Storage plugin interface for custom backends

**Jaeger v2** (current direction):
- Single `cmd/jaeger/` binary built on OpenTelemetry Collector
- Uses OTel Collector's extension/receiver/processor/exporter model
- More modular, easier to extend
- Better integration with OpenTelemetry ecosystem

**Current state**: Both v1 and v2 code exists in the repository. Don't be confused by seeing both patterns.

### Storage Architecture

Jaeger supports multiple storage backends through plugin interfaces:

```
SpanReader/SpanWriter interfaces (v1)
         ↓
   Implementation for:
   - Cassandra (./internal/storage/cassandra/)
   - Elasticsearch (./internal/storage/elasticsearch/) 
   - Memory (./internal/storage/memory/)
   - Badger (via external plugin)
   - ClickHouse (via external plugin)
```

**Key insight**: Storage interfaces are designed for the lowest common denominator. Not all backends support all query features.

### Component Lifecycle

OTel Collector components follow this lifecycle:
1. **Create**: Factory creates component from config
2. **Start**: Component initializes resources
3. **Run**: Component processes data
4. **Shutdown**: Component cleans up resources

```go
// Typical component implementation
type Component struct {
    config Config
    // resources
}

func (c *Component) Start(ctx context.Context, host component.Host) error {
    // Initialize resources
}

func (c *Component) Shutdown(ctx context.Context) error {
    // Clean up resources
}
```

### Sampling Architecture

Jaeger supports multiple sampling strategies:
- **Probabilistic**: Sample X% of traces
- **Rate-limiting**: Sample up to N traces per second
- **Adaptive**: Adjust sampling based on throughput
- **Per-operation**: Different sampling for different operations

Sampling decisions are made at trace creation time and propagated through the trace.

### Understanding the Data Model

Jaeger's data model (defined in `idl/` submodule):
- **Trace**: Collection of spans representing a single transaction
- **Span**: Single operation in a trace, with start time, duration, tags, logs
- **Process**: Information about the service that emitted spans
- **Tags**: Key-value metadata (indexed for search)
- **Logs**: Timestamped events within a span

```
Trace
├── Span 1 (root)
│   ├── Tags: {service="frontend", http.method="GET"}
│   └── Logs: [{timestamp: ..., event: "request started"}]
├── Span 2 (child of 1)
│   └── Tags: {service="backend", db.statement="SELECT ..."}
└── Span 3 (child of 1)
    └── Tags: {service="cache", cache.hit="true"}
```

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

## Summary: Quick Reference for AI Agents

### Critical Do's and Don'ts

**DO**:
- ✅ Create a named branch (not `main`)
- ✅ Sign all commits with `git commit -s`
- ✅ Run `make fmt && make lint && make test` before PR
- ✅ Add test files for all new packages
- ✅ Initialize submodules with `git submodule update --init --recursive`
- ✅ Follow existing code patterns in the repository
- ✅ Check `docs/adr/` for architectural decisions

**DON'T**:
- ❌ Work on `main` branch (CI will fail)
- ❌ Edit auto-generated files (`*.pb.go`, `*_mock.go`, `idl/`, `jaeger-ui/`)
- ❌ Skip test coverage (95% required)
- ❌ Ignore linter warnings
- ❌ Add dependencies without checking licenses
- ❌ Forget DCO sign-off on commits

### Most Common Errors

1. **Working on `main` branch** → Create feature branch
2. **Missing DCO sign-off** → Use `git commit -s`
3. **No test files** → Add `empty_test.go` or real tests
4. **Uninitialized submodules** → Run `git submodule update --init --recursive`
5. **macOS sed issues** → Install GNU sed with `brew install gnu-sed`

### First-Time Setup Checklist

```bash
# 1. Clone and setup
git clone https://github.com/jaegertracing/jaeger.git
cd jaeger
git submodule update --init --recursive

# 2. Install tools (required)
make install-tools

# 3. Verify setup
make test

# 4. Create feature branch
git checkout -b feat/your-feature

# 5. Make changes, then:
make fmt && make lint && make test

# 6. Commit with sign-off
git commit -s -m "feat: your change description"
```

### When in Doubt

- **Search for patterns**: `git grep "similar code"` to find examples
- **Check documentation**: `docs/adr/` for architectural decisions
- **Look at tests**: Test files show expected usage patterns
- **Ask maintainers**: Use GitHub Discussions or Slack

---

**Note**: This guide is specifically for AI coding agents working with this repository for the first time. Human contributors should primarily refer to [CONTRIBUTING.md](./CONTRIBUTING.md) and [CONTRIBUTING_GUIDELINES.md](./CONTRIBUTING_GUIDELINES.md) for official contribution guidelines.

**Last Updated**: 2025-12 - This document evolves with the project. If you encounter errors not documented here, please update this file as part of your PR.
