# PR Linked Issue

This action requires a pull request from an outside contributor to be linked to a qualifying issue: an open issue that carries the `help wanted` label. It adds the `missing-issue` and `stale` labels to a pull request that has no qualifying issue, and explains the policy in a comment. It is a composite action so that another Jaeger repository can call it at a pinned commit. A pull request that carries `missing-issue` is called flagged below.

The logic lives in [`pr-linked-issue.js`](pr-linked-issue.js) in this directory, next to its tests. The tests run with the jest setup of [`.github/scripts`](../../scripts), through `npm test` there.

## Using the action

```yaml
permissions:
  pull-requests: write
  issues: write

steps:
  - uses: jaegertracing/jaeger/.github/actions/pr-linked-issue@<commit-sha>
    with:
      token: ${{ github.token }}
      pull-request-number: ${{ github.event.pull_request.number }}
```

| Input | Required | Default | Purpose |
|---|---|---|---|
| `token` | yes | — | Token used to label and comment. Needs `issues: write` and `pull-requests: write`. |
| `pull-request-number` | no | — | The pull request to check. Takes precedence over `issue-number`. |
| `issue-number` | no | — | The issue whose event asked for the run. The open pull requests linked to it are checked. |
| `dry-run` | no | `false` | Log the intended labels and comments without writing anything. |

Without `pull-request-number` and `issue-number` the action checks every open pull request.

## The policy

The action reads GitHub's canonical `closingIssuesReferences` relationship, which a closing keyword in the description or a link in the sidebar creates. A pull request meets the policy when at least one linked issue is open, carries `help wanted`, and belongs to a repository of the same organization. An issue in another Jaeger repository qualifies because work on one repository is often tracked in another. The action reads the first 20 linked issues of a pull request, and a pull request with no qualifying issue among them violates the policy.

The action does not check who applied `help wanted`. GitHub lets only a user with triage permission or higher label an issue, so the label on an issue is already a maintainer's decision.

Three kinds of pull request are exempt:

- A pull request whose author has write access to the repository.
- A pull request from Dependabot or Renovate. Other bot accounts are not exempt, because a coding agent can also open a pull request as a bot.
- A pull request to which a maintainer applied the `no-issue-needed` label.

When a pull request violates the policy, the action posts one comment and then adds `missing-issue` and `stale`. When the pull request later meets the policy, the action removes both labels and rewrites its comment to say so. A pull request that is flagged again gets a new comment, because GitHub notifies the author of a new comment and not of an edited one.

The action recognizes its comment by a hidden marker in the comment body. It honors the marker only in a comment from `github-actions[bot]`, which is the account of the workflow, or from an owner, organization member or collaborator, which covers a maintainer who ran the local script. Pasting the marker into any other comment does not make the action rewrite that comment.

## Closing

The action closes nothing. The step of `.github/workflows/stale.yml` that is restricted to `missing-issue` closes a flagged pull request after about a week without activity. Activity on the pull request makes that step remove `stale`, and the step adds the label back on a later daily run, which restarts the count. The `do-not-expire` label does not protect a pull request from this step; `no-issue-needed` is the exemption for this policy. A repository that calls this action needs the same step in its own stale workflow, otherwise nothing closes a flagged pull request there.

The action adds `stale` itself so that other automation treats the pull request as inactive from the moment it is flagged. The [duplicate PR action](../pr-duplicates), for example, does not let a stale pull request be the primary one for an issue.

## When the action runs

`.github/workflows/pr-linked-issue.yml` calls the action when a pull request is opened, edited or reopened, when `no-issue-needed` is added to or removed from a pull request, and when an issue is closed, reopened, or has `help wanted` added or removed. The workflow has no scheduled run, so a pull request that was open before the workflow existed is checked only when one of those events touches it.

Two changes fire no event in the repository of the pull request: linking an issue through the sidebar, and a label or state change on a linked issue that lives in another repository. The pull request keeps the state it had until its description is edited or the workflow is run manually for it. A flagged pull request that has come to meet the policy in one of these two ways is therefore still closed unless it is checked again.

## Running locally

The script in `.github/scripts` applies the policy to every open pull request of a repository. It reports what it would do and writes nothing unless `--apply` is given:

```bash
cd .github/scripts
npm ci
export GITHUB_TOKEN=$(gh auth token)
node apply-pr-linked-issue-policy.js                              # dry run on jaegertracing/jaeger
node apply-pr-linked-issue-policy.js jaegertracing jaeger-ui      # dry run on another repository
node apply-pr-linked-issue-policy.js --pr 1234                    # dry run on one pull request
node apply-pr-linked-issue-policy.js --apply                      # label and comment
```

With `--apply` the comments are posted by the owner of the token, and the workflow later maintains those comments as its own.
