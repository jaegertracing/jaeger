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

### Criteria for Maintainership

Candidates for maintainership should demonstrate the following qualities:

*   **Sustained Contributions:** A history of significant and high-quality contributions to the Jaeger project (e.g., code, documentation, testing).
*   **Code Review Proficiency:** Demonstrated ability to provide thorough, constructive, and timely code reviews.
*   **Community Engagement:** Active participation in the Jaeger community (e.g., helping users on mailing lists/forums, participating in discussions, organizing events).
*   **Technical Expertise:** A strong understanding of Jaeger's architecture, codebase, and related technologies.
*   **Commitment:** A willingness to dedicate the necessary time and effort to fulfill maintainer responsibilities.
*   **Alignment with Project Values:** Adherence to the CNCF Code of Conduct and a commitment to fostering a welcoming and inclusive community.

### Maintainer Nomination Process

*   **Initiation:** Any existing Jaeger maintainer can nominate a candidate by creating a pull request (PR) to update the `OWNERS` file in the relevant Jaeger repository. It is a good idea to notify other maintainers of the nomination before creating the PR, this is typically done in our private Slack channel #jaeger-maintainers-only.
*   **PR Content:** The PR description must include:
    *   The nominee's full name and GitHub username.
    *   A justification for the nomination, highlighting the nominee's contributions and qualifications based on the criteria outlined in Section 2.
    *   Links to relevant contributions (e.g., PRs, issues, discussions).
*   **Notification:** The nominator should notify the other Jaeger maintainers of the nomination through the maintainers' communication channel (e.g., mailing list, Slack channel) and add a link to the PR.

### Evaluation and Voting

*   **Discussion:** Jaeger maintainers will discuss the nomination in a maintainers' meeting, on the maintainers' mailing list, or in the PR comments.
*   **Voting:**  After a reasonable discussion period (typically at least one week), maintainers will vote on the nomination.
    *   Voting can be done on our private slack channel $jaeger-maintainers-only.
    *   A supermajority vote (e.g., 2/3 or more of existing maintainers) is required for approval.
    *   The PR that adds the maintainer should be approved.
*   **Outcome:**
    *   **Approval:** If the nomination is approved, proceed to the Onboarding section.
    *   **Rejection:** If the nomination is rejected, provide feedback to the nominee (if appropriate) and close the PR.

### Onboarding

Upon approval, the following steps should be taken to onboard the new maintainer:

*   **1. Update Project Documentation**
    *   **`OWNERS` File:** Merge the PR to add the new maintainer to the `OWNERS` file(s) in the relevant Jaeger repositories.
    *   **Project Website:** Update the Jaeger website (if applicable) to list the new maintainer.
*   **2. Grant Permissions**
    *   **GitHub:** Add the new maintainer to the `jaeger-maintainer` GitHub team. This grants them write access to the Jaeger repositories.
    *   **CNCF Mailing List:** Add the new maintainer to the `cncf-jaeger-maintainers@lists.cncf.io` mailing list (and any other relevant Jaeger mailing lists). Contact the existing `cncf-jaeger-maintainers` to find out the precise process for adding to the mailing list, it will likely involve getting in touch with the CNCF.
    *   **CNCF Maintainer Registry:**
        *   Create a PR against the `cncf/foundation` repository to add the new maintainer's information to the `project-maintainers.csv` file. The following fields are required:
            *   `project`: `jaeger`
            *   `name`: Full name of the new maintainer
            *   `github`: GitHub username
            *   `irc`: IRC nickname (optional)
            *   `slack`: Slack ID (optional)
            *   `slack_notifications`: Slack ID for notifications (optional)
            *   `email`: Email address
            *   `modtime`: Timestamp (will be automatically updated)
        *   Reference the PR in the `cncf-jaeger-maintainers` mailing list.
    *   **Signing Keys:**
        *   Jaeger uses a GPG key for encrypted emails sent to the maintainers. This key is stored in our 1password repository. 
    *   **1Password:** Connect with an existing maintainer to be added to our jaegertracing 1Password team.
*   **5.4. Announcement**
    *   Announce the new maintainer to the Jaeger community through the mailing list, blog, or other appropriate channels.

### Removing Maintainers*

The process for removing a maintainer is similar to adding one. A maintainer can step down voluntarily or be removed by a vote of the other maintainers if they are no longer fulfilling their responsibilities or are violating the project's Code of Conduct. A supermajority vote is needed to remove a maintainer. Their access should be revoked from all relevant tools, and the project documentation updated accordingly.

### Conflict Resolution

In case of disagreements during the nomination, evaluation, or removal process, maintainers should strive to reach a consensus through respectful discussion. If a consensus cannot be reached, the maintainers may choose to escalate the issue to a higher authority within the CNCF (e.g., the TOC) as per the project's governance model.

### Document Updates

This document should be reviewed and updated periodically to ensure it remains accurate and relevant. Any changes to this document should be proposed and approved by the Jaeger maintainers.