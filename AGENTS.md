# AGENTS.md

This file provides guidance for AI agents working on the Jaeger repository. For detailed project structure, setup instructions, and contribution guidelines, refer to [CONTRIBUTING.md](./CONTRIBUTING.md).

What the project expects from a human contributor who works with an agent is stated in the [AI Usage Policy](./AI_POLICY.md). It applies to work done in this repository, and the human running you owns everything you produce.

## Setup

The primary branch is called `main`, all PRs are merged into it.

If checking out a fresh repository, initialize submodules:
```bash
git submodule update --init --recursive
```

## Upgrading Go

Follow [Upgrading the Go Version](./CONTRIBUTING.md#upgrading-the-go-version) in CONTRIBUTING.md. The version is mirrored across several files that a script propagates from the top-level `go.mod`, and Delve has to be upgraded alongside it in [jaegertracing/base-image-with-debugger](https://github.com/jaegertracing/base-image-with-debugger).

## Pull Requests

- **Require an issue:** Do not open a Pull Request unless there is an existing, open GitHub Issue that explicitly requests the work and carries the `help wanted` label. A PR without one is labelled `missing-issue` or `unapproved-issue` and closed automatically (see [Link each pull request to an issue](./CONTRIBUTING_GUIDELINES.md#link-each-pull-request-to-an-issue)). Speculative refactors and unsolicited feature work are not accepted.
- **Stay inside the issue's scope:** Implement only what the issue describes. If you discover related problems, mention them in the PR description or open a separate issue rather than fixing them in the same PR.
- **One issue, one PR:** Do not bundle multiple issues into a single PR.
- **No PR for chores already handled by automation:** Dependency bumps managed by Dependabot and similar housekeeping are handled automatically. Do not open PRs that duplicate that work.
- **State the operational risk:** When a change matches an item under [Operational risk](./REVIEW.md#operational-risk), the PR description says what fails when the new code misbehaves and for whom, how an operator turns the change off or rolls it back, and whether data written by the new version survives the rollback.

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

## Copyright and Third-Party Code

Write the code yourself rather than copying it in from another project. If a change genuinely needs third-party code, it must be Apache-2.0 compatible, and its original copyright header must be preserved — add the Jaeger copyright above it rather than replacing it. See [Copyright Header](./CONTRIBUTING_GUIDELINES.md#copyright-header) for the exact form. Never remove an existing copyright header.

New files get the standard header (but with current year):

```
// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
```

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

## Storage Integration Tests

Storage behavior belongs in the shared `internal/storage/integration` corpus and `StorageIntegration.AssertCorpus`, which run in both direct and e2e modes. New assertions run for every backend by default; unsupported backends must explicitly opt out in `internal/storage/integration/capabilities`. Extend the e2e reader/writer adapters when the shared tests need new API fields, and keep transport-specific assertions in the e2e package. Read [the integration architecture](cmd/jaeger/internal/integration/README.md) before adding tests.

## Scope Discipline

- Do not reformat, rename, or restructure code outside the scope of the requested change.
- Do not bump dependencies unless the task requires it.
- Do not change CI workflows or release tooling unless explicitly asked.
- Before adding a flag or field that controls behavior, find the mechanism that already owns that decision and extend it. Expressing one decision in two places is worse than either place alone, and replacing an established mechanism is a maintainer's call.

## RFC / ADR Documents

RFCs (`docs/rfc/`) are proposals; ADRs (`docs/adr/`) are decision records.

- When a PR implements a milestone described in an RFC or ADR, update that document in the same PR: mark the milestone ✅ and link the delivering PR. Keep the milestone/status tracking current.
- Before editing an RFC or an ADR beyond that, read the [RFC lifecycle](./docs/rfc/README.md#lifecycle) and the [ADR lifecycle](./docs/adr/README.md#lifecycle). They say when the prose of a document is rewritten, when it is left alone, and when a new document replaces it.
- Files in a directory next to an RFC (`docs/rfc/NNNN-short-slug/`) are evidence for the RFC's claims and not product code. Review them for whether each test backs the claim that cites it, not against the coding standards of this file. See [Evidence for claims about other systems](./docs/rfc/README.md#evidence-for-claims-about-other-systems).

## When in Doubt

Stop and ask rather than guessing. It is better to surface a question in the PR description than to invent behavior, fabricate API names, or silence failing checks.

Ask as well when you are *not* in doubt but are about to depart from a documented convention, because that is where confidence is least informative.
