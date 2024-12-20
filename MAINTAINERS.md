The current Maintainers Group for the Jaeger Project consists of:

| Name | Employer | Responsibilities |
| ---- | -------- | ---------------- |
| [@albertteoh](https://github.com/albertteoh) | PackSmith | ALL | 
| [@jkowall](https://github.com/jkowall) | Aiven | ALL |
| [@joe-elliott](https://github.com/joe-elliott) | Grafana Labs | ALL |
| [@mahadzaryab1](https://github.com/mahadzaryab1) | Bloomberg | ALL |
| [@pavolloffay](https://github.com/pavolloffay) | RedHat | ALL |
| [@yurishkuro](https://github.com/yurishkuro) | Meta | ALL |

This list must be kept in sync with the [CNCF Project Maintainers list](https://github.com/cncf/foundation/blob/master/project-maintainers.csv).

See [the project Governance](./GOVERNANCE.md) for how maintainers are selected and replaced.

### Emeritus Maintainers

We are grateful to our former maintainers for their contributions to the Jaeger project.

* [@black-adder](https://github.com/black-adder)
* [@jpkrohling](https://github.com/jpkrohling)
* [@objectiser](https://github.com/objectiser)
* [@tiffon](https://github.com/tiffon)
* [@vprithvi](https://github.com/vprithvi)

### Maintainer Onboarding

Upon approval, the following steps should be taken to onboard the new maintainer:

*   **1. Update Project Documentation**
    *   **`MAINTAINERS.md` File:** Merge the PR to add the new maintainer to the `MAINTAINERS.md` file(s) in the relevant Jaeger repositories.
*   **2. Grant Permissions**
    *   **GitHub:** Add the new maintainer to the `@jaegertracing/jaeger-maintainers` GitHub team. This grants them write access to the Jaeger repositories.
    *   **CNCF Mailing List:** Add the new maintainer to the `cncf-jaeger-maintainers@lists.cncf.io` mailing list (and any other relevant Jaeger mailing lists). Contact the existing `cncf-jaeger-maintainers` to find out the precise process for adding to the mailing list, it will likely involve getting in touch with the CNCF.
    *   **CNCF Maintainer Registry:**
        *   Create a PR against the `cncf/foundation` repository to add the new maintainer's information to the `project-maintainers.csv` file. The following fields are required:
        *   Reference the PR in the `cncf-jaeger-maintainers` mailing list.
    *   **Signing Keys:**
        *   Jaeger uses a GPG key for encrypted emails sent to the maintainers for security reports along with access to the `maintainers-only` GitHub repository. This key is stored in our 1password repository. 
    *   **1Password:** Connect with an existing maintainer to be added to our jaegertracing 1Password team.
*   **3. Announcement**
    *   Announce the new maintainer to the Jaeger community through the mailing list, blog, or other appropriate channels.

### Maintainer Offboarding

The process for removing a maintainer is similar to adding one. A maintainer can step down voluntarily or be removed by a vote of the other maintainers if they are no longer fulfilling their responsibilities or are violating the project's Code of Conduct. A supermajority vote is needed to remove a maintainer. Their access should be revoked from all relevant tools, and the project documentation updated accordingly.
