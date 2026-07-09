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
- **Cover your changed code before pushing.** Codecov enforces a **95% patch target** (`.codecov.yml`), so a PR whose diff dips below it fails CI. Measure patch coverage locally before opening or updating a PR — e.g. `go test -covermode=atomic -coverprofile=cover.out ./<changed-pkg>/... && go tool cover -func=cover.out` — and add tests for the uncovered new/changed lines. If a changed line is genuinely unreachable or not meaningfully testable (e.g. an error branch no test can trigger), restructure it to be testable or call it out in the PR description; don't leave the gap silent. Files matched by `.codecov.yml`'s `ignore` list (generated code, `mocks/`, `main.go`, integration tests, `internal/tools`) are exempt.
- Bug fixes must include a regression test that fails without the fix.
- Do not delete existing tests to make a build green. If a test is genuinely wrong, explain why in the PR description.
- Do not weaken assertions (e.g. replacing exact checks with `assert.NotNil`) just to make a flaky test pass.
- Every package must have at least one `*_test.go` file (enforced by `make nocover`). If no tests are possible (e.g. a package that only defines types), create an empty `empty_test.go`.

## Scope Discipline

- Do not reformat, rename, or restructure code outside the scope of the requested change.
- Do not bump dependencies unless the task requires it.
- Do not change CI workflows or release tooling unless explicitly asked.

## RFC / ADR Documents

RFCs (`docs/rfc/`) are point-in-time proposals; ADRs (`docs/adr/`) are decision records.

- When a PR implements a milestone described in an RFC or ADR, update that document in the same PR: mark the milestone ✅ and link the delivering PR. Keep the milestone/status tracking current.
- Do **not** rewrite an RFC's prose, abstract, or diagrams to match the evolving codebase — its narrative is a historical snapshot of the state and plan when it was written. The same applies to ADRs.
- When an RFC's work is fully delivered, mark its status Implemented; if the resulting architecture is worth an enduring reference, graduate it into a new ADR in [`docs/adr/`](./docs/adr/) that states the outcome and links back to the RFC, rather than mutating the RFC. [ADR-012](./docs/adr/012-unified-elasticsearch-client.md) (graduated from RFC 0006) is an example.

## When in Doubt

Stop and ask rather than guessing. It is better to surface a question in the PR description than to invent behavior, fabricate API names, or silence failing checks.
