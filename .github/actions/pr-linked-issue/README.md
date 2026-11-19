# PR Linked Issue

This action requires a pull request from an outside contributor to be linked to an open issue that carries the `help wanted` label. It adds the `missing-issue` and `stale` labels to a pull request that has no such issue, and explains the policy in a comment. It is a composite action so that another Jaeger repository can call it at a pinned commit.

The logic lives in [`pr-linked-issue.js`](pr-linked-issue.js), next to its tests, which run through `npm test` in [`.github/scripts`](../../scripts).

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

Without `pull-request-number` and `issue-number` the action checks every open pull request.
| `dry-run` | no | `false` | Report which pull requests violate the policy without writing anything. |

## The policy

A pull request meets the policy when one of the first 20 issues that it closes is open, carries `help wanted`, and belongs to a repository of the same organization. The action does not check who applied `help wanted`, because GitHub lets only a user with triage permission or higher label an issue.

A pull request is exempt when its author has write access to the repository, when it comes from Dependabot or Renovate, or when a maintainer applied the `no-issue-needed` label to it.

The action closes no pull request. The step of `.github/workflows/stale.yml` that is restricted to `missing-issue` closes the pull request after about a week without activity, and a repository that calls this action needs the same step. The `do-not-expire` label does not protect a pull request from that step; `no-issue-needed` is the exemption for this policy. When the pull request later meets the policy, the action removes both labels.

## When the action runs

`.github/workflows/pr-linked-issue.yml` calls the action when a pull request is opened, edited or reopened, when `no-issue-needed` is added to or removed from a pull request, and when an issue is closed, reopened, or has `help wanted` added or removed. The workflow has no scheduled run, so a pull request that was open before the workflow existed is checked only when one of those events touches it.

Linking an issue through the sidebar, and a label or state change on a linked issue in another repository, fire no event for the pull request. Its description has to be edited, or the workflow run manually for it, before it is checked again.

## The comment

The action keeps one current comment on a flagged pull request. It rewrites that comment when the linked issues change, and when the pull request meets the policy. A pull request that is flagged again gets a new comment, because GitHub notifies the author of a new comment and not of an edited one.

The action recognizes its comment by a hidden marker in the comment body. It honors the marker only in a comment from `github-actions[bot]`, which is the account of a workflow, or from an owner, organization member or collaborator. Pasting the marker into any other comment does not make the action rewrite that comment.
