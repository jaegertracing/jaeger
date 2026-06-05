# OpenSSF Best Practices Gold Evidence

This page tracks durable Jaeger evidence for the OpenSSF Best Practices badge entry at <https://www.bestpractices.dev/projects/1273>. It is maintained so badge evidence can point at current `main` branch resources instead of retired branches, old CI systems, or issue-only evidence.

Last reviewed: 2026-05-17.

## Badge Evidence Refresh

Use the following replacements for stale badge evidence.
The stale-evidence refresh is tracked by `https://github.com/jaegertracing/jaeger/issues/8482` and is complete.

| Badge criterion | Replace stale evidence | Current evidence |
| --- | --- | --- |
| `contribution` | `https://github.com/jaegertracing/jaeger/blob/master/CONTRIBUTING.md` | `https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING.md` |
| `contribution_requirements` | `https://github.com/jaegertracing/jaeger/blob/master/CONTRIBUTING.md#making-a-change` | `https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING.md#contributing-code` |
| `license_location` | `https://github.com/jaegertracing/jaeger/blob/master/LICENSE` | `https://github.com/jaegertracing/jaeger/blob/main/LICENSE` |
| `release_notes` | `https://github.com/jaegertracing/jaeger/blob/master/CHANGELOG.md` | `https://github.com/jaegertracing/jaeger/blob/main/CHANGELOG.md` |
| `report_process` | `https://github.com/jaegertracing/jaeger#questions-discussions-bug-reports` | `https://github.com/jaegertracing/jaeger/blob/main/README.md#get-in-touch` and `https://github.com/jaegertracing/jaeger/issues` |
| `build` | `https://github.com/jaegertracing/jaeger/blob/master/Makefile` | `https://github.com/jaegertracing/jaeger/blob/main/Makefile` |
| `build_common_tools` | `https://github.com/jaegertracing/jaeger/blob/master/Makefile` | `https://github.com/jaegertracing/jaeger/blob/main/Makefile` |
| `test_most` | `https://coveralls.io/github/jaegertracing/jaeger?branch=master` | `https://codecov.io/gh/jaegertracing/jaeger/branch/main/` and `https://github.com/jaegertracing/jaeger/blob/main/docs/adr/004-migrating-coverage-gating-to-github-actions.md` |
| `test_policy` | `https://github.com/jaegertracing/jaeger/blob/master/CONTRIBUTING.md#making-a-change` | `https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING.md#testing-guidelines` |
| `tests_documented_added` | `https://github.com/jaegertracing/jaeger/blob/master/CONTRIBUTING.md#making-a-change` | `https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING.md#testing-guidelines` |
| `test_continuous_integration` | `https://travis-ci.org/jaegertracing/jaeger` | `https://github.com/jaegertracing/jaeger/actions/workflows/ci-orchestrator.yml?query=branch%3Amain` and `https://github.com/jaegertracing/jaeger/blob/main/.github/workflows/README.md` |
| `dco` | `https://github.com/jaegertracing/jaeger/blob/master/DCO` | `https://github.com/jaegertracing/jaeger/blob/main/DCO` and `https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING.md#contributing-code` |
| `governance` | `https://github.com/jaegertracing/jaeger/blob/master/GOVERNANCE.md` | `https://github.com/jaegertracing/jaeger/blob/main/GOVERNANCE.md` |
| `code_of_conduct` | `https://github.com/jaegertracing/jaeger/blob/master/CODE_OF_CONDUCT.md` | `https://github.com/jaegertracing/jaeger/blob/main/CODE_OF_CONDUCT.md` |
| `maintenance_or_update` | `https://github.com/jaegertracing/jaeger/blob/master/CHANGELOG.md` | `https://github.com/jaegertracing/jaeger/blob/main/CHANGELOG.md` and `https://github.com/jaegertracing/jaeger/blob/main/RELEASE.md` |
| `coding_standards` | `https://github.com/jaegertracing/jaeger/blob/master/CONTRIBUTING.md` | `https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING.md` and `https://github.com/jaegertracing/jaeger/blob/main/Makefile` |
| `build_repeatable` | Mentions `Gopkg.lock` | `https://github.com/jaegertracing/jaeger/blob/main/go.mod`, `https://github.com/jaegertracing/jaeger/blob/main/go.sum`, and `https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING.md#getting-started` |
| `installation_development_quick` | `https://github.com/jaegertracing/jaeger/blob/master/CONTRIBUTING.md` | `https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING.md#getting-started` |
| `external_dependencies` | `https://github.com/jaegertracing/jaeger/blob/master/Gopkg.toml` | `https://github.com/jaegertracing/jaeger/blob/main/go.mod`, `https://github.com/jaegertracing/jaeger/blob/main/go.sum`, `https://github.com/jaegertracing/jaeger/blob/main/internal/tools/go.mod`, and `https://github.com/jaegertracing/jaeger/blob/main/SECURITY.md#dependency-policy` |
| `automated_integration_testing` | `https://travis-ci.org/jaegertracing/jaeger` | `https://github.com/jaegertracing/jaeger/actions/workflows/ci-e2e-all.yml?query=branch%3Amain` and `https://github.com/jaegertracing/jaeger/blob/main/.github/workflows/README.md#e2e-test-workflows` |
| `test_statement_coverage80` | `https://codecov.io/gh/jaegertracing/jaeger/branch/master/` | `https://codecov.io/gh/jaegertracing/jaeger/branch/main/` and `https://github.com/jaegertracing/jaeger/blob/main/docs/adr/004-migrating-coverage-gating-to-github-actions.md` |
| `crypto_used_network` | `https://github.com/jaegertracing/jaeger/blob/main/SECURITY-ARCHITECTURE.md#tls-configuration` | `https://github.com/jaegertracing/jaeger/blob/main/docs/security/architecture.md#tls-configuration` |
| `crypto_tls12` | `https://github.com/jaegertracing/jaeger/blob/main/SECURITY-ARCHITECTURE.md#tls-version` | `https://github.com/jaegertracing/jaeger/blob/main/docs/security/architecture.md#tls-version` |
| `crypto_certificate_verification` | `https://github.com/jaegertracing/jaeger/blob/main/SECURITY-ARCHITECTURE.md#certificate-verification` | `https://github.com/jaegertracing/jaeger/blob/main/docs/security/architecture.md#certificate-verification` |
| `crypto_verification_private` | `https://github.com/jaegertracing/jaeger/blob/main/SECURITY-ARCHITECTURE.md#certificate-verification` | `https://github.com/jaegertracing/jaeger/blob/main/docs/security/architecture.md#certificate-verification` |
| `hardening` | `https://github.com/jaegertracing/jaeger/blob/main/SECURITY-ARCHITECTURE.md#hardening` | `https://github.com/jaegertracing/jaeger/blob/main/docs/security/architecture.md#system-hardening` and current CI hardening evidence in `https://github.com/jaegertracing/jaeger/blob/main/.github/workflows/README.md#stage-3-expensive-checks--static-analysis` |
| `input_validation` | `https://github.com/jaegertracing/jaeger/blob/main/SECURITY-ARCHITECTURE.md#input-validation` | `https://github.com/jaegertracing/jaeger/blob/main/docs/security/architecture.md#input-validation` |
| `crypto_algorithm_agility` | `https://github.com/jaegertracing/jaeger/blob/main/SECURITY-ARCHITECTURE.md#algorithm-support` | `https://github.com/jaegertracing/jaeger/blob/main/docs/security/architecture.md#algorithm-support` |
| `crypto_credential_agility` | `https://github.com/jaegertracing/jaeger/blob/main/SECURITY-ARCHITECTURE.md#credential-management` | `https://github.com/jaegertracing/jaeger/blob/main/docs/security/architecture.md#credential-management` |

