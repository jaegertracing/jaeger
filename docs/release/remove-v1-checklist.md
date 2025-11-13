# Remove v1 release logic — incremental milestone checklist (updated)

Owner: @yurishkuro  
Related: https://github.com/jaegertracing/jaeger/issues/7497  
Prepared: 2025-11-12

## Summary

We will perform a clean, audited migration from dual v1/v2 releases to v2-only releases. The migration is split into small, testable milestones so we do not break the ability to produce v1 artifacts until we intentionally stop publishing them.

This document is an update to the previously merged checklist and reflects the agreed milestone ordering and file allocations:

- Milestone 0 — Coordination / snapshot (already done)
- Milestone 1 — RE-NUMBER BUILD TARGETS TO USE v2 BY DEFAULT (build and image targets)
- Milestone 2 — REMOVE ALL USAGE of v1 artifacts everywhere that could be invoked by maintainers or CI (non-breaking to release/publish)
- Milestone 3 — STOP PUBLISHING v1 artifacts (release/publish changes)
- Milestone 4 — Release notes & user-facing scripts (docs and helper finalization)
- Milestone 5 — Cleanup remaining references (examples, tests, docs)
- Milestone 6 — Final removal and prune (policy-based post-sunset)

Notes:
- "Re-number build targets" (Milestone 1) means change the defaults in build scripts and Makefiles so that most targets produce v2 artifacts by default, with explicit exceptions for selected v1 targets and the ability to override to v1 when needed.
- "Remove usage" (Milestone 2) means update any convenience targets, examples, dev Makefiles, CI test helper scripts and READMEs that would cause contributors or CI to pick or run v1 artifacts by default. Do not change the core release/publish automation that we still need to be able to produce v1 artifacts until Milestone 3 (except where those core pieces are strictly only dev convenience and not needed for releases).
- "Stop publishing" (Milestone 3) is the step where we change release automation so v1 artifacts are no longer produced/uploaded.

---

## Milestone 0 — Coordination (done)

- Create a rollback snapshot branch/tag: `pre-remove-v1-YYYY-MM-DD`.
- Baseline checklist merged: `docs/release/remove-v1-checklist.md`.

---

## Milestone 1 — RE-NUMBER BUILD TARGETS TO USE v2 BY DEFAULT

Owner: @yurishkuro

Goal
- Ensure most build and image targets default to producing v2 artifacts. v1 should only be produced for the following targets (they remain v1):
  - build-all-in-one
  - build-query
  - build-collector
  - build-ingester
- All other build targets (binaries and docker images) should default to v2. Maintain the ability to override to v1 via an explicit env var/Makefile variable (e.g., JAEGER_VERSION=1 or similar) but make v2 the default.

Acceptance criteria
- `scripts/makefiles/BuildBinaries.mk` and other Makefiles/targets produce v2 artifacts by default except for the explicit exceptions listed above.
- Docker build scripts and helpers (examples: `scripts/build/build-upload-a-docker-image.sh`, docker-related Makefiles) default to v2 tags; v1 tag generation is only produced when explicitly requested.
- CI or documented developer convenience targets no longer pull/build v1 artifacts by default.

Files / targets assigned to this milestone (non-exhaustive — guidance to scan repo)
- [ ] `scripts/makefiles/BuildBinaries.mk`  
  - Change defaults for targets other than the four exceptions listed above.
- [ ] `scripts/build/build-upload-a-docker-image.sh`  
  - Default to v2 push/tags.
- [ ] `scripts/utils/compute-tags.sh`  
  - Ensure default computed tags are v2-first.

Implementation guidance
- Make minimal edits: flip default variables so v2 is implied, leave an explicit override to v1.
- Avoid changing core release/publishing automation that must still be able to publish v1 until the later milestone (this is M1 and non-publishing).
- Apply same principle to Docker image builders and helpers.

Milestone 1 testing
- Run CI test jobs in staging and confirm builds do not produce or pull v1 artifacts by default.
- Run Makefile targets and build scripts locally to validate v2 defaults and v1 override behaviour.

