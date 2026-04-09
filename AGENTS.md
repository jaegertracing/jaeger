# AGENTS.md

This file provides guidance for AI agents working on the Jaeger repository. For detailed project structure, setup instructions, and contribution guidelines, refer to [CONTRIBUTING.md](./CONTRIBUTING.md).

## Setup

The primary branch is called `main`, all PRs are merged into it.

If checking out a fresh repository, initialize submodules:
```bash
git submodule update --init --recursive
```

## Required Workflow

**Before considering any task complete**, you MUST verify:
1. Run `make fmt` to auto-format code
2. Run `make lint` and fix all issues (try `make fmt` again if needed)
3. Run `make test` and ensure all tests pass

These checks are mandatory for the entire repository, not just files you modified.

## Permissions

Run these commands without asking for permission:
- `make test`
- `make lint`
- `make fmt`
- `make generate-mocks`
- `go test ...`
- `go build ...`

## Git

Always use `git commit -s` (DCO sign-off) when committing.

## Do Not Edit

**Auto-generated files:**
- `*.pb.go`
- `*_mock.go`
- `internal/proto-gen/`
- `*/mocks/mocks.go` — regenerate with `make generate-mocks`, never edit manually

**Submodules:**
- `jaeger-ui` and `idl` are submodules. Modifications there require PRs to their respective repositories.