## OpenSSF Scorecard Accepted Exceptions

The public OpenSSF Scorecard result remains detection-based. These entries document intentional Jaeger project decisions for checks where the project accepts the current score instead of adding superficial controls for score improvement only.

| Scorecard check | Current score | Decision | Rationale |
| --- | ---: | --- | --- |
| `Fuzzing` | `0` | Accepted exception | Jaeger is not adding fuzzing solely to improve Scorecard. Useful fuzzing would need explicit target selection, seed corpora, invariants, ownership, triage, and non-PR infrastructure. Placeholder fuzz targets would be misleading. Revisit only for a specific untrusted-input parser or decoder with an owner and clear triage path. |

The fuzzing exception is tracked by `https://github.com/jaegertracing/jaeger/issues/8636`.

## Gold Criteria With Current Evidence

| Gold criterion | Current evidence |
| --- | --- |
| `bus_factor` | `https://github.com/jaegertracing/jaeger/blob/main/MAINTAINERS.md` and `https://github.com/jaegertracing/jaeger/blob/main/GOVERNANCE.md` |
| `contributors_unassociated` | GitHub contributors and maintainers from multiple organizations; use `https://github.com/jaegertracing/jaeger/graphs/contributors`, `https://github.com/jaegertracing/jaeger/blob/main/MAINTAINERS.md`, and CNCF project governance evidence. |
| `small_tasks` | `https://github.com/jaegertracing/jaeger/issues?q=is%3Aopen%20is%3Aissue%20label%3A%22good%20first%20issue%22` and `https://github.com/jaegertracing/jaeger/issues?q=is%3Aopen%20is%3Aissue%20label%3A%22help%20wanted%22` |
| `require_2FA` | `https://github.com/jaegertracing/jaeger/blob/main/GOVERNANCE.md#maintainer-account-security` |
| `secure_2FA` | `https://github.com/jaegertracing/jaeger/blob/main/GOVERNANCE.md#maintainer-account-security` |
| `code_review_standards` | `https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING.md#code-review-requirements` |
| `two_person_review` | `https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING.md#code-review-requirements` |
| `test_invocation` | `https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING.md#getting-started` and `https://github.com/jaegertracing/jaeger/blob/main/Makefile` |
| `test_continuous_integration` | `https://github.com/jaegertracing/jaeger/actions/workflows/ci-orchestrator.yml?query=branch%3Amain` and `https://github.com/jaegertracing/jaeger/blob/main/.github/workflows/README.md` |
| `copyright_per_file` | Header policy: `https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING_GUIDELINES.md#copyright-header`; automated repair and lint enforcement: `https://github.com/jaegertracing/jaeger/blob/main/scripts/lint/updateLicense.py`, `https://github.com/jaegertracing/jaeger/blob/main/Makefile`, and `https://github.com/jaegertracing/jaeger/blob/main/.github/workflows/ci-lint-checks.yaml`. |
| `license_per_file` | SPDX header policy: `https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING_GUIDELINES.md#copyright-header`; automated repair and lint enforcement: `https://github.com/jaegertracing/jaeger/blob/main/scripts/lint/updateLicense.py`, `https://github.com/jaegertracing/jaeger/blob/main/Makefile`, and `https://github.com/jaegertracing/jaeger/blob/main/.github/workflows/ci-lint-checks.yaml`. |
| `security_review` | Current security review: `https://github.com/jaegertracing/jaeger/blob/main/docs/security/security-review-2026.md`; historical public audits are available at `https://github.com/jaegertracing/security-audits`. |