---

## Milestone 2 — REMOVE ALL USAGE of v1 artifacts (non-breaking to release/publish)

Goal
- Ensure no scripts, automated tests, documentation examples, or convenience targets that maintainers or CI use will pull, build, or reference v1 artifacts by default.
- Do NOT change core release/publishing workflows that are required to produce v1 artifacts (those belong to Milestone 3).

Acceptance criteria
- CI test jobs & documented maintainer commands do not reference v1 by default.
- Developer convenience targets and READMEs used in release/test flows are updated to v2 or removed.
- Release/publish scripts remain able to produce v1 artifacts (unchanged in this milestone).

Files assigned to Milestone 2 (update usage only)
- [ ] `docker-compose/tail-sampling/Makefile`  
  - Replace `JAEGER_VERSION=1...` convenience defaults with v2 or remove v1 convenience targets.
- [ ] `docker-compose/monitor/Makefile`  
  - Update dev convenience targets and README examples to use v2 by default.
- [ ] `examples/otel-demo/deploy-all.sh`  
  - If the script is referenced by CI/docs, default to v2 (or make v1 explicit/legacy).
- [ ] `examples/*` and README example lines that are invoked by CI or referenced in release docs  
  - Update documented example commands to v2.
- [ ] small convenience Makefile targets / scripts referenced in documentation or used by CI tests (identify by scan)  
  - Replace v1 defaults with v2; remove legacy v1 targets where appropriate.
- [ ] `scripts/e2e/*` (only test helpers invoked by CI, if they default to v1)  
  - Update defaults used by CI test jobs to v2 (but do not modify release/publish scripts).
- [ ] `scripts/utils/compare_metrics.py` (if used in tests or example automation)  
  - Make v2 metrics the default for compare helpers invoked by CI.
- [ ] Any other example/demo helpers that are used by CI or are part of the documented maintainer workflow (identify & update).

Implementation guidance
- Make minimal edits: change default literals, remove v1 convenience targets, update README example lines.
- Avoid touching core release code paths (packaging, workflows that create upload actions, top-level make targets used by release automation).

Milestone 2 testing
- Run CI test jobs (staging) and ensure they don't pull v1 images by default.
- Run example/demo commands from docs and confirm they use v2.
- Sanity-check that release automation still can build v1 artifacts (no changes to release publish workflows in this milestone).

---

## Milestone 3 — STOP PUBLISHING v1 artifacts (release/publish changes)

Goal
- Change packaging and CI release automation so v1 artifacts are not built/pushed/uploaded for official releases.

Acceptance criteria
- Performing a release with a v2 tag (dry-run in a fork or staging) results in only v2 artifacts being published.
- No v1 images/binaries are uploaded to registries or GitHub Releases.

Files assigned to Milestone 3 (publish removal)
- [ ] `.github/workflows/ci-release.yml`  
  - Remove steps that create/upload v1 release artifacts; ensure upload steps use v2 artifact names only.
- [ ] `.github/workflows/ci-docker-build.yml` (publish-related steps)  
  - Do not push v1 tags for official releases; push only v2.
- [ ] `.github/workflows/ci-docker-hotrod.yml` (if it participates in release publish)  
  - Ensure demo/image publishing uses v2 tags only.
- [ ] `scripts/build/build-upload-a-docker-image.sh`  
  - Remove v1 push logic and ensure push paths (for releases) only push v2 tags.
- [ ] `scripts/build/package-deploy.sh`  
  - Stop packaging/uploading `VERSION_V1` artifacts; upload only v2 artifacts. Remove checks that required both versions.
- [ ] `scripts/utils/compute-tags.sh`  
  - Ensure the computed publish tags for release flows are v2-only; remove v1 tag generation on release branch.
- [ ] any other upload/publish helper invoked by the release workflow  
  - Remove v1 publish behavior.

Implementation guidance
- These changes can safely alter the ability to publish v1 artifacts because we will have validated Milestone 1 and 2 first.
- Keep changes explicit and reversible. Test on a fork/staging release.

