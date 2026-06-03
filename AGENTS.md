# AGENTS.md

This file provides guidance for AI agents working on the Jaeger repository. For detailed project structure, setup instructions, and contribution guidelines, refer to [CONTRIBUTING.md](./CONTRIBUTING.md).

## Setup

The primary branch is called `main`, all PRs are merged into it.

If checking out a fresh repository, initialize submodules:
```bash
git submodule update --init --recursive
```

## Pull Requests

- **Require an issue:** Do not open a Pull Request unless there is an existing, open GitHub Issue that explicitly requests the work. Speculative refactors and unsolicited feature work are not accepted.
- **Stay inside the issue's scope:** Implement only what the issue describes. If you discover related problems, mention them in the PR description or open a separate issue rather than fixing them in the same PR.
- **One issue, one PR:** Do not bundle multiple issues into a single PR.
- **No PR for chores already handled by automation:** Dependency bumps managed by Dependabot and similar housekeeping are handled automatically. Do not open PRs that duplicate that work.

## Required Workflow

**Before considering any task complete**, you MUST verify:
1. Run `make fmt` to auto-format code
2. Run `make lint` and fix all issues (try `make fmt` again if needed)
3. Run `make test` and ensure all tests pass

These checks are mandatory for the entire repository, not just files you modified.

Do not skip, disable, or bypass these checks (e.g. `--no-verify`, commenting out linters, adding broad `//nolint` directives) to make CI pass. Fix the underlying issue.

## Permissions

Run these commands without asking for permission:
- `make test`
- `make lint`
- `make fmt`
- `make generate-mocks`
- `go test ...`
- `go build ...`

## Git

- Always use `git commit -s` (DCO sign-off) when committing.
- Capitalize the first word of the description after the `type(scope):` prefix, e.g. `fix(test): Inline all deps…` not `fix(test): inline all deps…`

## Do Not Edit

**Auto-generated files:**
- `*.pb.go`
- `*_mock.go`
- `internal/proto-gen/`
- `*/mocks/mocks.go` — regenerate with `make generate-mocks`, never edit manually

**Submodules:**
- `jaeger-ui` and `idl` are submodules. Modifications there require PRs to their respective repositories.

## Tests

- All new functionality must include tests.
- Bug fixes must include a regression test that fails without the fix.
- Do not delete existing tests to make a build green. If a test is genuinely wrong, explain why in the PR description.
- Do not weaken assertions (e.g. replacing exact checks with `assert.NotNil`) just to make a flaky test pass.
- Every package must have at least one `*_test.go` file (enforced by `make nocover`). If no tests are possible (e.g. a package that only defines types), create an empty `empty_test.go`.

## Scope Discipline

- Do not reformat, rename, or restructure code outside the scope of the requested change.
- Do not bump dependencies unless the task requires it.
- Do not change CI workflows or release tooling unless explicitly asked.

## When in Doubt

Stop and ask rather than guessing. It is better to surface a question in the PR description than to invent behavior, fabricate API names, or silence failing checks.
