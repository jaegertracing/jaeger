# OpenSSF Best Practices Gold Evidence

This page tracks durable Jaeger evidence for the OpenSSF Best Practices badge entry at <https://www.bestpractices.dev/projects/1273>. It is maintained so badge evidence can point at current `main` branch resources instead of retired branches, old CI systems, or issue-only evidence.

Last reviewed: 2026-05-08.

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

## Gold Criteria With Current Evidence

| Gold criterion | Current evidence |
| --- | --- |
| `bus_factor` | `https://github.com/jaegertracing/jaeger/blob/main/MAINTAINERS.md` and `https://github.com/jaegertracing/jaeger/blob/main/GOVERNANCE.md` |
| `contributors_unassociated` | GitHub contributors and maintainers from multiple organizations; use `https://github.com/jaegertracing/jaeger/graphs/contributors`, `https://github.com/jaegertracing/jaeger/blob/main/MAINTAINERS.md`, and CNCF project governance evidence. |
| `small_tasks` | `https://github.com/jaegertracing/jaeger/issues?q=is%3Aopen%20is%3Aissue%20label%3A%22good%20first%20issue%22` and `https://github.com/jaegertracing/jaeger/issues?q=is%3Aopen%20is%3Aissue%20label%3A%22help%20wanted%22` |
| `test_invocation` | `https://github.com/jaegertracing/jaeger/blob/main/CONTRIBUTING.md#getting-started` and `https://github.com/jaegertracing/jaeger/blob/main/Makefile` |
| `test_continuous_integration` | `https://github.com/jaegertracing/jaeger/actions/workflows/ci-orchestrator.yml?query=branch%3Amain` and `https://github.com/jaegertracing/jaeger/blob/main/.github/workflows/README.md` |
| `security_review` | Historical public audits are available at `https://github.com/jaegertracing/security-audits`; current-within-5-years evidence is tracked by issue `https://github.com/jaegertracing/jaeger/issues/8485`. |

## Pending Gold Work

The following criteria need more than URL refresh and are tracked by the Gold badge parent issue.

| Tracking issue | Pending criteria | Evidence needed before badge update |
| --- | --- | --- |
| `https://github.com/jaegertracing/jaeger/issues/8486` | `require_2FA`, `secure_2FA`, `code_review_standards`, `two_person_review` | Linkable maintainer account-security policy and code-review requirements covering reviewer responsibilities, approval expectations, tests, generated files, and security-sensitive changes. |
| `https://github.com/jaegertracing/jaeger/issues/8487` | `copyright_per_file`, `license_per_file` | In-scope source files carry copyright and SPDX license identifiers, with CI enforcement to prevent regressions. |
| `https://github.com/jaegertracing/jaeger/issues/8483` | `small_tasks` | Active newcomer-task evidence, discoverable issue labels, and a lightweight maintainer process for keeping suitable tasks available. |
| `https://github.com/jaegertracing/jaeger/issues/8484` | `build_reproducible`, `test_statement_coverage90`, `test_branch_coverage80`, `hardened_site`, `hardening`, `dynamic_analysis`, `dynamic_analysis_enable_assertions` | Maintained technical evidence or justified exceptions for reproducible builds, coverage, hardened headers, hardening, and release-time dynamic analysis. |
| `https://github.com/jaegertracing/jaeger/issues/8485` | `security_review` | Current security review evidence from within the Gold five-year window, published without disclosing sensitive vulnerability details. |