## Hard-Evidence Criteria

The following criteria are tracked by `https://github.com/jaegertracing/jaeger/issues/8484`. Badge evidence should point at this section or the linked maintained files once each entry is accepted in the badge app.

| Gold criterion | Current evidence |
| --- | --- |
| `build_reproducible` | Release binaries are built from source by `https://github.com/jaegertracing/jaeger/blob/main/.github/workflows/ci-release.yml` using the checked-in release scripts and Go toolchain setup. The build uses pinned Go module inputs from `https://github.com/jaegertracing/jaeger/blob/main/go.mod`, `https://github.com/jaegertracing/jaeger/blob/main/go.sum`, `https://github.com/jaegertracing/jaeger/blob/main/internal/tools/go.mod`, and `https://github.com/jaegertracing/jaeger/blob/main/internal/tools/go.sum`. `https://github.com/jaegertracing/jaeger/blob/main/Makefile` includes `make repro-check`, which cleans the workspace, builds all configured platform binaries twice, and verifies matching SHA-256 checksums. The reproducibility scope is the generated executables; signed archives, detached signatures, SBOMs, container images, and registry metadata are release artifacts with timestamps, signatures, or external registry state and are verified separately through `https://github.com/jaegertracing/jaeger/blob/main/docs/security/verifying-releases.md`. |
| `test_statement_coverage90` | Jaeger enforces a 95 percent statement coverage floor in `https://github.com/jaegertracing/jaeger/blob/main/.github/workflows/ci-summary-report.yml`, which merges coverage artifacts, applies the shared `.codecov.yml` exclusions, and fails the coverage gate below 95 percent or on regression from the `main` baseline. The design and evidence links are documented in `https://github.com/jaegertracing/jaeger/blob/main/docs/adr/004-migrating-coverage-gating-to-github-actions.md`; long-term trend evidence remains available at `https://codecov.io/gh/jaegertracing/jaeger/branch/main/`. |
| `test_branch_coverage80` | The maintained Go coverage path uses the FLOSS Go coverage tooling and reports statement coverage rather than a separate branch-coverage metric. Jaeger does not currently maintain a separate branch-coverage gate. If the badge app accepts language/tooling constraints for this criterion, this should be marked N/A with this rationale and the statement-coverage gate above as the maintained coverage control. |
| `hardened_site` | The repository and GitHub release/download pages are served by GitHub with nonpermissive security headers. The Jaeger website and documentation are served from project website hosting, and Jaeger-controlled pages should be verified with `curl -fsSI -L https://www.jaegertracing.io/`, `curl -fsSI -L https://www.jaegertracing.io/docs/latest/`, and `curl -fsSI -L https://www.jaegertracing.io/download/` before updating the badge. As of the 2026-05-17 review, these Jaeger website surfaces return HSTS, but additional hardening headers such as Content-Security-Policy, X-Content-Type-Options, X-Frame-Options, Referrer-Policy, and Permissions-Policy require website-hosting configuration or an explicit BadgeApp hosting-constraint note. |
| `hardening` | Jaeger documents system hardening in `https://github.com/jaegertracing/jaeger/blob/main/docs/security/architecture.md#system-hardening`. CI hardening evidence includes pinned GitHub Actions, `step-security/harden-runner`, CodeQL, dependency review, FOSSA, multi-platform binary builds, Docker image builds, and E2E workflows in `https://github.com/jaegertracing/jaeger/blob/main/.github/workflows/README.md#stage-3-expensive-checks--static-analysis`. Release integrity evidence is documented in `https://github.com/jaegertracing/jaeger/blob/main/docs/security/verifying-releases.md`. |
| `dynamic_analysis` | Jaeger applies dynamic analysis through race-enabled Go tests and integration/E2E workflows. `https://github.com/jaegertracing/jaeger/blob/main/Makefile` runs `make test` with the Go race detector on supported architectures and `make cover` with the same race-enabled test path. The CI workflow inventory in `https://github.com/jaegertracing/jaeger/blob/main/.github/workflows/README.md` documents unit tests, storage integration tests, and E2E workflows that exercise Jaeger binaries and storage backends before changes are merged and before release candidates are accepted. |
| `dynamic_analysis_enable_assertions` | Jaeger's dynamic-analysis path exercises Go test assertions in unit, integration, and E2E tests. The project is not adding fuzzing for this criterion; dynamic-analysis evidence is based on the maintained race-enabled test and E2E workflows above. |

## Remaining Gold Work

The following criteria need more than URL refresh and are tracked by the Gold badge parent issue.

| Tracking issue | Pending criteria | Evidence needed before badge update |
| --- | --- | --- |
| `https://github.com/jaegertracing/jaeger/issues/8483` | `small_tasks` | Active newcomer-task evidence, discoverable issue labels, and a lightweight maintainer process for keeping suitable tasks available. |
| `https://github.com/jaegertracing/jaeger/issues/8484` | `test_branch_coverage80`, `hardened_site` | Hard-evidence criteria are documented above. Remaining follow-up is to record the branch-coverage N/A rationale in BadgeApp if accepted and either add the missing website hardening headers in website hosting or document the hosting constraint in BadgeApp. |
