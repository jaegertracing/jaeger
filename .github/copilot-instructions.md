# Copilot Review Instructions

## Core principle

Prefer silence over uncertainty. A review comment has a cost — it demands
the author's attention, may trigger another round, and can obscure the
comments that actually matter. Only post a comment when you are confident
the issue is real and material.

## What to flag

Flag genuine defects in the diff: logic errors, incorrect conditions,
missing error handling, concurrency hazards, resource leaks, security
regressions, data corruption. These always warrant a comment.

## What not to flag

- **Style and formatting** — CI enforces these automatically. Do not
  comment on indentation, line length, naming conventions, comment
  phrasing, or anything a linter or formatter would catch.
- **Pre-existing issues** — if a problem exists in code the PR did not
  introduce, do not raise it. The PR is not the right vehicle to fix it.
- **Low-confidence observations** — if you are not certain an issue is
  real, stay silent. Do not post speculative or "consider whether..."
  comments.
- **Already-raised issues** — if you flagged something in an earlier
  review round on this PR and the author did not act on it, do not raise
  it again. Either it was resolved, or the author made a deliberate
  choice. Repeating it creates noise without value.

## Threshold

Before posting, ask: is this a correctness or security defect that a
careful human reviewer would block the PR on? If not, stay silent.