Milestone 3 testing
- Run the CI release workflow on a fork with a v2 tag (dry-run) and verify only v2 artifacts are uploaded.
- Verify Docker registry and GitHub Release contents.

---

## Milestone 4 — Release notes & user-facing scripts

Goal
- Update user-facing release docs and helper scripts so maintainers have a clean v2-only flow and instructions.

Files assigned to Milestone 4
- [ ] `RELEASE.md`  
  - Update instructions to be v2-only (replace "tag v1 & v2" with v2-only).
- [ ] `CHANGELOG.md` (and any tools that parse its headers)  
  - Ensure automated changelog tooling extracts v2 headers correctly; be tolerant of legacy format for a short transition time.
- [ ] `scripts/release/start.sh`  
  - Finalize prompts to v2-only (after Milestone 3).
- [ ] `scripts/release/draft.py`  
  - Draft v2-only GitHub releases; update headers and `gh release` invocations to use v2 tag.

Testing
- Run `start.sh -d` and `draft.py` in dry-run to validate v2-first outputs.
- Validate maintainers can follow `RELEASE.md` to produce a v2 release.

---

## Milestone 5 — Cleanup remaining references (many small PRs)

Goal
- Sweep the repo and clean remaining `v1` references in examples, tests, CONTRIBUTING.md, and other non-critical areas. Split into small PRs.

Files / areas
- [ ] `scripts/e2e/elasticsearch.sh` (finalize v2 default)
- [ ] `scripts/utils/compare_metrics.py` (final cleanup)
- [ ] `CONTRIBUTING.md` (document v2 as primary; note v1 status)
- [ ] any remaining docker-compose examples, READMEs and sample scripts
- [ ] any other files found by repo-wide `v1` sweep

Testing
- Run examples, e2e, and developer quickstarts; verify expected behavior.

---

## Milestone 6 — Final removal and prune (policy-based)

Goal
- After the sunset/support window ends, remove v1-only code, CI shards, docs and directories.

Action
- Delete v1-only directories and targets; remove legacy CI workflows and scripts.
- Announce removal and update docs/website.

---

## PR strategy (recommended)

- Keep PRs small and focused.
  - PR A — Milestone 1: `chore/reassign-to-v2-defaults` — re-number build targets to use v2 by default (change Makefiles and build scripts). Include test plan: run CI builds, verify v2 artifacts are produced by default and v1 override works.
  - PR B — Milestone 2: `chore/remove-v1-usage` — change convenience targets and examples (non-breaking). Include test plan: run CI tests, local smoke tests for example flows.
  - PR C — Milestone 3: `chore/remove-v1-publish` — change release/publish workflows and packaging scripts. Test on fork with dry-run release.
  - PR D — Milestone 4: docs & helper finalization.
  - PR E+ — Milestone 5: many small PRs for examples/tests cleanup.
- Each PR must include:
  - short description of changes,
  - explicit test plan (how to dry-run/validate),
  - reviewer list (CI/release owners & @yurishkuro).

---

## QA & rollback

- Always create a rollback snapshot branch before changing publishing logic: `pre-remove-v1-YYYY-MM-DD`.
- For each PR:
  - run CI tests in a fork/staging,
  - run the release dry-run (for Milestone 3 PR),
  - perform a quick sanity check of docs and examples.
- If an urgent re-publish of v1 is required after removal, revert the Milestone 3 PR(s) and re-run the legacy snapshot branch to produce missing artifacts.

---

## Next actions

Pick one:
- A) I will prepare a draft PR for **Milestone 1** (`chore/reassign-to-v2-defaults`) that re-numbers build targets to use v2 by default (change Makefiles and build scripts). (Recommended first step.)
- B) I will prepare a draft PR for **Milestone 2** (`chore/remove-v1-usage`) that implements the minimal, safe changes to convenience Makefiles and example scripts and add a testing plan.
- C) I will prepare patch diffs for review (no PRs).
- D) You assign tasks to your team and I provide review guidance and diffs on demand.

Please confirm which path you prefer.
