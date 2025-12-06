# AGENTS.md

## Overview
This file provides context and instructions for AI agents working on the Jaeger repository. Jaeger is a distributed tracing platform.

## Project Structure
- `cmd/`: Main applications and binaries.
    - `jaeger/`: The main Jaeger v2 binary based on the OpenTelemetry Collector.
    - Other tools and utilities.
- `internal/`: Private library code.
    - `storage/`: Various implementations of storage backends.
- `jaeger-ui/`: Submodule for the frontend (React).
- `idl/`: Submodule for data models (Protobuf, Thrift).
- `scripts/`: Build and maintenance scripts.

## Development Workflow

### Setup
Run the following to initialize submodules and tools:
```bash
git submodule update --init --recursive
make install-tools
```

### Build
- **Binaries**: `make build-binaries` or specific targets like `make build-all-in-one`.
- **UI**: `make build-ui` (requires Node.js).
- **All-in-One**: `make run-all-in-one` (builds UI and runs backend).

### Test
- **Unit Tests**: `make test` matches standard `go test` but includes tags for specific storages.
- **Lint**: `make lint` runs `golangci-lint` and other checks.
- **Format**: `make fmt` runs `gofumpt` and updates license headers. Note: Run this before submitting changes.

## Agent Guidelines
- **Testing**: Always run `make test` after changes.
- **Linting**: If `make lint` fails, try `make fmt` to fix formatting issues automatically.
- **Submodules**: Be aware that `jaeger-ui` and `idl` are submodules. Modifications there might require PRs to their respective repositories.
- **Context**: Refer to `CONTRIBUTING.md` for human-centric guidelines like DCO signing and PR etiquette.

## Do Not Edit
The following files are auto-generated. Do not edit them manually:
- `*.pb.go`
- `*_mock.go`
- `internal/proto-gen/`
